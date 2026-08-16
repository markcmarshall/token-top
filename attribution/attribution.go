package attribution

import (
	"path"
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

// CWDBasename labels an event by the last path element of its CWD.
// The standalone git-root attributor replaces this in a later phase.
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
