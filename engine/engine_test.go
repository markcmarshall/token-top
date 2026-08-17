package engine

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/markcmarshall/token-top/attribution"
	"github.com/markcmarshall/token-top/telemetry"
)

func testClock() *FixedClock {
	loc := time.FixedZone("test", -8*3600)
	return &FixedClock{T: time.Date(2026, 8, 16, 20, 32, 14, 0, loc)}
}

func newTestEngine(clk *FixedClock) *Engine {
	return New(clk, attribution.Func(attribution.CWDBasename))
}

func eventAt(id, session string, src telemetry.SourceName, at time.Time, in, out uint64) telemetry.TokenEvent {
	return telemetry.TokenEvent{
		ID:        id,
		Source:    src,
		SessionID: session,
		Timestamp: at,
		Model:     "gpt-test",
		CWD:       "/work/acme",
		Input:     in,
		Output:    out,
		Complete:  true,
	}
}

func applyOne(e *Engine, ev telemetry.TokenEvent) {
	e.Apply(telemetry.Batch{
		Events: []telemetry.TokenEvent{ev},
		Health: telemetry.SourceHealth{Source: ev.Source, State: telemetry.HealthOK},
	})
}

func TestEmptySnapshotHasAllSources(t *testing.T) {
	clk := testClock()
	snap := newTestEngine(clk).Snapshot()
	if len(snap.Sources) != 3 {
		t.Fatalf("sources %d", len(snap.Sources))
	}
	for i, name := range telemetry.AllSources {
		if snap.Sources[i].Name != name {
			t.Fatalf("order %s", snap.Sources[i].Name)
		}
		if snap.Sources[i].Health.State != telemetry.HealthNotDetected {
			t.Fatalf("%s state %s", name, snap.Sources[i].Health.State)
		}
	}
	if snap.Global.Today != 0 || len(snap.Sessions) != 0 {
		t.Fatal("expected empty activity")
	}
}

func TestRatesOnFirstSnapshot(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	applyOne(eng, eventAt("e1", "s1", telemetry.SourceCodex, clk.Now(), 300, 37))
	snap := eng.Snapshot()
	if snap.Global.Rate1m != 337 {
		t.Fatalf("1m %v", snap.Global.Rate1m)
	}
	if snap.Global.Rate5m != 337.0/5 {
		t.Fatalf("5m %v", snap.Global.Rate5m)
	}
	if snap.Global.Rate15m != 337.0/15 {
		t.Fatalf("15m %v", snap.Global.Rate15m)
	}
	if snap.Global.Today != 337 {
		t.Fatalf("today %d", snap.Global.Today)
	}
}

func TestWindowBoundaries(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	now := clk.Now()
	applyOne(eng, eventAt("in1", "s", telemetry.SourceClaude, now.Add(-Window1m), 10, 0))
	applyOne(eng, eventAt("out1", "s", telemetry.SourceClaude, now.Add(-Window1m-time.Nanosecond), 100, 0))
	applyOne(eng, eventAt("now", "s", telemetry.SourceClaude, now, 1, 0))
	applyOne(eng, eventAt("future", "s", telemetry.SourceClaude, now.Add(time.Nanosecond), 1000, 0))
	applyOne(eng, eventAt("in5", "s", telemetry.SourceClaude, now.Add(-Window5m), 7, 0))
	applyOne(eng, eventAt("out15", "s", telemetry.SourceClaude, now.Add(-Window15m-time.Nanosecond), 50, 0))

	snap := eng.Snapshot()
	if snap.Global.Rate1m != 11 { // 10 + 1
		t.Fatalf("1m %v", snap.Global.Rate1m)
	}
	if snap.Global.Rate5m != (10+100+1+7)/5.0 {
		t.Fatalf("5m %v", snap.Global.Rate5m)
	}
	if snap.Global.Rate15m != (10+100+1+7)/15.0 {
		t.Fatalf("15m %v", snap.Global.Rate15m)
	}
	if snap.Sessions[0].Total != 10+100+1+1000+7+50 {
		t.Fatalf("lifetime %d", snap.Sessions[0].Total)
	}
}

func TestDedupKeepsFirstEvent(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	first := eventAt("same", "s", telemetry.SourceClaude, clk.Now(), 4, 1)
	second := eventAt("same", "s", telemetry.SourceClaude, clk.Now(), 400, 100)
	applyOne(eng, first)
	applyOne(eng, second)
	snap := eng.Snapshot()
	if snap.Global.Today != 5 {
		t.Fatalf("today %d", snap.Global.Today)
	}
}

