//go:build linux

package posix

import "syscall"

const (
	tmpfsMagic = 0x01021994
	ramfsMagic = 0x858458f6
)

// OnMemoryFilesystem reports whether path is on tmpfs or ramfs. A secret that
// must never reach a disk is written only where this is true (D-68).
func OnMemoryFilesystem(path string) (bool, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return false, err
	}
	t := int64(st.Type)
	return t == tmpfsMagic || t == ramfsMagic, nil
}
