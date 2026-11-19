//go:build unix

package evidencefile_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/evidence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// TD-M6-01: the hash of a check reads at most what the row says. A file that a
// local process made huge is reported by its size and is not read.
func TestObserveReadsAtMostTheExpectedSize(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Put(context.Background(), keyA, []byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	o, err := s.Observe(context.Background(), keyA, 10)
	if err != nil || o.Size != 10 || o.Digest != evidence.DigestOf([]byte("0123456789")) {
		t.Fatalf("the plain case = %+v, %v", o, err)
	}
	for name, grow := range map[string]int64{"a little": 11, "a lot": 64 << 20} {
		if err := os.Truncate(filepath.Join(dir, keyA), grow); err != nil {
			t.Fatal(err)
		}
		o, err := s.Observe(context.Background(), keyA, 10)
		if err != nil || !o.Found || !o.Safe || o.Size != grow || o.Digest != (evidence.Digest{}) {
			t.Fatalf("%s: observation = %+v, %v: the file must be reported by its size without a hash", name, o, err)
		}
		if got := evidence.Check(itemFor(t, []byte("0123456789")), o); got != evidence.ResultSizeMismatch {
			t.Errorf("%s: check = %s", name, got)
		}
	}
}

func TestObserveRefusesALimitAboveTheStoreMaximum(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Observe(context.Background(), keyA, maxBytes+1); err == nil {
		t.Error("a limit above the maximum was accepted")
	}
	if _, err := s.Observe(context.Background(), keyA, -1); err == nil {
		t.Error("a negative limit was accepted")
	}
}

func TestListShowsEveryNameWithItsAgeAndNeverFollowsOne(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Put(context.Background(), keyA, []byte("x")); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	for _, name := range []string{".tmp-deadbeef", "stray"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("y"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filepath.Join(dir, name), old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	files, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	var names []string
	byName := map[string]outbound.StoredFile{}
	for _, f := range files {
		names = append(names, f.Name)
		byName[f.Name] = f
	}
	want := []string{".tmp-deadbeef", keyA, "link", "stray"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v, want %v", names, want)
		}
	}
	if !byName[keyA].IsKey || byName[keyA].Temporary || time.Since(byName[keyA].ModTime) > time.Minute {
		t.Errorf("the published file = %+v", byName[keyA])
	}
	if !byName[".tmp-deadbeef"].Temporary || byName[".tmp-deadbeef"].IsKey || time.Since(byName[".tmp-deadbeef"].ModTime) < 47*time.Hour {
		t.Errorf("the temporary file = %+v", byName[".tmp-deadbeef"])
	}
	if byName["stray"].IsKey || byName["stray"].Temporary || !byName["link"].Special {
		t.Errorf("stray = %+v, link = %+v", byName["stray"], byName["link"])
	}
}

func TestRemoveTemporaryDeletesOnlyATemporaryFile(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Put(context.Background(), keyA, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tmp-feed"), []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveTemporary(context.Background(), ".tmp-feed"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{keyA, "stray", "../x", ".tmp-../x", ".tmp-a/b", ".tmp-", ""} {
		if err := s.RemoveTemporary(context.Background(), name); err == nil {
			t.Errorf("RemoveTemporary(%q) was accepted", name)
		}
	}
	if names := entries(t, dir); len(names) != 1 || names[0] != keyA {
		t.Errorf("directory = %v", names)
	}
}
