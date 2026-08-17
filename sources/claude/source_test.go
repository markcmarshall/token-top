package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/markcmarshall/token-top/engine"
	"github.com/markcmarshall/token-top/telemetry"
)

func writeCopy(t *testing.T, dst, srcName string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", srcName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPollNotDetected(t *testing.T) {
	src := New(Options{Home: t.TempDir()})
	batch := src.Poll(context.Background(), time.Now())
	if batch.Health.State != telemetry.HealthNotDetected {
		t.Fatalf("health %+v", batch.Health)
	}
}

func TestPollNormalAndEngine(t *testing.T) {
	root := t.TempDir()
	writeCopy(t, filepath.Join(root, "proj", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.jsonl"), "normal.jsonl")
	src := New(Options{Projects: root})
	now := time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC)
	batch := src.Poll(context.Background(), now)
	if batch.Health.State != telemetry.HealthOK || len(batch.Events) != 2 {
		t.Fatalf("batch events=%d health=%+v", len(batch.Events), batch.Health)
	}
	clk := &engine.FixedClock{T: now}
	eng := engine.New(clk, nil)
	eng.Apply(batch)
	snap := eng.Snapshot()
	if snap.Global.Today != 120 {
		t.Fatalf("today %d", snap.Global.Today)
	}
	if snap.Global.Burning != 1 || snap.Sessions[0].Source != telemetry.SourceClaude {
		t.Fatalf("snap %+v", snap)
	}
	if snap.Sessions[0].ProjectLabel != "FounderOS" {
		t.Fatalf("project %q", snap.Sessions[0].ProjectLabel)
	}
}

func TestPollDedupsAcrossFiles(t *testing.T) {
	root := t.TempDir()
	writeCopy(t, filepath.Join(root, "proj", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb.jsonl"), "duplicate.jsonl")
	writeCopy(t, filepath.Join(root, "proj", "agent-side.jsonl"), "duplicate.jsonl")
	src := New(Options{Projects: root})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC))
	if len(batch.Events) != 1 {
		t.Fatalf("events %d", len(batch.Events))
	}
}

func TestPollDegradesOnMalformed(t *testing.T) {
	root := t.TempDir()
	writeCopy(t, filepath.Join(root, "proj", "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee.jsonl"), "malformed.jsonl")
	src := New(Options{Projects: root})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC))
	if batch.Health.State != telemetry.HealthDegraded || len(batch.Events) != 2 {
		t.Fatalf("health=%+v events=%d", batch.Health, len(batch.Events))
	}
}

