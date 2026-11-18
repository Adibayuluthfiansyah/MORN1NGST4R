//go:build unix

package posix_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/platform/posix"
)

func TestLinkCountSeesAHardLink(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	if err := os.WriteFile(a, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	count := func() uint64 {
		fi, err := os.Lstat(a)
		if err != nil {
			t.Fatal(err)
		}
		n, ok := posix.LinkCount(fi)
		if !ok {
			t.Fatal("no link count")
		}
		return n
	}
	if n := count(); n != 1 {
		t.Fatalf("link count = %d, want 1", n)
	}
	if err := os.Link(a, filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 2 {
		t.Fatalf("link count after a hard link = %d, want 2", n)
	}
}
