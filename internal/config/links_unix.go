//go:build unix

package config

import (
	"io/fs"
	"syscall"
)

// linkCount is the number of hard links to the file info describes.
func linkCount(info fs.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink)
	}
	return 1
}
