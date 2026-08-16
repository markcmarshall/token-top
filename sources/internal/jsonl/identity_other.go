//go:build !unix

package jsonl

import "io/fs"

type fileID struct {
	dev uint64
	ino uint64
}

func identity(info fs.FileInfo) fileID {
	return fileID{ino: uint64(info.Size()) ^ uint64(info.ModTime().UnixNano())}
}
