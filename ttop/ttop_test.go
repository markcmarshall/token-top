package ttop

import (
	"context"
	"testing"
	"time"

	"github.com/markcmarshall/token-top/attribution"
	"github.com/markcmarshall/token-top/engine"
	"github.com/markcmarshall/token-top/telemetry"
)

func TestInjectedAttributorIsUsed(t *testing.T) {
	clk := &engine.FixedClock{T: time.Date(2026, 8, 16, 20, 32, 14, 0, time.Local)}
	eng := New(Options{
		Clock: clk,
		Attributor: attribution.Func(func(e telemetry.TokenEvent) attribution.Attribution {
			return attribution.Attribution{Key: "forced", Label: "Forced Label", Method: "test"}
		}),
	})
	eng.Apply(telemetry.Batch{
		Events: []telemetry.TokenEvent{{
			ID: "1", Source: telemetry.SourceClaude, SessionID: "s",
			Timestamp: clk.Now(), Model: "m", CWD: "/work/FounderOS",
			Input: 4, Output: 1, Complete: true,
		}},
		Health: telemetry.SourceHealth{Source: telemetry.SourceClaude, State: telemetry.HealthOK},
	})
	row := eng.Snapshot().Sessions[0]
	if row.ProjectLabel != "Forced Label" || row.ProjectMethod != "test" {
		t.Fatalf("%+v", row)
	}
}

func TestSourcesOrder(t *testing.T) {
	src := Sources()
	if len(src) != 3 {
		t.Fatalf("len %d", len(src))
	}
	if src[0].Name() != telemetry.SourceClaude || src[1].Name() != telemetry.SourceCodex || src[2].Name() != telemetry.SourceGrok {
		t.Fatalf("%s %s %s", src[0].Name(), src[1].Name(), src[2].Name())
	}
}

func TestSnapshotDoesNotPanic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	snap := Snapshot(ctx, Options{})
	if len(snap.Sources) != 3 {
		t.Fatalf("sources %d", len(snap.Sources))
	}
}
