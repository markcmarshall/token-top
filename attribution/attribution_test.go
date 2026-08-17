package attribution

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/markcmarshall/token-top/telemetry"
)

func TestCWDBasename(t *testing.T) {
	got := CWDBasename(telemetry.TokenEvent{CWD: "/work/FounderOS"})
	if got.Label != "FounderOS" || got.Method != "cwd" {
		t.Fatalf("%+v", got)
	}
	unknown := CWDBasename(telemetry.TokenEvent{CWD: ""})
	if unknown.Label != "unknown" {
		t.Fatalf("%+v", unknown)
	}
}

func TestGitRootUsesRepoBasename(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "FounderOS")
	sub := filepath.Join(repo, "cmd", "ttop")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := GitRoot(telemetry.TokenEvent{CWD: sub})
	if got.Label != "FounderOS" || got.Method != "git" {
		t.Fatalf("%+v", got)
	}
	if got.Key != repo {
		t.Fatalf("key %q want %q", got.Key, repo)
	}
}

func TestGitRootWorktreeFile(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "token-top")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: /tmp/worktrees/token-top\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := GitRoot(telemetry.TokenEvent{CWD: repo})
	if got.Label != "token-top" || got.Method != "git" {
		t.Fatalf("%+v", got)
	}
}

func TestGitRootFallsBackToCWD(t *testing.T) {
	got := GitRoot(telemetry.TokenEvent{CWD: "/tmp/no-git-here"})
	if got.Label != "no-git-here" || got.Method != "cwd" {
		t.Fatalf("%+v", got)
	}
}
