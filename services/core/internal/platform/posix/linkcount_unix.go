//go:build unix

package posix

import (
	"io/fs"
	"syscall"
)

// LinkCount returns the number of hard links to the file that fi describes. A
// stored evidence file has exactly one: a second name is a way to change the
// content without touching the published name.
func LinkCount(fi fs.FileInfo) (n uint64, ok bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Nlink), true
}
