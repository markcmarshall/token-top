package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/markcmarshall/token-top/engine"
	"github.com/markcmarshall/token-top/telemetry"
)

func testSnap() engine.Snapshot {
	now := time.Date(2026, 8, 16, 20, 32, 14, 0, time.UTC)
	cache := 0.78
	return engine.Snapshot{
		GeneratedAt: now,
		Global: engine.Global{
			Rate1m:  337000,
			Rate5m:  221000,
			Rate15m: 190000,
			Today:   18_400_000,
			Burning: 2,
			Recent:  1,
		},
		QuietHidden: 6,
		Sources: []engine.SourceSnapshot{
			{Name: telemetry.SourceClaude, Health: telemetry.SourceHealth{Source: telemetry.SourceClaude, State: telemetry.HealthOK}, Rate1m: 92000, Share1m: 0.27},
			{Name: telemetry.SourceCodex, Health: telemetry.SourceHealth{Source: telemetry.SourceCodex, State: telemetry.HealthOK}, Rate1m: 245000, Share1m: 0.73},
			{Name: telemetry.SourceGrok, Health: telemetry.SourceHealth{Source: telemetry.SourceGrok, State: telemetry.HealthOK}, Rate1m: 0, Share1m: 0},
		},
		Sessions: []engine.Session{
			{
				Source: telemetry.SourceCodex, SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
				Activity: engine.ActivityBurning, Model: "gpt-5.6-sol", ProjectLabel: "acme",
				Rate1m: 184000, Rate5m: 121000, Rate15m: 90000, Total: 4_200_000, Input: 3_000_000, Output: 1_200_000,
				CacheRatio: &cache, FirstEvent: now.Add(-2 * time.Hour), LastEvent: now.Add(-2 * time.Second),
			},
			{
				Source: telemetry.SourceClaude, SessionID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
				Activity: engine.ActivityBurning, Model: "claude-opus-4.8", ProjectLabel: "payments",
				Rate1m: 92000, Rate5m: 76000, Rate15m: 40000, Total: 2_700_000,
				LastEvent: now.Add(-5 * time.Second), FirstEvent: now.Add(-40 * time.Minute),
			},
			{
				Source: telemetry.SourceGrok, SessionID: "cccccccc-cccc-cccc-cccc-cccccccccccc",
				Activity: engine.ActivityRecent, Model: "grok-4.6", ProjectLabel: "website",
				Rate1m: 0, Rate5m: 18000, Rate15m: 12000, Total: 1_100_000,
				LastEvent: now.Add(-54 * time.Second), FirstEvent: now.Add(-3 * time.Hour),
			},
		},
	}
}

func renderAt(t *testing.T, width, height int, color bool) string {
	t.Helper()
	snap := testSnap()
	return Render(snap, Options{
		Width:    width,
		Height:   height,
		Now:      snap.GeneratedAt,
		Color:    color,
		Interval: 2 * time.Second,
	})
}

