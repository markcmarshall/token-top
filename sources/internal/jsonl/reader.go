package jsonl

import (
	"bytes"
	"io"
	"os"
)

type Cursor struct {
	Offset   int64
	Size     int64
	Dev      uint64
	Ino      uint64
	Partial  []byte
	MidStart bool
}

type Result struct {
	Lines     [][]byte
	Cursor    Cursor
	Read      int64
	Reset     bool
	Truncated bool
}

func Read(path string, cur Cursor, budget int64) (Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return Result{Cursor: cur}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return Result{Cursor: cur}, err
	}
	id := identity(info)
	out := Result{Cursor: cur}
	if cur.Ino != 0 && (cur.Dev != id.dev || cur.Ino != id.ino) {
		out.Reset = true
		cur = Cursor{}
	}
	if cur.Offset > info.Size() {
		out.Truncated = true
		out.Reset = true
		cur = Cursor{}
	}

	if _, err := f.Seek(cur.Offset, io.SeekStart); err != nil {
		return Result{Cursor: cur}, err
	}

	if budget <= 0 {
		budget = 1 << 20
	}
	buf := make([]byte, budget)
	n, err := io.ReadFull(f, buf)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		err = nil
	}
	if err != nil {
		return Result{Cursor: cur}, err
	}
	buf = buf[:n]
	out.Read = int64(n)

	data := append(append([]byte{}, cur.Partial...), buf...)
	cur.Partial = nil
	start := 0
	if cur.MidStart {
		nl := bytes.IndexByte(data, '\n')
		if nl < 0 {
			cur.Offset += int64(n)
			cur.Size = info.Size()
			cur.Dev = id.dev
			cur.Ino = id.ino
			out.Cursor = cur
			return out, nil
		}
		start = nl + 1
		cur.MidStart = false
	}
	for i := start; i < len(data); i++ {
		if data[i] != '\n' {
			continue
		}
		line := data[start:i]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		cp := make([]byte, len(line))
		copy(cp, line)
		out.Lines = append(out.Lines, cp)
		start = i + 1
	}
	if start < len(data) {
		cur.Partial = append([]byte{}, data[start:]...)
	}
	cur.Offset += int64(n)
	cur.Size = info.Size()
	cur.Dev = id.dev
	cur.Ino = id.ino
	out.Cursor = cur
	return out, nil
}

func FullyConsumed(cur Cursor) bool {
	return cur.Offset >= cur.Size && len(cur.Partial) == 0
}
