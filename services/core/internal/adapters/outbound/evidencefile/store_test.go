//go:build unix

package evidencefile_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/evidencefile"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

const (
	keyA     = "0198a2b4-1111-7222-8333-444455556666"
	keyB     = "0198a2b4-2222-7222-8333-444455556666"
	maxBytes = 1 << 10
)

func newStore(t *testing.T) (*evidencefile.Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "evidence")
	s, err := evidencefile.New(dir, maxBytes)
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

// itemFor is the metadata that a capture of content would have stored.
func itemFor(t *testing.T, content []byte) evidence.Item {
	t.Helper()
	it, err := evidence.New(evidence.NewItem{
		ID: keyA, ProgramID: keyB, AssessmentID: keyB, Kind: evidence.KindNote, ContentType: "text/plain", Origin: evidence.HumanEntered,
		Redaction: evidence.Redacted, SHA256: evidence.DigestOf(content), Size: int64(len(content)),
		CapturedAt: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

func TestPutPublishesAnOwnerOnlyFileAndReportsWhatItWrote(t *testing.T) {
	s, dir := newStore(t)
	content := []byte("the evidence")
	w, err := s.Put(context.Background(), keyA, content)
	if err != nil {
		t.Fatal(err)
	}
	if w.Size != int64(len(content)) || w.Digest != evidence.DigestOf(content) {
		t.Fatalf("written = %+v", w)
	}
	fi, err := os.Lstat(filepath.Join(dir, keyA))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode() != 0o600 {
		t.Errorf("mode = %v, want -rw-------", fi.Mode())
	}
	if got := entries(t, dir); len(got) != 1 || got[0] != keyA {
		t.Errorf("directory = %v, want only the published file and no temporary one", got)
	}
	o, data, err := s.Read(context.Background(), keyA, int64(len(content)))
	if err != nil || !bytes.Equal(data, content) {
		t.Fatalf("read = %v, %q", err, data)
	}
	if got := evidence.Check(itemFor(t, content), o); got != evidence.ResultMatch {
		t.Errorf("check = %s, want match (%+v)", got, o)
	}
}

func TestAnExistingNameIsNeverReplaced(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Put(context.Background(), keyA, []byte("first")); err != nil {
		t.Fatal(err)
	}
	_, err := s.Put(context.Background(), keyA, []byte("second"))
	if !errors.Is(err, outbound.ErrEvidenceExists) {
		t.Fatalf("err = %v, want ErrEvidenceExists", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, keyA))
	if err != nil || string(got) != "first" {
		t.Fatalf("file = %q, %v: the first content was replaced", got, err)
	}
	if names := entries(t, dir); len(names) != 1 {
		t.Errorf("directory = %v: a refused write left a temporary file", names)
	}
}

func TestAFileThatIsNotOursIsNotReplacedEither(t *testing.T) {
	s, dir := newStore(t)
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Symlink(target, filepath.Join(dir, keyA)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(context.Background(), keyA, []byte("x")); !errors.Is(err, outbound.ErrEvidenceExists) {
		t.Fatalf("err = %v, want ErrEvidenceExists for a dangling symlink at the name", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the write went through the symlink: %v", err)
	}
}

func TestConcurrentWritersOfOneKeyHaveExactlyOneWinner(t *testing.T) {
	s, dir := newStore(t)
	const n = 16
	var wg sync.WaitGroup
	results := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = s.Put(context.Background(), keyA, bytes.Repeat([]byte{byte('a' + i)}, 64))
		}()
	}
	wg.Wait()
	wins := 0
	for _, err := range results {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, outbound.ErrEvidenceExists):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("%d writers won, want exactly one", wins)
	}
	if names := entries(t, dir); len(names) != 1 || names[0] != keyA {
		t.Errorf("directory = %v", names)
	}
	data, _ := os.ReadFile(filepath.Join(dir, keyA))
	if len(data) != 64 || len(bytes.Trim(data, string(data[:1]))) != 0 {
		t.Errorf("the published file mixes the writers: %q", data)
	}
}

func TestAnOverwriteAndASwapAreDetected(t *testing.T) {
	content := []byte("0123456789")
	it := itemFor(t, content)
	for name, tamper := range map[string]struct {
		do   func(t *testing.T, dir string)
		want evidence.CheckResult
	}{
		"overwrite in place, same size": {func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, keyA), []byte("9876543210"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, evidence.ResultHashMismatch},
		"overwrite in place, longer": {func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, keyA), []byte("0123456789!"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, evidence.ResultSizeMismatch},
		"append": {func(t *testing.T, dir string) {
			f, err := os.OpenFile(filepath.Join(dir, keyA), os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if _, err := f.WriteString("more"); err != nil {
				t.Fatal(err)
			}
		}, evidence.ResultSizeMismatch},
		"swap for another file of the same size": {func(t *testing.T, dir string) {
			other := filepath.Join(dir, ".swap")
			if err := os.WriteFile(other, []byte("abcdefghij"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(other, filepath.Join(dir, keyA)); err != nil {
				t.Fatal(err)
			}
		}, evidence.ResultHashMismatch},
		"removal": {func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, keyA)); err != nil {
				t.Fatal(err)
			}
		}, evidence.ResultMissing},
	} {
		t.Run(name, func(t *testing.T) {
			s, dir := newStore(t)
			if _, err := s.Put(context.Background(), keyA, content); err != nil {
				t.Fatal(err)
			}
			tamper.do(t, dir)
			o, err := s.Observe(context.Background(), keyA)
			if err != nil {
				t.Fatal(err)
			}
			if got := evidence.Check(it, o); got != tamper.want {
				t.Errorf("Observe: check = %s, want %s (%+v)", got, tamper.want, o)
			}
			o, _, err = s.Read(context.Background(), keyA, it.Size())
			if err != nil {
				t.Fatal(err)
			}
			if got := evidence.Check(it, o); got != tamper.want {
				t.Errorf("Read: check = %s, want %s (%+v)", got, tamper.want, o)
			}
		})
	}
}

func TestAFileThatIsNotAPlainOwnerOnlyFileIsNotSafe(t *testing.T) {
	content := []byte("0123456789")
	it := itemFor(t, content)
	for name, prepare := range map[string]func(t *testing.T, dir string){
		"symlink to a file with the right content": func(t *testing.T, dir string) {
			real := filepath.Join(dir, "real")
			if err := os.WriteFile(real, content, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("real", filepath.Join(dir, keyA)); err != nil {
				t.Fatal(err)
			}
		},
		"dangling symlink": func(t *testing.T, dir string) {
			if err := os.Symlink("nowhere", filepath.Join(dir, keyA)); err != nil {
				t.Fatal(err)
			}
		},
		"second hard link": func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, keyA), content, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(filepath.Join(dir, keyA), filepath.Join(dir, "alias")); err != nil {
				t.Fatal(err)
			}
		},
		"readable by the group": func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, keyA), content, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(filepath.Join(dir, keyA), 0o640); err != nil {
				t.Fatal(err)
			}
		},
		"writable by others": func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, keyA), content, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(filepath.Join(dir, keyA), 0o602); err != nil {
				t.Fatal(err)
			}
		},
		"a directory": func(t *testing.T, dir string) {
			if err := os.Mkdir(filepath.Join(dir, keyA), 0o700); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, dir := newStore(t)
			prepare(t, dir)
			o, err := s.Observe(context.Background(), keyA)
			if err != nil {
				t.Fatal(err)
			}
			if !o.Found || o.Safe {
				t.Fatalf("observation = %+v, want found and not safe", o)
			}
			if got := evidence.Check(it, o); got != evidence.ResultMetadataInconsistent {
				t.Errorf("check = %s, want metadata_inconsistent", got)
			}
			o, data, err := s.Read(context.Background(), keyA, it.Size())
			if err != nil || data != nil || o.Safe {
				t.Errorf("Read = %+v, %q, %v: an unsafe file must give no bytes", o, data, err)
			}
		})
	}
}

