package codex

import (
	"context"
	"os"
	"path/filepath"
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
	sessions := filepath.Join(root, "sessions")
	writeCopy(t, filepath.Join(sessions, "rollout-2026-08-16T20-00-00-11111111-1111-1111-1111-111111111111.jsonl"), "normal.jsonl")
	src := New(Options{Sessions: sessions, Archive: filepath.Join(root, "archived")})
	now := time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC)
	batch := src.Poll(context.Background(), now)
	if batch.Health.State != telemetry.HealthOK || len(batch.Events) != 2 {
		t.Fatalf("batch events=%d health=%+v", len(batch.Events), batch.Health)
	}
	clk := &engine.FixedClock{T: now}
	eng := engine.New(clk, nil)
	eng.Apply(batch)
	snap := eng.Snapshot()
	if snap.Global.Today != 165 {
		t.Fatalf("today %d", snap.Global.Today)
	}
	if snap.Global.Burning != 1 || snap.Sessions[0].Source != telemetry.SourceCodex {
		t.Fatalf("snap %+v", snap)
	}
	if snap.Sessions[0].ProjectLabel != "FounderOS" {
		t.Fatalf("project %q", snap.Sessions[0].ProjectLabel)
	}
}

func TestPollPrefersLiveOverArchive(t *testing.T) {
	root := t.TempDir()
	name := "rollout-2026-08-16T20-00-00-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.jsonl"
	writeCopy(t, filepath.Join(root, "sessions", name), "normal.jsonl")
	writeCopy(t, filepath.Join(root, "archived", name), "reset.jsonl")
	src := New(Options{
		Sessions: filepath.Join(root, "sessions"),
		Archive:  filepath.Join(root, "archived"),
	})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC))
	if len(batch.Events) != 2 || batch.Events[0].SessionID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("used archive? %+v", batch.Events)
	}
}

func TestPollIncludesArchiveOnlySession(t *testing.T) {
	root := t.TempDir()
	writeCopy(t, filepath.Join(root, "archived", "rollout-2026-08-16T20-00-00-33333333-3333-3333-3333-333333333333.jsonl"), "reset.jsonl")
	os.MkdirAll(filepath.Join(root, "sessions"), 0o755)
	src := New(Options{
		Sessions: filepath.Join(root, "sessions"),
		Archive:  filepath.Join(root, "archived"),
	})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 30, 0, time.UTC))
	if len(batch.Events) != 2 {
		t.Fatalf("events %d", len(batch.Events))
	}
}

func TestPollDegradesOnMalformed(t *testing.T) {
	root := t.TempDir()
	writeCopy(t, filepath.Join(root, "sessions", "rollout-2026-08-16T20-00-00-55555555-5555-5555-5555-555555555555.jsonl"), "malformed.jsonl")
	src := New(Options{Sessions: filepath.Join(root, "sessions"), Archive: filepath.Join(root, "missing")})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC))
	if batch.Health.State != telemetry.HealthDegraded || len(batch.Events) != 2 {
		t.Fatalf("health=%+v events=%d", batch.Health, len(batch.Events))
	}
}

func TestPollIncrementalAppend(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sessions", "rollout-2026-08-16T20-00-00-22222222-2222-2222-2222-222222222222.jsonl")
	writeCopy(t, path, "repeated_last.jsonl")
	src := New(Options{Sessions: filepath.Join(root, "sessions"), Archive: filepath.Join(root, "archived")})
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
	_, err = f.WriteString(`{"timestamp":"2026-08-16T20:00:20.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":15,"output_tokens":3,"total_tokens":18},"total_token_usage":{"input_tokens":95,"cached_input_tokens":10,"output_tokens":11,"reasoning_output_tokens":2,"total_tokens":106}}}}` + "\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	third := src.Poll(context.Background(), now.Add(20*time.Second))
	if len(third.Events) != 1 || third.Events[0].Total() != 18 {
		t.Fatalf("append %+v", third.Events)
	}
}

func TestPollIndexingBudget(t *testing.T) {
	root := t.TempDir()
	sessions := filepath.Join(root, "sessions")
	writeCopy(t, filepath.Join(sessions, "rollout-2026-08-16T20-00-00-11111111-1111-1111-1111-111111111111.jsonl"), "normal.jsonl")
	writeCopy(t, filepath.Join(sessions, "rollout-2026-08-16T19-00-00-22222222-2222-2222-2222-222222222222.jsonl"), "repeated_last.jsonl")
	src := New(Options{
		Sessions:   sessions,
		Archive:    filepath.Join(root, "archived"),
		ReadBudget: 200,
	})
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
	if _, err := os.Stat(filepath.Join(home, ".codex", "sessions")); err != nil {
		t.Skip("no local Codex sessions")
	}
	src := New(Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	batch := src.Poll(ctx, time.Now())
	if batch.Health.State == telemetry.HealthNotDetected {
		t.Fatal("sessions exist but source not detected")
	}
	for i := range batch.Events {
		if err := batch.Events[i].Validate(); err != nil {
			t.Fatalf("event %d: %v", i, err)
		}
	}
}

func TestPollTailFirstDoesNotBurstLifetime(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sessions", "rollout-2026-08-16T20-00-00-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 80; i++ {
		f.WriteString(`{"timestamp":"2026-08-16T19:00:00.000Z","type":"response_item","payload":{"type":"message","content":"REDACTED"}}` + "\n")
	}
	f.WriteString(`{"timestamp":"2026-08-16T20:00:03.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":5000,"output_tokens":20,"total_tokens":5020},"total_token_usage":{"input_tokens":5000,"output_tokens":20,"total_tokens":5020}}}}` + "\n")
	f.WriteString(`{"timestamp":"2026-08-16T20:00:10.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":30,"output_tokens":4,"total_tokens":34},"total_token_usage":{"input_tokens":5030,"output_tokens":24,"total_tokens":5054}}}}` + "\n")
	f.Close()
	src := New(Options{
		Sessions:   filepath.Join(root, "sessions"),
		Archive:    filepath.Join(root, "archived"),
		ReadBudget: 900,
		TailAfter:  500,
	})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC))
	if len(batch.Events) != 1 {
		t.Fatalf("events %d", len(batch.Events))
	}
	if batch.Events[0].Total() != 34 {
		t.Fatalf("burst %d", batch.Events[0].Total())
	}
	if batch.Events[0].SessionID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("session %s", batch.Events[0].SessionID)
	}
}

func TestPollContextSamples(t *testing.T) {
	root := t.TempDir()
	writeCopy(t, filepath.Join(root, "sessions", "rollout-2026-08-16T20-00-00-44444444-4444-4444-4444-444444444444.jsonl"), "context.jsonl")
	src := New(Options{Sessions: filepath.Join(root, "sessions"), Archive: filepath.Join(root, "archived")})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC))
	if len(batch.Events) != 2 || len(batch.ContextSamples) != 1 {
		t.Fatalf("events=%d samples=%d", len(batch.Events), len(batch.ContextSamples))
	}
}
