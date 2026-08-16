//go:build unix

package jsonl

import (
	"io/fs"
	"syscall"
)

type fileID struct {
	dev uint64
	ino uint64
}

func identity(info fs.FileInfo) fileID {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileID{}
	}
	return fileID{dev: uint64(st.Dev), ino: uint64(st.Ino)}
}
