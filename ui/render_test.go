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
			Rate1m:               337000,
			Rate5m:               221000,
			Rate15m:              190000,
			Tokens5m:             1_105_000,
			Tokens15m:            2_850_000,
			Today:                18_400_000,
			TodayInput:           17_200_000,
			TodayOutput:          1_200_000,
			TodayCacheRead:       13_416_000,
			TodayCacheKnownInput: 17_200_000,
			Burning:              2,
			Recent:               1,
		},
		QuietHidden: 6,
		Sources: []engine.SourceSnapshot{
			{Name: telemetry.SourceClaude, Health: telemetry.SourceHealth{Source: telemetry.SourceClaude, State: telemetry.HealthOK}, Rate1m: 92000, Share1m: 0.27, Tokens15m: 1_000_000, Today: 5_000_000, ShareToday: 5_000_000.0 / 18_400_000},
			{Name: telemetry.SourceCodex, Health: telemetry.SourceHealth{Source: telemetry.SourceCodex, State: telemetry.HealthOK}, Rate1m: 245000, Share1m: 0.73, Tokens15m: 1_800_000, Today: 12_300_000, ShareToday: 12_300_000.0 / 18_400_000},
			{Name: telemetry.SourceGrok, Health: telemetry.SourceHealth{Source: telemetry.SourceGrok, State: telemetry.HealthOK}, Rate1m: 0, Share1m: 0, Tokens15m: 50_000, Today: 1_100_000, ShareToday: 1_100_000.0 / 18_400_000},
		},
		Attributions: []engine.AttributionSnapshot{
			{Source: telemetry.SourceCodex, Key: "acme", Label: "acme", Tokens15m: 1_800_000, Today: 12_300_000, LastEvent: now.Add(-2 * time.Second)},
			{Source: telemetry.SourceClaude, Key: "payments", Label: "payments", Tokens15m: 1_000_000, Today: 5_000_000, LastEvent: now.Add(-5 * time.Second)},
			{Source: telemetry.SourceGrok, Key: "website", Label: "website", Tokens15m: 50_000, Today: 1_100_000, LastEvent: now.Add(-54 * time.Second)},
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
	for _, want := range []string{"TOKEN TOP", "TODAY", "INPUT", "OUTPUT", "RECENT", "HARNESS", "CLAUDE", "CODEX", "GROK", "PROJECT", "acme", "q quit"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
}

func TestRenderNarrowUsesAbbreviatedHarness(t *testing.T) {
	out := renderAt(t, 70, 24, false)
	if !strings.Contains(out, "CDX") || !strings.Contains(out, "cdx") {
		t.Fatalf("narrow missing abbreviated harness\n%s", out)
	}
}

func TestRenderWideKeepsLeanColumns(t *testing.T) {
	out := renderAt(t, 160, 24, false)
	for _, want := range []string{"TODAY SHARE", "TODAY", "15M", "LAST"} {
		if !strings.Contains(out, want) {
			t.Fatalf("wide missing %q in\n%s", want, out)
		}
	}
	for _, reject := range []string{"MODEL", "SID", "AGE", "quiet"} {
		if strings.Contains(out, reject) {
			t.Fatalf("wide retained %q in\n%s", reject, out)
		}
	}
}

func TestRenderShortOverflow(t *testing.T) {
	out := renderAt(t, 100, 12, false)
	if !strings.Contains(out, "+") || !strings.Contains(out, "more") {
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
	if !strings.Contains(out, "TODAY  0 processed") || !strings.Contains(out, "TOKEN TOP") {
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
	snap.Attributions[0].Label = "AVeryLongProjectNameThatShouldTruncate"
	out := Render(snap, Options{Width: 60, Height: 24, Now: snap.GeneratedAt, Interval: 2 * time.Second})
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

func TestRenderHealthyIndexingIsSilent(t *testing.T) {
	snap := testSnap()
	snap.Sources[1].Health.Indexing = true
	out := Render(snap, Options{Width: 120, Height: 24, Now: snap.GeneratedAt, Interval: 2 * time.Second})
	if strings.Contains(out, "indexing") {
		t.Fatalf("healthy indexing leaked into default UI\n%s", out)
	}
}

func TestRenderTodayApprox(t *testing.T) {
	snap := testSnap()
	snap.Global.TodayApprox = true
	out := Render(snap, Options{Width: 100, Height: 24, Now: snap.GeneratedAt, Interval: 2 * time.Second})
	if !strings.Contains(out, "TODAY  ~") {
		t.Fatalf("%s", out)
	}
}

func TestHarnessBarsHaveExactPercentages(t *testing.T) {
	out := renderAt(t, 100, 24, false)
	for _, want := range []string{"27.2%", "66.8%", "6.0%"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
}

func TestCacheBreakdownHandlesUnknownCoverage(t *testing.T) {
	snap := testSnap()
	snap.Global.TodayCacheKnownInput = 16_000_000
	out := Render(snap, Options{Width: 100, Height: 24, Now: snap.GeneratedAt})
	if !strings.Contains(out, "unknown") || !strings.Contains(out, "cached where known") {
		t.Fatalf("partial cache coverage presented as exact\n%s", out)
	}
}

func TestCacheBreakdownReportsUnavailableTelemetry(t *testing.T) {
	snap := testSnap()
	snap.Global.TodayCacheRead = 0
	snap.Global.TodayCacheKnownInput = 0
	out := Render(snap, Options{Width: 100, Height: 24, Now: snap.GeneratedAt})
	if !strings.Contains(out, "17.2M unknown") || !strings.Contains(out, "cache telemetry unavailable") {
		t.Fatalf("missing cache coverage presented as no breakdown\n%s", out)
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
