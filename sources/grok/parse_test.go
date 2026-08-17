package grok

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
)

func consumeFile(t *testing.T, name string, p Parser) (n int, bad int, events int) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	var evCount int
	for sc.Scan() {
		n++
		got := p.Consume(sc.Bytes())
		if got.Malformed || got.Unexpected {
			bad++
		}
		evCount += len(got.Events)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return n, bad, evCount
}

func consumeEvents(t *testing.T, name string) parsed {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p := Parser{SessionID: "11111111-1111-1111-1111-111111111111", CWD: "/work/FounderOS", Model: "grok-4.6"}
	var out parsed
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		got := p.Consume(sc.Bytes())
		out.Events = append(out.Events, got.Events...)
		out.Malformed = out.Malformed || got.Malformed
		out.Unexpected = out.Unexpected || got.Unexpected
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNormalCompletedTurns(t *testing.T) {
	got := consumeEvents(t, "normal.jsonl")
	if got.Malformed || got.Unexpected || len(got.Events) != 2 {
		t.Fatalf("events=%d malformed=%v unexpected=%v", len(got.Events), got.Malformed, got.Unexpected)
	}
	if got.Events[0].Input != 100 || got.Events[0].Output != 10 || got.Events[0].Total() != 110 {
		t.Fatalf("first %+v", got.Events[0])
	}
	if got.Events[0].CacheRead == nil || *got.Events[0].CacheRead != 40 {
		t.Fatalf("cache %+v", got.Events[0].CacheRead)
	}
	if got.Events[0].Reasoning == nil || *got.Events[0].Reasoning != 4 {
		t.Fatalf("reason %+v", got.Events[0].Reasoning)
	}
	if got.Events[0].Model != "grok-4.6" || got.Events[0].CWD != "/work/FounderOS" {
		t.Fatalf("meta %+v", got.Events[0])
	}
	if got.Events[1].Input != 50 || got.Events[1].Total() != 55 {
		t.Fatalf("second %+v", got.Events[1])
	}
	if err := got.Events[0].Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestMultiModelTurnEmitsPerComponent(t *testing.T) {
	got := consumeEvents(t, "multi.jsonl")
	if len(got.Events) != 2 {
		t.Fatalf("events %d", len(got.Events))
	}
	if got.Events[0].Model != "grok-4.6" || got.Events[0].Total() != 110 {
		t.Fatalf("first %+v", got.Events[0])
	}
	if got.Events[1].Model != "grok-4.6-build" || got.Events[1].Total() != 55 {
		t.Fatalf("second %+v", got.Events[1])
	}
	if got.Events[0].ID == got.Events[1].ID {
		t.Fatal("shared id")
	}
}

func TestIncompleteUsageDegrades(t *testing.T) {
	got := consumeEvents(t, "incomplete.jsonl")
	if !got.Unexpected || len(got.Events) != 1 {
		t.Fatalf("events=%d unexpected=%v", len(got.Events), got.Unexpected)
	}
	if got.Events[0].Complete {
		t.Fatal("expected incomplete")
	}
	if got.Events[0].Total() != 88 {
		t.Fatalf("total %d", got.Events[0].Total())
	}
}

func TestMalformedSurroundedByValid(t *testing.T) {
	p := Parser{SessionID: "11111111-1111-1111-1111-111111111111", CWD: "/work/delta", Model: "grok-4.6"}
	_, bad, evs := consumeFile(t, "malformed.jsonl", p)
	if bad != 2 || evs != 2 {
		t.Fatalf("bad=%d events=%d", bad, evs)
	}
}

func TestIgnoresUnifiedInferenceShape(t *testing.T) {
	p := Parser{SessionID: "s", CWD: "/work", Model: "grok-4.6"}
	got := p.Consume([]byte(`{"msg":"shell.turn.inference_done","sid":"s","ts":"2026-08-16T20:00:03.000Z","ctx":{"prompt_tokens":10,"completion_tokens":2}}`))
	if got.Malformed || got.Unexpected || len(got.Events) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestParseSummary(t *testing.T) {
	id, cwd, model, err := ParseSummary([]byte(`{"info":{"id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","cwd":"/work/FounderOS"},"current_model_id":"grok-4.6","session_summary":"REDACTED"}`))
	if err != nil || id != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" || cwd != "/work/FounderOS" || model != "grok-4.6" {
		t.Fatalf("%s %s %s %v", id, cwd, model, err)
	}
}

func TestStableIDs(t *testing.T) {
	a := consumeEvents(t, "normal.jsonl")
	b := consumeEvents(t, "normal.jsonl")
	if a.Events[0].ID == "" || a.Events[0].ID != b.Events[0].ID {
		t.Fatalf("ids %q %q", a.Events[0].ID, b.Events[0].ID)
	}
}