func TestActivityTransitions(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	applyOne(eng, eventAt("e", "s", telemetry.SourceGrok, clk.Now(), 2, 0))
	snap := eng.Snapshot()
	if snap.Global.Burning != 1 || snap.Sessions[0].Activity != ActivityBurning {
		t.Fatalf("burning %+v", snap)
	}

	clk.Advance(Window1m + time.Second)
	snap = eng.Snapshot()
	if snap.Global.Burning != 0 || snap.Global.Recent != 1 || snap.Sessions[0].Activity != ActivityRecent {
		t.Fatalf("recent %+v", snap)
	}

	clk.Advance(Window15m)
	snap = eng.Snapshot()
	if snap.Global.Recent != 0 || len(snap.Sessions) != 0 || snap.QuietHidden != 1 {
		t.Fatalf("quiet %+v", snap)
	}
}

func TestQuietHiddenIgnoresOlderDays(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	yesterday := clk.Now().Add(-24 * time.Hour)
	applyOne(eng, eventAt("old", "s", telemetry.SourceCodex, yesterday, 9, 0))
	snap := eng.Snapshot()
	if snap.QuietHidden != 0 || snap.Global.Today != 0 || len(snap.Sessions) != 0 {
		t.Fatalf("unexpected %+v", snap)
	}
}

func TestTodayCountsUTCEventsOnLocalDay(t *testing.T) {
	now := time.Date(2026, 8, 16, 20, 30, 0, 0, time.Local)
	clk := &FixedClock{T: now}
	eng := newTestEngine(clk)
	ev := eventAt("e", "s", telemetry.SourceClaude, now.UTC(), 10, 0)
	applyOne(eng, ev)
	if eng.Snapshot().Global.Today != 10 {
		t.Fatalf("today %d; local %s utc %s", eng.Snapshot().Global.Today, now, ev.Timestamp)
	}
}

func TestLocalDayRollover(t *testing.T) {
	loc := time.Local
	clk := &FixedClock{T: time.Date(2026, 8, 16, 23, 55, 0, 0, loc)}
	eng := newTestEngine(clk)
	applyOne(eng, eventAt("e", "s", telemetry.SourceClaude, clk.Now(), 80, 20))
	if eng.Snapshot().Global.Today != 100 {
		t.Fatal("pre-rollover")
	}

	clk.Set(time.Date(2026, 8, 17, 0, 5, 0, 0, loc))
	snap := eng.Snapshot()
	if snap.Global.Today != 0 {
		t.Fatalf("today after rollover %d", snap.Global.Today)
	}
	if snap.Global.Recent != 1 {
		t.Fatalf("event should still be recent: %+v", snap)
	}
	if snap.Global.Rate5m != 0 {
		t.Fatalf("5m %v", snap.Global.Rate5m)
	}
	if snap.Global.Rate15m != 100.0/15 {
		t.Fatalf("15m %v", snap.Global.Rate15m)
	}
}

func TestProjectFollowsLatestEvent(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	a := eventAt("a", "s", telemetry.SourceClaude, clk.Now().Add(-30*time.Second), 1, 0)
	a.CWD = "/work/alpha"
	b := eventAt("b", "s", telemetry.SourceClaude, clk.Now(), 1, 0)
	b.CWD = "/work/beta"
	applyOne(eng, b)
	applyOne(eng, a)
	row := eng.Snapshot().Sessions[0]
	if row.ProjectLabel != "beta" {
		t.Fatalf("project %q", row.ProjectLabel)
	}
	if row.FirstEvent != a.Timestamp || row.LastEvent != b.Timestamp {
		t.Fatalf("first/last %+v", row)
	}
}

func TestModelIsLatestEventAndCountIsDistinct(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	a := eventAt("a", "s", telemetry.SourceGrok, clk.Now().Add(-10*time.Second), 1, 0)
	a.Model = "grok-a"
	b := eventAt("b", "s", telemetry.SourceGrok, clk.Now(), 1, 0)
	b.Model = "grok-b"
	applyOne(eng, a)
	applyOne(eng, b)
	row := eng.Snapshot().Sessions[0]
	if row.Model != "grok-b" || row.ModelCount != 2 {
		t.Fatalf("model %+v", row)
	}
}