func TestAMissingFileIsAFindingAndNotAnError(t *testing.T) {
	s, _ := newStore(t)
	o, err := s.Observe(context.Background(), keyA)
	if err != nil || o.Found {
		t.Fatalf("Observe = %+v, %v", o, err)
	}
	o, data, err := s.Read(context.Background(), keyA, 5)
	if err != nil || o.Found || data != nil {
		t.Fatalf("Read = %+v, %q, %v", o, data, err)
	}
}

func TestAReadStopsAtTheExpectedSize(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Put(context.Background(), keyA, []byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	o, data, err := s.Read(context.Background(), keyA, 4)
	if err != nil || data != nil {
		t.Fatalf("Read = %+v, %q, %v: a file longer than the limit must not be read into memory", o, data, err)
	}
	if o.Size != 10 || !o.Found || !o.Safe {
		t.Errorf("observation = %+v, want the real size", o)
	}
	if got := evidence.Check(itemFor(t, []byte("0123")), o); got != evidence.ResultSizeMismatch {
		t.Errorf("check = %s, want size_mismatch", got)
	}
	if _, _, err := s.Read(context.Background(), keyA, maxBytes+1); !errors.Is(err, outbound.ErrEvidenceTooLarge) {
		t.Errorf("a limit above the store maximum: err = %v", err)
	}
}

func TestNoKeyCanLeaveTheDirectory(t *testing.T) {
	s, dir := newStore(t)
	outside := filepath.Join(filepath.Dir(dir), "outside")
	for _, key := range []string{
		"", ".", "..", "../outside", "../../etc/passwd", "a/b", "/etc/passwd", keyA + "/x", keyA + "\x00", strings.ToUpper(keyA),
		".tmp-0198a2b4", "0198a2b4", keyA + ".bak",
	} {
		if _, err := s.Put(context.Background(), key, []byte("x")); err == nil {
			t.Errorf("Put(%q) succeeded", key)
		}
		if _, err := s.Observe(context.Background(), key); err == nil {
			t.Errorf("Observe(%q) succeeded", key)
		}
		if _, _, err := s.Read(context.Background(), key, 1); err == nil {
			t.Errorf("Read(%q) succeeded", key)
		}
		if err := s.Remove(context.Background(), key); err == nil {
			t.Errorf("Remove(%q) succeeded", key)
		}
	}
	if _, err := os.Lstat(outside); err == nil {
		t.Error("a file was created outside the directory")
	}
	if names := entries(t, dir); len(names) != 0 {
		t.Errorf("directory = %v", names)
	}
}

func TestAContentOverTheLimitIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Put(context.Background(), keyA, make([]byte, maxBytes+1)); !errors.Is(err, outbound.ErrEvidenceTooLarge) {
		t.Fatalf("err = %v", err)
	}
	if names := entries(t, dir); len(names) != 0 {
		t.Errorf("directory = %v", names)
	}
	if _, err := s.Put(context.Background(), keyA, make([]byte, maxBytes)); err != nil {
		t.Errorf("content of exactly the limit was refused: %v", err)
	}
}

