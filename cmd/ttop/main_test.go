package main

import (
	"bytes"
	"runtime/debug"
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if got := stdout.String(); got != "dev\n" {
		t.Fatalf("version %q", got)
	}
}

func TestVersionFlagUsesModuleVersionForGoInstall(t *testing.T) {
	originalVersion, originalReadBuildInfo := version, readBuildInfo
	t.Cleanup(func() {
		version = originalVersion
		readBuildInfo = originalReadBuildInfo
	})
	version = "dev"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, true
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if got := stdout.String(); got != "v1.2.3\n" {
		t.Fatalf("version %q", got)
	}
}

func TestVersionFlagPrefersReleaseLinkerVersion(t *testing.T) {
	originalVersion, originalReadBuildInfo := version, readBuildInfo
	t.Cleanup(func() {
		version = originalVersion
		readBuildInfo = originalReadBuildInfo
	})
	version = "v2.0.0"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, true
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if got := stdout.String(); got != "v2.0.0\n" {
		t.Fatalf("version %q", got)
	}
}

func TestHelpFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "usage: ttop") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--wat"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestOnceSnapshot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--once"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "TOKEN TOP") {
		t.Fatalf("stdout %q", stdout.String())
	}
	if strings.ContainsRune(stdout.String(), '\x1b') {
		t.Fatal("non-tty --once emitted ANSI")
	}
}

func TestNonTTYSnapshotWithoutOnce(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "TOKEN TOP") {
		t.Fatalf("stdout %q", stdout.String())
	}
	if strings.ContainsRune(stdout.String(), '\x1b') {
		t.Fatal("non-tty snapshot emitted ANSI")
	}
}

func TestUnexpectedArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"extra"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
}