func TestRenderContainsRegions(t *testing.T) {
	out := renderAt(t, 100, 24, false)
	for _, want := range []string{"TOKEN TOP", "BURN", "ACTIVE", "CLAUDE", "CODEX", "GROK", "acme", "trailing completed usage", "q quit"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
}

func TestRenderNarrowDropsModel(t *testing.T) {
	out := renderAt(t, 70, 24, false)
	if strings.Contains(out, "gpt-5.6-sol") {
		t.Fatalf("narrow still has model\n%s", out)
	}
	if !strings.Contains(out, "cdx") {
		t.Fatalf("narrow missing abbreviated harness\n%s", out)
	}
}

func TestRenderWideHasExtraColumns(t *testing.T) {
	out := renderAt(t, 160, 24, false)
	for _, want := range []string{"15M", "IN", "OUT", "LAST", "AGE", "SID", "aaaaaaaa"} {
		if !strings.Contains(out, want) {
			t.Fatalf("wide missing %q in\n%s", want, out)
		}
	}
}

func TestRenderShortOverflow(t *testing.T) {
	out := renderAt(t, 100, 12, false)
	if !strings.Contains(out, "+") || !strings.Contains(out, "more recent") {
		t.Fatalf("expected overflow\n%s", out)
	}
}

func TestRenderEmptyState(t *testing.T) {
	now := time.Date(2026, 8, 16, 20, 32, 14, 0, time.UTC)
	out := Render(engine.Snapshot{
		GeneratedAt: now,
		Sources: []engine.SourceSnapshot{
			{Name: telemetry.SourceClaude, Health: telemetry.SourceHealth{State: telemetry.HealthNotDetected}},
			{Name: telemetry.SourceCodex, Health: telemetry.SourceHealth{State: telemetry.HealthNotDetected}},
			{Name: telemetry.SourceGrok, Health: telemetry.SourceHealth{State: telemetry.HealthNotDetected}},
		},
	}, Options{Width: 80, Height: 24, Now: now, Interval: 2 * time.Second})
	if !strings.Contains(out, "0 burning") || !strings.Contains(out, "TOKEN TOP") {
		t.Fatalf("%s", out)
	}
}

func TestRenderDegradedSourceExpands(t *testing.T) {
	snap := testSnap()
	snap.Sources[2].Health.State = telemetry.HealthDegraded
	snap.Sources[2].Health.Detail = "telemetry incomplete"
	out := Render(snap, Options{Width: 100, Height: 24, Now: snap.GeneratedAt, Interval: 2 * time.Second})
	if !strings.Contains(out, "GROK ! telemetry incomplete") {
		t.Fatalf("%s", out)
	}
}

func TestRenderNoColorHasNoANSI(t *testing.T) {
	out := renderAt(t, 100, 24, false)
	if strings.ContainsRune(out, '\x1b') {
		t.Fatalf("ansi in no-color output")
	}
}

func TestRenderLongLabelsTruncate(t *testing.T) {
	snap := testSnap()
	snap.Sessions[0].ProjectLabel = "AVeryLongProjectNameThatShouldTruncate"
	snap.Sessions[0].Model = "an-unreasonably-long-model-identifier"
	out := Render(snap, Options{Width: 100, Height: 24, Now: snap.GeneratedAt, Interval: 2 * time.Second})
	if strings.Contains(out, "AVeryLongProjectNameThatShouldTruncate") {
		t.Fatal("project not truncated")
	}
	if !strings.Contains(out, "…") {
		t.Fatalf("expected ellipsis\n%s", out)
	}
}

func TestRenderFitsWidth(t *testing.T) {
	for _, w := range []int{60, 80, 100, 160} {
		out := renderAt(t, w, 24, false)
		for i, line := range strings.Split(out, "\n") {
			if lipgloss.Width(line) > w {
				t.Fatalf("width %d line %d is %d: %q", w, i, lipgloss.Width(line), line)
			}
		}
	}
}

func TestRenderIndexingVisible(t *testing.T) {
	snap := testSnap()
	snap.Sources[1].Health.Indexing = true
	out := Render(snap, Options{Width: 120, Height: 24, Now: snap.GeneratedAt, Interval: 2 * time.Second})
	if !strings.Contains(out, "indexing") {
		t.Fatalf("%s", out)
	}
	if strings.Contains(out, "CODEX !") && strings.Contains(out, "indexing") && strings.Contains(out, "CODEX ! indexing") {
		t.Fatal("indexing should not look like failure")
	}
}

func TestRenderTodayApprox(t *testing.T) {
	snap := testSnap()
	snap.Global.TodayApprox = true
	out := Render(snap, Options{Width: 100, Height: 24, Now: snap.GeneratedAt, Interval: 2 * time.Second})
	if !strings.Contains(out, "TODAY ~") {
		t.Fatalf("%s", out)
	}
}

func TestBarSharesDoesNotFillZeroShare(t *testing.T) {
	got := barShares([]float64{0.27, 0.73, 0}, 48)
	if got[2] != 0 {
		t.Fatalf("zero share got %d: %v", got[2], got)
	}
	sum := got[0] + got[1] + got[2]
	if sum > 48 {
		t.Fatalf("overflow %d", sum)
	}
	if got[0] == 0 || got[1] == 0 {
		t.Fatalf("positive shares collapsed %v", got)
	}
}

func TestRenderColorHasANSI(t *testing.T) {
	out := renderAt(t, 100, 24, true)
	if !strings.ContainsRune(out, '\x1b') {
		t.Fatal("expected color codes")
	}
	if !strings.Contains(out, "TOKEN TOP") {
		t.Fatal("color hid title")
	}
}
