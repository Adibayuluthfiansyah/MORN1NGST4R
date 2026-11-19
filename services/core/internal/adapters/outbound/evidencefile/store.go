// Package evidencefile is the local file store of evidence (D-62, D-77, P-37).
//
// It publishes a file with a write-once protocol: a temporary file created
// with O_EXCL and mode 0600, hashed while it is written, synced, linked to its
// final name with link(2), and then the directory is synced. link(2) never
// replaces an existing name, so a published file is never overwritten by a later
// publish. The store never renames over a name.
//
// A read does not trust the metadata. It looks at the file as it is, and reports
// a file that is not a plain, owner-only file with a single link as unsafe,
// whatever its bytes are. All access goes through an os.Root, so a key cannot
// leave the directory, and a symlink is never followed.
package evidencefile

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/platform/posix"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

const (
	tmpPrefix = ".tmp-"
	chunk     = 64 << 10
)

var keyPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var errInvalidKey = errors.New("evidencefile: the key is not a lower-case UUID")

// Store keeps evidence files in one directory.
type Store struct {
	root *os.Root
	max  int64
}

var _ outbound.EvidenceStore = (*Store)(nil)

// New returns a Store for dir, which must be a clean absolute path. It creates
// the directory with mode 0700 when it does not exist. It refuses a directory
// that is a symlink, is not owned by the running user, or is open to others.
// maxBytes is the largest content that the store accepts.
func New(dir string, maxBytes int64) (*Store, error) {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return nil, errors.New("evidencefile: the directory must be a clean absolute path")
	}
	if maxBytes < 1 {
		return nil, errors.New("evidencefile: the size limit must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return nil, fmt.Errorf("evidencefile: create the parent directory: %w", err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, fmt.Errorf("evidencefile: create the directory: %w", err)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("evidencefile: %w", err)
	}
	if !fi.IsDir() {
		return nil, errors.New("evidencefile: the evidence path is not a directory (a symlink is refused)")
	}
	if uid, ok := posix.Owner(fi); !ok || uid != posix.CurrentUser() {
		return nil, errors.New("evidencefile: the directory is not owned by the running user")
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("evidencefile: the directory has mode %o and must not be accessible to others (0700)", fi.Mode().Perm())
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("evidencefile: open the directory: %w", err)
	}
	return &Store{root: root, max: maxBytes}, nil
}

// Close releases the directory handle.
func (s *Store) Close() error { return s.root.Close() }

func (s *Store) MaxBytes() int64 { return s.max }

func checkKey(key string) error {
	if !keyPattern.MatchString(key) {
		return errInvalidKey
	}
	return nil
}

// Put publishes content under key. It returns ErrEvidenceExists when the name is
// taken, whatever holds it, and leaves that name as it was.
func (s *Store) Put(ctx context.Context, key string, content []byte) (outbound.EvidenceWritten, error) {
	if err := checkKey(key); err != nil {
		return outbound.EvidenceWritten{}, err
	}
	if int64(len(content)) > s.max {
		return outbound.EvidenceWritten{}, outbound.ErrEvidenceTooLarge
	}
	if err := ctx.Err(); err != nil {
		return outbound.EvidenceWritten{}, err
	}
	var rnd [16]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return outbound.EvidenceWritten{}, fmt.Errorf("evidencefile: random name: %w", err)
	}
	tmp := tmpPrefix + hex.EncodeToString(rnd[:])
	f, err := s.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|posix.NoFollow, 0o600)
	if err != nil {
		return outbound.EvidenceWritten{}, fmt.Errorf("evidencefile: create the temporary file: %w", err)
	}
	// The temporary name goes away on every path. After a successful link the
	// file has two names until this removal, and the check below makes sure that
	// the removal worked.
	removed := false
	defer func() {
		if !removed {
			_ = s.root.Remove(tmp)
		}
	}()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), bytes.NewReader(content)); err != nil {
		_ = f.Close()
		return outbound.EvidenceWritten{}, fmt.Errorf("evidencefile: write: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return outbound.EvidenceWritten{}, fmt.Errorf("evidencefile: sync the file: %w", err)
	}
	if err := f.Close(); err != nil {
		return outbound.EvidenceWritten{}, fmt.Errorf("evidencefile: close the file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return outbound.EvidenceWritten{}, err
	}
	if err := s.root.Link(tmp, key); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return outbound.EvidenceWritten{}, outbound.ErrEvidenceExists
		}
		return outbound.EvidenceWritten{}, fmt.Errorf("evidencefile: publish: %w", err)
	}
	if err := s.root.Remove(tmp); err != nil {
		// The published file would keep two names and read as unsafe. Take it back.
		_ = s.root.Remove(key)
		return outbound.EvidenceWritten{}, fmt.Errorf("evidencefile: remove the temporary name: %w", err)
	}
	removed = true
	if err := s.syncDir(); err != nil {
		// The file is published and complete, but its name may not survive a crash.
		// The caller treats this as a failed capture and removes the file.
		return outbound.EvidenceWritten{}, err
	}
	var d evidence.Digest
	copy(d[:], h.Sum(nil))
	return outbound.EvidenceWritten{Size: int64(len(content)), Digest: d}, nil
}

func (s *Store) syncDir() error {
	d, err := s.root.Open(".")
	if err != nil {
		return fmt.Errorf("evidencefile: open the directory to sync it: %w", err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("evidencefile: sync the directory: %w", err)
	}
	return nil
}

// Remove deletes the file under key. It removes the name and never follows it.
func (s *Store) Remove(ctx context.Context, key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.root.Remove(key); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("evidencefile: remove: %w", err)
	}
	return nil
}

