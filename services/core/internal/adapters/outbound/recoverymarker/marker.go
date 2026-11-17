// Package recoverymarker stores the external recovery marker
// (docs/design/database/recovery-generation.md section 3) in a file that lives
// outside PostgreSQL and outside the documented backup set.
//
// The file is the one witness that a restore of the database cannot change. It
// is not a secret and it is not authenticated: its check value detects
// accidental damage and nothing else (section 10).
package recoverymarker

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/platform/posix"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

const (
	fileName  = "recovery-generation.json"
	lockName  = "recovery-generation.lock"
	tmpPrefix = ".recovery-generation.json.tmp-"
	// maxSize is far above the size of a valid marker (about 250 bytes).
	maxSize = 4 << 10
)

// Store reads and writes the marker in one directory.
type Store struct {
	dir string
}

var _ outbound.RecoveryMarkerStore = (*Store)(nil)

// New returns a Store for dir, which must be an absolute path. It creates the
// directory with mode 0700 when it does not exist. It refuses a directory that
// is a symlink, is not owned by the running user, or is accessible to others.
func New(dir string) (*Store, error) {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return nil, errors.New("recoverymarker: the directory must be a clean absolute path")
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return nil, fmt.Errorf("recoverymarker: create the parent directory: %w", err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("recoverymarker: create the directory: %w", err)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("recoverymarker: %w", err)
	}
	if !fi.IsDir() {
		return nil, errors.New("recoverymarker: the state path is not a directory (a symlink is refused)")
	}
	if uid, ok := posix.Owner(fi); !ok || uid != posix.CurrentUser() {
		return nil, errors.New("recoverymarker: the directory is not owned by the running user")
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("recoverymarker: the directory has mode %o and must not be accessible to others (0700)", fi.Mode().Perm())
	}
	return &Store{dir: dir}, nil
}

type wire struct {
	Version        int    `json:"version"`
	InstanceID     string `json:"instance_id"`
	Generation     int64  `json:"generation"`
	AuditWatermark int64  `json:"audit_watermark"`
	WrittenAt      string `json:"written_at"`
	Check          string `json:"check"`
}

func (w wire) check() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("morningstar-recovery-marker\nv%d\n%s\n%d\n%d\n%s", w.Version, w.InstanceID, w.Generation, w.AuditWatermark, w.WrittenAt)))
	return hex.EncodeToString(sum[:])
}

func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}

func validate(m outbound.RecoveryMarker) error {
	switch {
	case !validUUID(m.InstanceID):
		return errors.New("the instance id is not a lower-case UUID")
	case m.Generation < 1 || m.Generation > math.MaxInt64:
		return errors.New("the generation is out of range")
	case m.AuditWatermark > math.MaxInt64:
		return errors.New("the audit watermark is out of range")
	case m.WrittenAt.IsZero():
		return errors.New("the write time is missing")
	}
	return nil
}

// Read returns the marker. It returns ErrMarkerMissing when there is no file
// and ErrMarkerInvalid for anything that is not a regular, owner-only,
// well-formed file whose check value matches.
func (s *Store) Read(ctx context.Context) (outbound.RecoveryMarker, error) {
	if err := ctx.Err(); err != nil {
		return outbound.RecoveryMarker{}, err
	}
	return s.read()
}