func TestAFailedWriteLeavesNothingBehind(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	s, dir := newStore(t)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o700) }()
	if _, err := s.Put(context.Background(), keyA, []byte("x")); err == nil {
		t.Fatal("a write into a read-only directory succeeded")
	}
	if names := entries(t, dir); len(names) != 0 {
		t.Errorf("directory = %v", names)
	}
}

func TestACancelledContextWritesNothing(t *testing.T) {
	s, dir := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Put(ctx, keyA, []byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if names := entries(t, dir); len(names) != 0 {
		t.Errorf("directory = %v", names)
	}
}

func TestRemoveDeletesTheFileAndForgivesAMissingOne(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Put(context.Background(), keyA, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(context.Background(), keyA); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(context.Background(), keyA); err != nil {
		t.Fatalf("a second removal: %v", err)
	}
	if names := entries(t, dir); len(names) != 0 {
		t.Errorf("directory = %v", names)
	}
}

func TestNewCreatesAPrivateDirectoryAndRefusesAnOpenOne(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "a", "evidence")
	if _, err := evidencefile.New(dir, maxBytes); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("directory = %v, %v", fi, err)
	}
	open := filepath.Join(base, "open")
	if err := os.Mkdir(open, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(open, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := evidencefile.New(open, maxBytes); err == nil {
		t.Error("a directory open to others was accepted")
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := evidencefile.New(link, maxBytes); err == nil {
		t.Error("a symlink to the directory was accepted")
	}
	for _, bad := range []string{"relative/dir", dir + "/../evidence", ""} {
		if _, err := evidencefile.New(bad, maxBytes); err == nil {
			t.Errorf("New(%q) succeeded", bad)
		}
	}
	if _, err := evidencefile.New(dir, 0); err == nil {
		t.Error("a limit of zero was accepted")
	}
}