// Observe hashes the file under key. It reads at most limit bytes: a file that
// is longer is reported by its size and is not hashed, so a file that a local
// process made huge costs a stat and not a read.
func (s *Store) Observe(ctx context.Context, key string, limit int64) (evidence.Observation, error) {
	if limit < 0 {
		return evidence.Observation{}, errors.New("evidencefile: negative limit")
	}
	if limit > s.max {
		return evidence.Observation{}, outbound.ErrEvidenceTooLarge
	}
	o, _, err := s.look(ctx, key, limit, false)
	return o, err
}

// Read returns the observation and the bytes that it hashed.
func (s *Store) Read(ctx context.Context, key string, limit int64) (evidence.Observation, []byte, error) {
	if limit < 0 {
		return evidence.Observation{}, nil, errors.New("evidencefile: negative limit")
	}
	if limit > s.max {
		return evidence.Observation{}, nil, outbound.ErrEvidenceTooLarge
	}
	if err := checkKey(key); err != nil {
		return evidence.Observation{}, nil, err
	}
	return s.look(ctx, key, limit, true)
}

// look inspects the file. It reads at most limit bytes, and returns them when
// retain is set.
func (s *Store) look(ctx context.Context, key string, limit int64, retain bool) (evidence.Observation, []byte, error) {
	if err := checkKey(key); err != nil {
		return evidence.Observation{}, nil, err
	}
	if err := ctx.Err(); err != nil {
		return evidence.Observation{}, nil, err
	}
	before, err := s.root.Lstat(key)
	if errors.Is(err, fs.ErrNotExist) {
		return evidence.Observation{}, nil, nil
	}
	if err != nil {
		return evidence.Observation{}, nil, fmt.Errorf("evidencefile: stat: %w", err)
	}
	unsafe := evidence.Observation{Found: true, Size: before.Size()}
	if !before.Mode().IsRegular() {
		unsafe.Size = 0
		return unsafe, nil, nil
	}
	f, err := s.root.OpenFile(key, os.O_RDONLY|posix.NoFollow|posix.NonBlock, 0)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return evidence.Observation{}, nil, nil
		}
		return evidence.Observation{}, nil, fmt.Errorf("evidencefile: open: %w", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return evidence.Observation{}, nil, fmt.Errorf("evidencefile: stat the open file: %w", err)
	}
	// The checks are made on the open file, so what is checked is what is read.
	if !os.SameFile(before, st) || !safe(st) {
		unsafe.Size = st.Size()
		return unsafe, nil, nil
	}
	o := evidence.Observation{Found: true, Safe: true, Size: st.Size()}
	if st.Size() > limit {
		return o, nil, nil
	}
	h := sha256.New()
	var buf bytes.Buffer
	var dst io.Writer = h
	if retain {
		dst = io.MultiWriter(h, &buf)
	}
	n, err := copyCtx(ctx, dst, io.LimitReader(f, limit+1))
	if err != nil {
		return evidence.Observation{}, nil, fmt.Errorf("evidencefile: read: %w", err)
	}
	o.Size = n
	if n > limit {
		return o, nil, nil
	}
	copy(o.Digest[:], h.Sum(nil))
	if retain {
		return o, buf.Bytes(), nil
	}
	return o, nil, nil
}

// List names every entry of the directory with its age. It reads the entries and
// follows none of them. The sweep decides what an entry is: a published file, a
// temporary file of an interrupted write, or something that does not belong here.
func (s *Store) List(ctx context.Context) ([]outbound.StoredFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d, err := s.root.Open(".")
	if err != nil {
		return nil, fmt.Errorf("evidencefile: open the directory: %w", err)
	}
	defer d.Close()
	entries, err := d.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("evidencefile: list the directory: %w", err)
	}
	out := make([]outbound.StoredFile, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue // removed while we read
		}
		if err != nil {
			return nil, fmt.Errorf("evidencefile: stat an entry: %w", err)
		}
		name := e.Name()
		out = append(out, outbound.StoredFile{
			Name: name, ModTime: info.ModTime(), Size: info.Size(), Special: !info.Mode().IsRegular(),
			IsKey: keyPattern.MatchString(name), Temporary: validTemporary(name),
		})
	}
	return out, nil
}

// validTemporary is the name that Put gives to a file that is not yet published.
func validTemporary(name string) bool {
	rest, ok := strings.CutPrefix(name, tmpPrefix)
	if !ok || rest == "" || len(rest) > 64 {
		return false
	}
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// RemoveTemporary deletes a temporary file of an interrupted write. It refuses
// every other name, so the sweep cannot remove evidence by a mistake.
func (s *Store) RemoveTemporary(ctx context.Context, name string) error {
	if !validTemporary(name) {
		return errors.New("evidencefile: not the name of a temporary file")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("evidencefile: remove a temporary file: %w", err)
	}
	return nil
}

// safe says that the file is a regular file of the running user, closed to the
// group and to others, with no special bit and a single link.
func safe(st fs.FileInfo) bool {
	if !st.Mode().IsRegular() || st.Mode().Perm()&0o077 != 0 || st.Mode()&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky) != 0 {
		return false
	}
	if uid, ok := posix.Owner(st); !ok || uid != posix.CurrentUser() {
		return false
	}
	n, ok := posix.LinkCount(st)
	return ok && n == 1
}

func copyCtx(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, chunk)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
		}
		if errors.Is(rerr, io.EOF) {
			return total, nil
		}
		if rerr != nil {
			return total, rerr
		}
	}
}