func TestPollIncrementalAppend(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "proj", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb.jsonl")
	writeCopy(t, path, "duplicate.jsonl")
	src := New(Options{Projects: root})
	now := time.Date(2026, 8, 16, 20, 0, 5, 0, time.UTC)
	first := src.Poll(context.Background(), now)
	if len(first.Events) != 1 {
		t.Fatalf("first %d", len(first.Events))
	}
	second := src.Poll(context.Background(), now)
	if len(second.Events) != 0 {
		t.Fatalf("replayed %d", len(second.Events))
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(`{"type":"assistant","timestamp":"2026-08-16T20:00:20.000Z","sessionId":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb","cwd":"/work/alpha","message":{"id":"msg_next","model":"claude-sonnet-5","usage":{"input_tokens":4,"output_tokens":2}}}` + "\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	third := src.Poll(context.Background(), now.Add(20*time.Second))
	if len(third.Events) != 1 || third.Events[0].Total() != 6 {
		t.Fatalf("append %+v", third.Events)
	}
}

func TestPollIndexingBudget(t *testing.T) {
	root := t.TempDir()
	writeCopy(t, filepath.Join(root, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.jsonl"), "normal.jsonl")
	writeCopy(t, filepath.Join(root, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb.jsonl"), "duplicate.jsonl")
	src := New(Options{Projects: root, ReadBudget: 180})
	now := time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC)
	var all []telemetry.TokenEvent
	sawIndex := false
	for i := 0; i < 32; i++ {
		batch := src.Poll(context.Background(), now)
		all = append(all, batch.Events...)
		if batch.Health.Indexing {
			sawIndex = true
		} else {
			break
		}
	}
	if !sawIndex {
		t.Fatal("expected a budget-limited indexing pass")
	}
	if len(all) != 3 {
		t.Fatalf("collected %d", len(all))
	}
}

func TestPollDefaultCorpusSmoke(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "projects")); err != nil {
		t.Skip("no local Claude projects")
	}
	src := New(Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	batch := src.Poll(ctx, time.Now())
	if batch.Health.State == telemetry.HealthNotDetected {
		t.Fatal("projects exist but source not detected")
	}
	for i := range batch.Events {
		if err := batch.Events[i].Validate(); err != nil {
			t.Fatalf("event %d: %v", i, err)
		}
	}
}

func TestPollTailFirstThenHistNoDoubleCount(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		f.WriteString(`{"type":"user","timestamp":"2026-08-16T19:00:00.000Z","sessionId":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","cwd":"/work/FounderOS","message":{"role":"user"}}` + "\n")
	}
	f.WriteString(`{"type":"assistant","timestamp":"2026-08-16T19:50:00.000Z","sessionId":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","cwd":"/work/FounderOS","message":{"id":"msg_old","model":"claude-opus-4.8","usage":{"input_tokens":100,"output_tokens":20}}}` + "\n")
	f.WriteString(`{"type":"assistant","timestamp":"2026-08-16T20:00:10.000Z","sessionId":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","cwd":"/work/FounderOS","message":{"id":"msg_new","model":"claude-opus-4.8","usage":{"input_tokens":5,"output_tokens":3}}}` + "\n")
	f.Close()
	src := New(Options{Projects: root, ReadBudget: 500, TailAfter: 250})
	now := time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC)
	var all []telemetry.TokenEvent
	for i := 0; i < 32; i++ {
		batch := src.Poll(context.Background(), now)
		all = append(all, batch.Events...)
		if !batch.Health.Indexing {
			break
		}
	}
	if len(all) != 2 {
		t.Fatalf("events %d", len(all))
	}
	seen := map[string]int{}
	for _, ev := range all {
		seen[ev.ID]++
		if ev.Validate() != nil {
			t.Fatalf("invalid %+v", ev)
		}
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("id %s counted %d", id, n)
		}
	}
}

func TestPollFileReplacementDoesNotDoubleCount(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.jsonl")
	writeCopy(t, path, "normal.jsonl")
	src := New(Options{Projects: root})
	now := time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC)
	first := src.Poll(context.Background(), now)
	if len(first.Events) != 2 {
		t.Fatalf("first %d", len(first.Events))
	}
	writeCopy(t, path, "duplicate.jsonl")
	second := src.Poll(context.Background(), now)
	if second.Health.State != telemetry.HealthDegraded {
		t.Fatalf("replace health %+v", second.Health)
	}
	if !strings.Contains(second.Health.Detail, "replaced") && !strings.Contains(second.Health.Detail, "malformed") && len(second.Events) > 2 {
		t.Fatalf("events %d health %+v", len(second.Events), second.Health)
	}
	clk := &engine.FixedClock{T: now}
	eng := engine.New(clk, nil)
	eng.Apply(first)
	eng.Apply(second)
	// stable IDs from the first file must not inflate after replacement
	if eng.Snapshot().Global.Today < 120 {
		t.Fatalf("lost first file %d", eng.Snapshot().Global.Today)
	}
}

func TestPollCWDChangeDoesNotAggregateByFile(t *testing.T) {
	root := t.TempDir()
	writeCopy(t, filepath.Join(root, "proj", "dddddddd-dddd-dddd-dddd-dddddddddddd.jsonl"), "cwd_change.jsonl")
	src := New(Options{Projects: root})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC))
	if len(batch.Events) != 2 {
		t.Fatalf("events %d", len(batch.Events))
	}
	if batch.Events[0].CWD == batch.Events[1].CWD {
		t.Fatal("cwd collapsed")
	}
}