func (s *Store) read() (outbound.RecoveryMarker, error) {
	path := filepath.Join(s.dir, fileName)
	fi, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return outbound.RecoveryMarker{}, outbound.ErrMarkerMissing
	}
	if err != nil {
		return outbound.RecoveryMarker{}, outbound.ErrMarkerInvalid
	}
	if !fi.Mode().IsRegular() || fi.Size() > maxSize {
		return outbound.RecoveryMarker{}, outbound.ErrMarkerInvalid
	}
	if uid, ok := posix.Owner(fi); !ok || uid != posix.CurrentUser() || fi.Mode().Perm()&^0o600 != 0 {
		return outbound.RecoveryMarker{}, outbound.ErrMarkerInvalid
	}
	f, err := os.OpenFile(path, os.O_RDONLY|posix.NoFollow, 0)
	if err != nil {
		return outbound.RecoveryMarker{}, outbound.ErrMarkerInvalid
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil || len(b) > maxSize {
		return outbound.RecoveryMarker{}, outbound.ErrMarkerInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var w wire
	if err := dec.Decode(&w); err != nil || dec.More() {
		return outbound.RecoveryMarker{}, outbound.ErrMarkerInvalid
	}
	at, err := time.Parse(time.RFC3339Nano, w.WrittenAt)
	if err != nil || w.Version != 1 || w.Check != w.check() || w.Generation < 1 || w.AuditWatermark < 0 || !validUUID(w.InstanceID) {
		return outbound.RecoveryMarker{}, outbound.ErrMarkerInvalid
	}
	return outbound.RecoveryMarker{InstanceID: w.InstanceID, Generation: uint64(w.Generation), AuditWatermark: uint64(w.AuditWatermark), WrittenAt: at}, nil
}

// Replace writes the marker atomically, creating it or repairing it. See
// outbound.RecoveryMarkerStore.
func (s *Store) Replace(ctx context.Context, m outbound.RecoveryMarker) error {
	return s.put(ctx, m, false)
}

// Advance records a new watermark in the marker that exists and writes nothing
// when the marker is missing or invalid.
func (s *Store) Advance(ctx context.Context, m outbound.RecoveryMarker) error {
	return s.put(ctx, m, true)
}

// put holds an exclusive lock for the whole read-compare-write, so writers in
// other processes queue up. Within one instance and generation it keeps the
// highest audit watermark. It refuses to lower the generation, and it refuses a
// marker of another instance unless the generation is higher (a reconciliation
// does that).
func (s *Store) put(ctx context.Context, m outbound.RecoveryMarker, requireExisting bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validate(m); err != nil {
		return fmt.Errorf("recoverymarker: %w", err)
	}
	unlock, err := posix.LockFile(filepath.Join(s.dir, lockName))
	if err != nil {
		return fmt.Errorf("recoverymarker: %w", err)
	}
	defer unlock() //nolint:errcheck

	cur, err := s.read()
	switch {
	case err == nil:
		switch {
		case m.Generation < cur.Generation:
			return outbound.ErrMarkerStale
		case m.Generation == cur.Generation && m.InstanceID != cur.InstanceID:
			return outbound.ErrMarkerStale
		case m.Generation == cur.Generation && cur.AuditWatermark > m.AuditWatermark:
			m.AuditWatermark = cur.AuditWatermark
		}
	case requireExisting:
		return err // missing or invalid: Advance never creates or repairs
	}
	// With Replace, a missing or invalid marker is replaced: this is how the first
	// run creates it and how a reconciliation repairs it.

	w := wire{Version: 1, InstanceID: m.InstanceID, Generation: int64(m.Generation), AuditWatermark: int64(m.AuditWatermark), WrittenAt: m.WrittenAt.UTC().Format(time.RFC3339Nano)}
	w.Check = w.check()
	b, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("recoverymarker: %w", err)
	}
	return s.replace(append(b, '\n'))
}

func (s *Store) replace(content []byte) error {
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return fmt.Errorf("recoverymarker: %w", err)
	}
	tmp := filepath.Join(s.dir, tmpPrefix+hex.EncodeToString(rnd[:]))
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|posix.NoFollow, 0o600)
	if err != nil {
		return fmt.Errorf("recoverymarker: create the temporary file: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return fmt.Errorf("recoverymarker: write: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("recoverymarker: fsync: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("recoverymarker: close: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(s.dir, fileName)); err != nil {
		return fmt.Errorf("recoverymarker: rename: %w", err)
	}
	ok = true
	// The rename is the commit. The directory sync makes it durable.
	d, err := os.Open(s.dir)
	if err != nil {
		return fmt.Errorf("recoverymarker: open the directory: %w", err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("recoverymarker: fsync the directory: %w", err)
	}
	return nil
}

// Cleanup removes the temporary files of interrupted writes. It touches no
// other file.
func (s *Store) Cleanup(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	unlock, err := posix.LockFile(filepath.Join(s.dir, lockName))
	if err != nil {
		return fmt.Errorf("recoverymarker: %w", err)
	}
	defer unlock() //nolint:errcheck
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("recoverymarker: %w", err)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), tmpPrefix) || !e.Type().IsRegular() {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, e.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("recoverymarker: remove a temporary file: %w", err)
		}
	}
	return nil
}
