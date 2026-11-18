// Package bootstrapfile writes the first-run bootstrap secret to a file on a
// memory-backed filesystem (D-68). It refuses any other location, so the secret
// never reaches a disk.
package bootstrapfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/platform/posix"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

const (
	dirName  = "morningstar"
	fileName = "bootstrap-secret"
)

var ErrUnavailable = errors.New("bootstrapfile: no memory-backed runtime directory is available")

type File struct {
	runtimeDir string
}

var _ outbound.BootstrapSecretFile = (*File)(nil)

// New returns a File under runtimeDir, normally $XDG_RUNTIME_DIR. An empty
// runtimeDir makes every Write fail with ErrUnavailable.
func New(runtimeDir string) *File { return &File{runtimeDir: runtimeDir} }

func (f *File) dir() (string, error) {
	if f.runtimeDir == "" || !filepath.IsAbs(f.runtimeDir) {
		return "", ErrUnavailable
	}
	if err := checkOwnedPrivate(f.runtimeDir, 0o022); err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	dir := filepath.Join(f.runtimeDir, dirName)
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if err := checkOwnedPrivate(dir, 0o077); err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	onMemory, err := posix.OnMemoryFilesystem(dir)
	if err != nil || !onMemory {
		return "", fmt.Errorf("%w: the directory is not on tmpfs or ramfs", ErrUnavailable)
	}
	return dir, nil
}

func checkOwnedPrivate(dir string, forbidden os.FileMode) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return errors.New("not a directory (a symlink is refused)")
	}
	if uid, ok := posix.Owner(fi); !ok || uid != posix.CurrentUser() {
		return errors.New("not owned by the running user")
	}
	if fi.Mode().Perm()&forbidden != 0 {
		return fmt.Errorf("mode %o is too open", fi.Mode().Perm())
	}
	return nil
}

// Write replaces any earlier secret file with one that holds secret.
func (f *File) Write(secret string) error {
	dir, err := f.dir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, fileName)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("bootstrapfile: %w", err)
	}
	fd, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|posix.NoFollow, 0o600)
	if err != nil {
		return fmt.Errorf("bootstrapfile: %w", err)
	}
	if _, err := fd.WriteString(secret + "\n"); err != nil {
		_ = fd.Close()
		_ = os.Remove(path)
		return fmt.Errorf("bootstrapfile: %w", err)
	}
	if err := fd.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("bootstrapfile: %w", err)
	}
	return nil
}

// Remove deletes the secret file. A missing file is not an error.
func (f *File) Remove() error {
	if f.runtimeDir == "" {
		return nil
	}
	err := os.Remove(filepath.Join(f.runtimeDir, dirName, fileName))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("bootstrapfile: %w", err)
	}
	return nil
}