func TestCacheRatioUsesKnownReadsOnly(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	unknown := eventAt("u", "s", telemetry.SourceClaude, clk.Now().Add(-2*time.Second), 50, 0)
	known := eventAt("k", "s", telemetry.SourceClaude, clk.Now(), 100, 0)
	known.CacheRead = telemetry.Uint64Ptr(40)
	applyOne(eng, unknown)
	applyOne(eng, known)
	row := eng.Snapshot().Sessions[0]
	if row.CacheRatio == nil || *row.CacheRatio != 0.4 {
		t.Fatalf("ratio %+v", row.CacheRatio)
	}

	eng2 := newTestEngine(clk)
	applyOne(eng2, unknown)
	if eng2.Snapshot().Sessions[0].CacheRatio != nil {
		t.Fatal("unknown should stay blank")
	}

	zero := eventAt("z", "s2", telemetry.SourceClaude, clk.Now(), 8, 0)
	zero.CacheRead = telemetry.Uint64Ptr(0)
	applyOne(eng2, zero)
	var found Session
	var ok bool
	for _, row := range eng2.Snapshot().Sessions {
		if row.SessionID == "s2" {
			found = row
			ok = true
		}
	}
	if !ok || found.CacheRatio == nil || *found.CacheRatio != 0 {
		t.Fatalf("known zero %+v", found)
	}
}

func TestStableOrdering(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	now := clk.Now()
	// burning low 1m, should still beat recent high 5m
	burn := eventAt("b", "burn", telemetry.SourceCodex, now, 1, 0)
	recent := eventAt("r", "recent", telemetry.SourceCodex, now.Add(-2*time.Minute), 500, 0)
	// two burning with same 1m; higher 5m first, then lifetime, then id
	a := eventAt("a1", "aaa", telemetry.SourceClaude, now, 10, 0)
	aOld := eventAt("a0", "aaa", telemetry.SourceClaude, now.Add(-3*time.Minute), 1, 0)
	z := eventAt("z1", "zzz", telemetry.SourceClaude, now, 10, 0)
	for _, ev := range []telemetry.TokenEvent{z, recent, burn, a, aOld} {
		applyOne(eng, ev)
	}
	ids := make([]string, 0)
	for _, s := range eng.Snapshot().Sessions {
		ids = append(ids, s.SessionID)
	}
	want := []string{"aaa", "zzz", "burn", "recent"}
	if len(ids) != len(want) {
		t.Fatalf("ids %v", ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("order %v want %v", ids, want)
		}
	}
}

func TestSourceDegradationIsolation(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	applyOne(eng, eventAt("c", "s", telemetry.SourceClaude, clk.Now(), 9, 1))
	eng.Apply(telemetry.Batch{
		Health: telemetry.SourceHealth{Source: telemetry.SourceGrok, State: telemetry.HealthFailed, Detail: "unreadable"},
	})
	snap := eng.Snapshot()
	if snap.Global.Today != 10 || snap.Global.Burning != 1 {
		t.Fatalf("claude lost: %+v", snap.Global)
	}
	var grok, claude SourceSnapshot
	for _, s := range snap.Sources {
		switch s.Name {
		case telemetry.SourceGrok:
			grok = s
		case telemetry.SourceClaude:
			claude = s
		}
	}
	if grok.Health.State != telemetry.HealthFailed {
		t.Fatalf("grok %+v", grok.Health)
	}
	if claude.Health.State != telemetry.HealthOK {
		t.Fatalf("claude %+v", claude.Health)
	}
}

func TestIncompleteUsageDegradesButCounts(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	ev := eventAt("e", "s", telemetry.SourceGrok, clk.Now(), 3, 2)
	ev.Complete = false
	applyOne(eng, ev)
	snap := eng.Snapshot()
	if snap.Global.Today != 5 {
		t.Fatalf("today %d", snap.Global.Today)
	}
	if snap.Sources[2].Health.State != telemetry.HealthDegraded {
		t.Fatalf("health %+v", snap.Sources[2].Health)
	}
	applyOne(eng, eventAt("ok", "s2", telemetry.SourceGrok, clk.Now(), 1, 0))
	if eng.Snapshot().Sources[2].Health.State != telemetry.HealthDegraded {
		t.Fatal("incomplete health should stick")
	}
}

func TestInvalidEventDoesNotCount(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	bad := eventAt("bad", "s", telemetry.SourceCodex, clk.Now(), 10, 0)
	bad.CacheRead = telemetry.Uint64Ptr(11)
	applyOne(eng, bad)
	snap := eng.Snapshot()
	if snap.Global.Today != 0 || len(snap.Sessions) != 0 {
		t.Fatalf("accepted invalid: %+v", snap)
	}
	if snap.Sources[1].Health.State != telemetry.HealthDegraded {
		t.Fatalf("health %+v", snap.Sources[1].Health)
	}
}

func TestLifetimeOverflowRejected(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	first := eventAt("a", "s", telemetry.SourceCodex, clk.Now(), math.MaxUint64-5, 0)
	second := eventAt("b", "s", telemetry.SourceCodex, clk.Now(), 10, 0)
	applyOne(eng, first)
	applyOne(eng, second)
	snap := eng.Snapshot()
	if snap.Sessions[0].Total != math.MaxUint64-5 {
		t.Fatalf("total %d", snap.Sessions[0].Total)
	}
	if snap.Sources[1].Health.State != telemetry.HealthDegraded {
		t.Fatal("expected overflow degrade")
	}
}

