package attribution

import (
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
