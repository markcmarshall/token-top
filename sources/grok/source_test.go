package grok

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

func writeSession(t *testing.T, root, id, fixture string, summary bool) string {
	t.Helper()
	dir := filepath.Join(root, "proj", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if summary {
		body := `{"info":{"id":"` + id + `","cwd":"/work/acme"},"current_model_id":"grok-4.6"}`
		if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if fixture != "" {
		data, err := os.ReadFile(filepath.Join("testdata", fixture))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "updates.jsonl"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
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
	writeSession(t, root, "11111111-1111-1111-1111-111111111111", "normal.jsonl", true)
	src := New(Options{Sessions: root})
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
	if snap.Sessions[0].Source != telemetry.SourceGrok || snap.Sessions[0].ProjectLabel != "acme" {
		t.Fatalf("snap %+v", snap.Sessions)
	}
}

func TestPollMultiModel(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, "22222222-2222-2222-2222-222222222222", "multi.jsonl", true)
	src := New(Options{Sessions: root})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC))
	if len(batch.Events) != 2 {
		t.Fatalf("events %d", len(batch.Events))
	}
}

func TestPollIncompleteDegrades(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, "33333333-3333-3333-3333-333333333333", "incomplete.jsonl", true)
	src := New(Options{Sessions: root})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC))
	if batch.Health.State != telemetry.HealthDegraded || len(batch.Events) != 1 || batch.Events[0].Complete {
		t.Fatalf("health=%+v events=%+v", batch.Health, batch.Events)
	}
}

func TestPollMissingUpdatesDegrades(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, "44444444-4444-4444-4444-444444444444", "", true)
	src := New(Options{Sessions: root})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC))
	if batch.Health.State != telemetry.HealthDegraded || !strings.Contains(batch.Health.Detail, "missing updates") {
		t.Fatalf("health %+v", batch.Health)
	}
	if len(batch.Events) != 0 {
		t.Fatalf("events %d", len(batch.Events))
	}
}

func TestPollMissingUpdatesWithPositiveActivityDegrades(t *testing.T) {
	root := t.TempDir()
	dir := writeSession(t, root, "44444444-4444-4444-4444-444444444445", "", true)
	body := `{"info":{"id":"44444444-4444-4444-4444-444444444445","cwd":"/work/acme"},"current_model_id":"grok-4.6","num_messages":1}`
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	src := New(Options{Sessions: root})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC))
	if batch.Health.State != telemetry.HealthDegraded || !strings.Contains(batch.Health.Detail, "missing updates") {
		t.Fatalf("health %+v", batch.Health)
	}
}

func TestPollMissingUpdatesWithZeroActivityIsHealthy(t *testing.T) {
	root := t.TempDir()
	dir := writeSession(t, root, "44444444-4444-4444-4444-444444444446", "", true)
	body := `{"info":{"id":"44444444-4444-4444-4444-444444444446","cwd":"/work/acme"},"current_model_id":"grok-4.6","num_messages":0}`
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	src := New(Options{Sessions: root})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC))
	if batch.Health.State != telemetry.HealthOK {
		t.Fatalf("health %+v", batch.Health)
	}
}

func TestPollDegradesOnMalformed(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, "55555555-5555-5555-5555-555555555555", "malformed.jsonl", true)
	src := New(Options{Sessions: root})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC))
	if batch.Health.State != telemetry.HealthDegraded || len(batch.Events) != 2 {
		t.Fatalf("health=%+v events=%d", batch.Health, len(batch.Events))
	}
}

func TestPollDoesNotReadUnifiedLog(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, "11111111-1111-1111-1111-111111111111", "normal.jsonl", true)
	if err := os.WriteFile(filepath.Join(root, "unified.jsonl"), []byte(`{"msg":"shell.turn.inference_done","ctx":{"prompt_tokens":999,"completion_tokens":9}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := New(Options{Sessions: root})
	batch := src.Poll(context.Background(), time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC))
	if len(batch.Events) != 2 {
		t.Fatalf("events %d", len(batch.Events))
	}
	for _, ev := range batch.Events {
		if ev.Total() == 1008 {
			t.Fatal("read unified log")
		}
	}
}

func TestPollIncrementalAppend(t *testing.T) {
	root := t.TempDir()
	dir := writeSession(t, root, "11111111-1111-1111-1111-111111111111", "normal.jsonl", true)
	src := New(Options{Sessions: root})
	now := time.Date(2026, 8, 16, 20, 0, 12, 0, time.UTC)
	first := src.Poll(context.Background(), now)
	if len(first.Events) != 2 {
		t.Fatalf("first %d", len(first.Events))
	}
	second := src.Poll(context.Background(), now)
	if len(second.Events) != 0 {
		t.Fatalf("replayed %d", len(second.Events))
	}
	f, err := os.OpenFile(filepath.Join(dir, "updates.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(`{"method":"session/update","timestamp":"2026-08-16T20:00:20.000Z","params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":7,"outputTokens":1,"totalTokens":8,"modelUsage":{"grok-4.6":{"inputTokens":7,"outputTokens":1,"totalTokens":8}}}}}}` + "\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	third := src.Poll(context.Background(), now.Add(20*time.Second))
	if len(third.Events) != 1 || third.Events[0].Total() != 8 {
		t.Fatalf("append %+v", third.Events)
	}
}

func TestPollDefaultCorpusSmoke(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".grok", "sessions")); err != nil {
		t.Skip("no local Grok sessions")
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