func TestIndexingMarksLifetimeApproxNotRates(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	eng.Apply(telemetry.Batch{
		Events: []telemetry.TokenEvent{eventAt("e", "s", telemetry.SourceClaude, clk.Now(), 20, 0)},
		Health: telemetry.SourceHealth{Source: telemetry.SourceClaude, State: telemetry.HealthOK, Indexing: true, Detail: "indexing"},
	})
	snap := eng.Snapshot()
	if !snap.Sessions[0].TotalApprox {
		t.Fatal("expected approx total")
	}
	if snap.Global.Rate1m != 20 {
		t.Fatalf("rate %v", snap.Global.Rate1m)
	}
	if !snap.Sources[0].Health.Indexing {
		t.Fatal("expected indexing flag")
	}
}

func TestSnapshotIsImmutableCopy(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	applyOne(eng, eventAt("e", "s", telemetry.SourceClaude, clk.Now(), 1, 0))
	a := eng.Snapshot()
	a.Sessions[0].Total = 99
	a.Sources[0].Health.State = telemetry.HealthFailed
	b := eng.Snapshot()
	if b.Sessions[0].Total != 1 {
		t.Fatal("session mutated")
	}
	if b.Sources[0].Health.State != telemetry.HealthOK {
		t.Fatal("health mutated")
	}
}

type fakeSource struct {
	name  telemetry.SourceName
	batch telemetry.Batch
	boom  bool
	delay time.Duration
}

func (s fakeSource) Name() telemetry.SourceName { return s.name }

func (s fakeSource) Poll(ctx context.Context, _ time.Time) telemetry.Batch {
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return telemetry.Batch{Health: telemetry.SourceHealth{Source: s.name, State: telemetry.HealthFailed, Detail: "timeout"}}
		}
	}
	if s.boom {
		panic("boom")
	}
	return s.batch
}

func TestPollIsolatesSourcePanic(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	eng.Poll(context.Background(), []telemetry.Source{
		fakeSource{
			name: telemetry.SourceClaude,
			batch: telemetry.Batch{
				Events: []telemetry.TokenEvent{eventAt("c", "s", telemetry.SourceClaude, clk.Now(), 4, 0)},
				Health: telemetry.SourceHealth{Source: telemetry.SourceClaude, State: telemetry.HealthOK},
			},
		},
		fakeSource{name: telemetry.SourceCodex, boom: true},
	})
	snap := eng.Snapshot()
	if snap.Global.Today != 4 {
		t.Fatalf("claude dropped %d", snap.Global.Today)
	}
	if snap.Sources[1].Health.State != telemetry.HealthFailed {
		t.Fatalf("codex %+v", snap.Sources[1].Health)
	}
}

func TestPollSourcesInParallel(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	start := time.Now()
	eng.Poll(context.Background(), []telemetry.Source{
		fakeSource{
			name:  telemetry.SourceClaude,
			delay: 80 * time.Millisecond,
			batch: telemetry.Batch{
				Events: []telemetry.TokenEvent{eventAt("c", "s", telemetry.SourceClaude, clk.Now(), 1, 0)},
				Health: telemetry.SourceHealth{Source: telemetry.SourceClaude, State: telemetry.HealthOK},
			},
		},
		fakeSource{
			name:  telemetry.SourceCodex,
			delay: 80 * time.Millisecond,
			batch: telemetry.Batch{
				Events: []telemetry.TokenEvent{eventAt("x", "s", telemetry.SourceCodex, clk.Now(), 1, 0)},
				Health: telemetry.SourceHealth{Source: telemetry.SourceCodex, State: telemetry.HealthOK},
			},
		},
	})
	if time.Since(start) > 150*time.Millisecond {
		t.Fatalf("sources ran sequentially: %s", time.Since(start))
	}
	if eng.Snapshot().Global.Today != 2 {
		t.Fatalf("today %d", eng.Snapshot().Global.Today)
	}
}

func TestTodayApproxWhileIndexingToday(t *testing.T) {
	clk := testClock()
	eng := newTestEngine(clk)
	eng.Apply(telemetry.Batch{
		Events: []telemetry.TokenEvent{eventAt("e", "s", telemetry.SourceClaude, clk.Now(), 4, 0)},
		Health: telemetry.SourceHealth{Source: telemetry.SourceClaude, State: telemetry.HealthOK, Indexing: true, TodayIncomplete: true},
	})
	snap := eng.Snapshot()
	if !snap.Global.TodayApprox {
		t.Fatal("today should be approx")
	}
	if snap.Global.Today != 4 {
		t.Fatalf("today %d", snap.Global.Today)
	}
}
