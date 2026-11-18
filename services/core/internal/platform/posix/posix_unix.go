//go:build unix

// Package posix holds the file-system facts that need an operating system call
// and that the recovery marker depends on: an exclusive file lock and the owner
// of a file. The MVP supports Linux (structure.md section 3). The build tag is
// unix because Linux and macOS share these calls. Windows gets its own
// implementation by an approved decision. Until then the package does not
// build there, which is the honest state of the platform support.
package posix

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// NoFollow is the open flag that refuses a symlink in the last path element.
const NoFollow = syscall.O_NOFOLLOW

// NonBlock is the open flag that keeps the open of a named pipe from waiting for a writer.
const NonBlock = syscall.O_NONBLOCK

// LockFile takes an exclusive advisory lock (flock) on the file at path, which
// it creates with mode 0600 when it does not exist. It blocks until the lock is
// held. The returned function releases the lock and closes the file. The lock
// belongs to the open file, so two callers in one process exclude each other.
func LockFile(path string) (unlock func() error, err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|NoFollow, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock: %w", err)
	}
	return func() error {
		// Closing the file releases the lock.
		return f.Close()
	}, nil
}

// Owner returns the numeric owner of the file that fi describes.
func Owner(fi fs.FileInfo) (uid uint32, ok bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Uid, true
}

// CurrentUser returns the numeric user of the running process.
func CurrentUser() uint32 { return uint32(os.Getuid()) }
