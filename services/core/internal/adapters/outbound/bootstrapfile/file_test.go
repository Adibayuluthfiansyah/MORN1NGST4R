package bootstrapfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/bootstrapfile"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/platform/posix"
)

func memoryDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/dev/shm", "ms-bootstrap-")
	if err != nil {
		t.Skipf("no memory-backed directory to test with: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if ok, err := posix.OnMemoryFilesystem(dir); err != nil || !ok {
		t.Skipf("%s is not on tmpfs here", dir)
	}
	return dir
}

func diskDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", "disk-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(abs, 0o700); err != nil {
		t.Fatal(err)
	}
	if ok, _ := posix.OnMemoryFilesystem(abs); ok {
		t.Skip("the working directory is on tmpfs here, so there is no disk location to test the refusal with")
	}
	return abs
}

func TestWriteCreatesAnOwnerOnlyFileOnTmpfs(t *testing.T) {
	rt := memoryDir(t)
	f := bootstrapfile.New(rt)
	if err := f.Write("the-secret"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(rt, "morningstar", "bootstrap-secret")
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "the-secret\n" {
		t.Fatalf("content %q, %v", got, err)
	}
	for p, want := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
		st, _ := os.Stat(p)
		if st.Mode().Perm() != want {
			t.Errorf("%s mode %o, want %o", p, st.Mode().Perm(), want)
		}
	}
	if err := f.Write("second"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "second\n" {
		t.Errorf("an older secret survived: %q", got)
	}
	if err := f.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the file is still there")
	}
	if err := f.Remove(); err != nil {
		t.Errorf("a second Remove failed: %v", err)
	}
}

// D-68: account creation is refused when the location is not memory backed or
// the variable is unset.
func TestWriteRefusesALocationThatIsNotMemoryBacked(t *testing.T) {
	disk := diskDir(t)
	err := bootstrapfile.New(disk).Write("secret")
	if !errors.Is(err, bootstrapfile.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
	if _, err := os.Stat(filepath.Join(disk, "morningstar", "bootstrap-secret")); !os.IsNotExist(err) {
		t.Error("the secret reached the disk")
	}
	for _, rt := range []string{"", "relative/dir", filepath.Join(t.TempDir(), "missing")} {
		if err := bootstrapfile.New(rt).Write("secret"); !errors.Is(err, bootstrapfile.ErrUnavailable) {
			t.Errorf("runtime dir %q: error = %v, want ErrUnavailable", rt, err)
		}
	}
}

func TestWriteRefusesOpenOrForeignDirectories(t *testing.T) {
	rt := memoryDir(t)
	if err := os.Chmod(rt, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := bootstrapfile.New(rt).Write("s"); !errors.Is(err, bootstrapfile.ErrUnavailable) {
		t.Errorf("a world-writable runtime directory: %v", err)
	}
	if err := os.Chmod(rt, 0o700); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(rt, "morningstar")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := bootstrapfile.New(rt).Write("s"); !errors.Is(err, bootstrapfile.ErrUnavailable) {
		t.Errorf("a secret directory with mode 755: %v", err)
	}
}

func TestWriteRefusesASymlinkedDirectoryAndAPlantedFile(t *testing.T) {
	rt := memoryDir(t)
	target := filepath.Join(rt, "elsewhere")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(rt, "morningstar")); err != nil {
		t.Fatal(err)
	}
	if err := bootstrapfile.New(rt).Write("s"); !errors.Is(err, bootstrapfile.ErrUnavailable) {
		t.Errorf("a symlinked directory: %v", err)
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Error("the secret was written through the symlink")
	}
}

func TestRemoveWithoutARuntimeDirIsHarmless(t *testing.T) {
	if err := bootstrapfile.New("").Remove(); err != nil {
		t.Error(err)
	}
}
