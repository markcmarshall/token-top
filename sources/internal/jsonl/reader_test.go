package jsonl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadIncrementalAndPartialLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.jsonl")
	if err := os.WriteFile(path, []byte("{\"n\":1}\n{\"n\":"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Read(path, Cursor{}, 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Lines) != 1 || string(res.Lines[0]) != `{"n":1}` {
		t.Fatalf("lines %#v", res.Lines)
	}
	if string(res.Cursor.Partial) != `{"n":` {
		t.Fatalf("partial %q", res.Cursor.Partial)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("2}\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	res, err = Read(path, res.Cursor, 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Lines) != 1 || string(res.Lines[0]) != `{"n":2}` {
		t.Fatalf("second %#v", res.Lines)
	}
	if !FullyConsumed(res.Cursor) {
		t.Fatal("expected consumed")
	}
}

func TestReadMidStartDropsPartialFirstLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.jsonl")
	if err := os.WriteFile(path, []byte("AAA\n{\"n\":1}\n{\"n\":2}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Read(path, Cursor{Offset: 1, MidStart: true}, 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Lines) != 2 || string(res.Lines[0]) != `{"n":1}` {
		t.Fatalf("lines %#v", res.Lines)
	}
}

func TestReadResetOnTruncate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.jsonl")
	if err := os.WriteFile(path, []byte("{\"n\":1}\n{\"n\":2}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Read(path, Cursor{}, 64)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"n\":9}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = Read(path, res.Cursor, 64)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Reset || !res.Truncated {
		t.Fatalf("reset=%v truncated=%v", res.Reset, res.Truncated)
	}
	if len(res.Lines) != 1 || string(res.Lines[0]) != `{"n":9}` {
		t.Fatalf("lines %#v", res.Lines)
	}
}
