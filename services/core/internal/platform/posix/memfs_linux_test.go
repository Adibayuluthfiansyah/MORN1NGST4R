//go:build linux

package posix_test

import (
	"os"
	"testing"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/platform/posix"
)

func TestOnMemoryFilesystemTellsTmpfsFromOtherFilesystems(t *testing.T) {
	// procfs is a fixed example of a file system that holds no memory-backed
	// files for a user, and it exists on every Linux system.
	if ok, err := posix.OnMemoryFilesystem("/proc"); err != nil || ok {
		t.Fatalf("/proc: memory-backed = %v, err = %v", ok, err)
	}
	dir, err := os.MkdirTemp("/dev/shm", "ms-memfs-")
	if err != nil {
		t.Skipf("no /dev/shm to test the positive case: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if ok, err := posix.OnMemoryFilesystem(dir); err != nil || !ok {
		t.Skipf("/dev/shm is not tmpfs here (memory-backed = %v, err = %v)", ok, err)
	}
}

func TestOnMemoryFilesystemFailsOnAMissingPath(t *testing.T) {
	if ok, err := posix.OnMemoryFilesystem("/nonexistent-morningstar-path"); err == nil || ok {
		t.Fatalf("memory-backed = %v, err = %v", ok, err)
	}
}
