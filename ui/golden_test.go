package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/markcmarshall/token-top/engine"
	"github.com/markcmarshall/token-top/telemetry"
)

func TestRenderGoldens(t *testing.T) {
	now := testSnap().GeneratedAt
	cases := []struct {
		name string
		w, h int
		snap engine.Snapshot
	}{
		{"narrow", 70, 24, testSnap()},
		{"standard", 80, 24, testSnap()},
		{"normal", 100, 24, testSnap()},
		{"wide", 160, 24, testSnap()},
		{"short", 100, 12, testSnap()},
		{"empty", 80, 24, engine.Snapshot{
			GeneratedAt: now,
			Sources: []engine.SourceSnapshot{
				{Name: telemetry.SourceClaude, Health: telemetry.SourceHealth{State: telemetry.HealthNotDetected}},
				{Name: telemetry.SourceCodex, Health: telemetry.SourceHealth{State: telemetry.HealthNotDetected}},
				{Name: telemetry.SourceGrok, Health: telemetry.SourceHealth{State: telemetry.HealthNotDetected}},
			},
		}},
		{"one-source", 100, 24, oneSourceSnap()},
		{"degraded", 100, 24, degradedSnap()},
		{"huge", 100, 24, hugeSnap()},
	}
	dir := filepath.Join("testdata", "golden")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	update := os.Getenv("UPDATE_GOLDEN") == "1"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Render(tc.snap, Options{Width: tc.w, Height: tc.h, Now: now, Interval: 2 * time.Second})
			path := filepath.Join(dir, tc.name+".golden")
			if update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden (set UPDATE_GOLDEN=1): %v", err)
			}
			if string(want) != got {
				t.Fatalf("golden mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}

func oneSourceSnap() engine.Snapshot {
	snap := testSnap()
	snap.Sources = []engine.SourceSnapshot{snap.Sources[1]}
	snap.Sources[0].Share1m = 1
	snap.Sources[0].ShareToday = 1
	snap.Sources[0].Today = snap.Global.Today
	snap.Sources[0].Tokens15m = snap.Global.Tokens15m
	snap.Sessions = snap.Sessions[:1]
	snap.Attributions = snap.Attributions[:1]
	snap.Attributions[0].Today = snap.Global.Today
	snap.Attributions[0].Tokens15m = snap.Global.Tokens15m
	snap.Global.Burning = 1
	snap.Global.Recent = 0
	return snap
}

func degradedSnap() engine.Snapshot {
	snap := testSnap()
	snap.Sources[2].Health.State = telemetry.HealthDegraded
	snap.Sources[2].Health.Detail = "telemetry incomplete"
	return snap
}

func hugeSnap() engine.Snapshot {
	snap := testSnap()
	snap.Global.Today = 18_400_000_000
	snap.Global.TodayInput = 17_200_000_000
	snap.Global.TodayOutput = 1_200_000_000
	snap.Global.TodayCacheRead = 13_416_000_000
	snap.Global.TodayCacheKnownInput = 17_200_000_000
	snap.Global.Tokens5m = 1_105_000_000
	snap.Global.Tokens15m = 2_850_000_000
	snap.Sources[0].Today = 5_000_000_000
	snap.Sources[1].Today = 12_300_000_000
	snap.Sources[2].Today = 1_100_000_000
	snap.Attributions[0].Today = 12_300_000_000
	snap.Sessions[0].Total = 9_900_000_000
	snap.Sessions[0].Rate1m = 12_300_000
	return snap
}

func TestRenderOneSource(t *testing.T) {
	out := Render(oneSourceSnap(), Options{Width: 100, Height: 24, Now: testSnap().GeneratedAt, Interval: 2 * time.Second})
	if !strings.Contains(out, "CODEX") || !strings.Contains(out, "acme") {
		t.Fatalf("%s", out)
	}
}

func TestRenderHugeValues(t *testing.T) {
	out := Render(hugeSnap(), Options{Width: 100, Height: 24, Now: testSnap().GeneratedAt, Interval: 2 * time.Second})
	if !strings.Contains(out, "M") {
		t.Fatalf("%s", out)
	}
}
