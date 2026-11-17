//go:build unix

package posix_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/platform/posix"
)

func TestLockFileExcludesASecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	unlock, err := posix.LockFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan struct{})
	go func() {
		u, err := posix.LockFile(path)
		if err != nil {
			t.Error(err)
			close(got)
			return
		}
		close(got)
		_ = u()
	}()
	select {
	case <-got:
		t.Fatal("the second lock was granted while the first was held")
	case <-time.After(150 * time.Millisecond):
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("the second lock was not granted after the release")
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("lock file mode = %o, want 600", st.Mode().Perm())
	}
}

func TestLockFileRefusesASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "x.lock")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := posix.LockFile(link); err == nil {
		t.Error("LockFile followed a symlink")
	}
}

func TestOwnerIsTheCurrentUser(t *testing.T) {
	st, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	uid, ok := posix.Owner(st)
	if !ok || uid != posix.CurrentUser() {
		t.Errorf("Owner = %d, %v, want %d", uid, ok, posix.CurrentUser())
	}
}
