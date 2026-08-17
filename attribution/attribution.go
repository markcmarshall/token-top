package attribution

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/markcmarshall/token-top/telemetry"
)

type Attributor interface {
	Attribute(telemetry.TokenEvent) Attribution
}

type Attribution struct {
	Key    string
	Label  string
	Method string
}

type Func func(telemetry.TokenEvent) Attribution

func (f Func) Attribute(e telemetry.TokenEvent) Attribution {
	return f(e)
}

// GitRoot walks up from the event CWD to a git root, including worktree
// `.git` files. If none is found it falls back to CWDBasename.
func GitRoot(e telemetry.TokenEvent) Attribution {
	cleaned := path.Clean(strings.TrimSpace(e.CWD))
	if cleaned == "." || cleaned == "/" || cleaned == "" {
		return CWDBasename(e)
	}
	dir := cleaned
	if !filepath.IsAbs(dir) {
		dir = cleaned
	}
	for {
		info, err := os.Lstat(filepath.Join(dir, ".git"))
		if err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			label := path.Base(filepath.Clean(dir))
			if label == "." || label == "/" || label == "" {
				label = "unknown"
			}
			return Attribution{Key: filepath.Clean(dir), Label: label, Method: "git"}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return CWDBasename(e)
}

// CWDBasename labels an event by the last path element of its CWD.
func CWDBasename(e telemetry.TokenEvent) Attribution {
	cleaned := path.Clean(strings.TrimSpace(e.CWD))
	label := path.Base(cleaned)
	if label == "." || label == "/" || label == "" {
		label = "unknown"
	}
	key := cleaned
	if key == "." {
		key = ""
	}
	return Attribution{Key: key, Label: label, Method: "cwd"}
}
