package claude

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"

	"github.com/markcmarshall/token-top/telemetry"
)

func consumeFile(t *testing.T, name string) (events []telemetry.TokenEvent, bad int) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p := Parser{SessionID: SessionIDFromPath(name)}
	var d Deduper
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		got := p.Consume(sc.Bytes())
		if got.Malformed || got.Unexpected {
			bad++
		}
		if got.Event == nil {
			continue
		}
		ev, ok := d.Filter(*got.Event, got.MessageID)
		if ok {
			events = append(events, ev)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return events, bad
}

func TestCacheAndThinkingNormalization(t *testing.T) {
	events, bad := consumeFile(t, "normal.jsonl")
	if bad != 0 || len(events) != 2 {
		t.Fatalf("events=%d bad=%d", len(events), bad)
	}
	if events[0].SessionID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("session %s", events[0].SessionID)
	}
	if events[0].Model != "claude-opus-4.8" || events[0].CWD != "/work/acme" {
		t.Fatalf("meta %+v", events[0])
	}
	if events[0].Input != 52 || events[0].Output != 10 || events[0].Total() != 62 {
		t.Fatalf("first %+v", events[0])
	}
	if events[0].CacheRead == nil || *events[0].CacheRead != 30 {
		t.Fatalf("cache read %+v", events[0].CacheRead)
	}
	if events[0].CacheWrite == nil || *events[0].CacheWrite != 20 {
		t.Fatalf("cache write %+v", events[0].CacheWrite)
	}
	if events[0].Reasoning == nil || *events[0].Reasoning != 4 {
		t.Fatalf("thinking %+v", events[0].Reasoning)
	}
	if events[1].Input != 51 || events[1].Output != 7 {
		t.Fatalf("second %+v", events[1])
	}
	if events[1].Reasoning != nil {
		t.Fatalf("second thinking %+v", events[1].Reasoning)
	}
	if err := events[0].Validate(); err != nil {
		t.Fatal(err)
	}
	if err := events[1].Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDuplicateMessageIDCountedOnce(t *testing.T) {
	events, bad := consumeFile(t, "duplicate.jsonl")
	if bad != 0 || len(events) != 1 {
		t.Fatalf("events=%d bad=%d", len(events), bad)
	}
	if events[0].Total() != 52 {
		t.Fatalf("total %d", events[0].Total())
	}
}

func TestStreamedMessageEmitsOutputDelta(t *testing.T) {
	events, bad := consumeFile(t, "streamed.jsonl")
	if bad != 0 || len(events) != 2 {
		t.Fatalf("events=%d bad=%d", len(events), bad)
	}
	if events[0].Output != 4 || events[0].Input != 32 {
		t.Fatalf("first %+v", events[0])
	}
	if events[1].Input != 0 || events[1].Output != 76 {
		t.Fatalf("delta %+v", events[1])
	}
	if events[0].ID == events[1].ID {
		t.Fatal("delta reused id")
	}
	if err := events[1].Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEventLevelCWDPreserved(t *testing.T) {
	events, bad := consumeFile(t, "cwd_change.jsonl")
	if bad != 0 || len(events) != 2 {
		t.Fatalf("events=%d bad=%d", len(events), bad)
	}
	if events[0].CWD != "/work/acme" || events[1].CWD != "/work/scripts" {
		t.Fatalf("cwd %q %q", events[0].CWD, events[1].CWD)
	}
	if events[0].SessionID != events[1].SessionID {
		t.Fatal("split session")
	}
}

func TestMalformedSurroundedByValid(t *testing.T) {
	events, bad := consumeFile(t, "malformed.jsonl")
	if bad != 2 || len(events) != 2 {
		t.Fatalf("events=%d bad=%d", len(events), bad)
	}
	if events[0].Total() != 6 || events[1].Total() != 4 {
		t.Fatalf("totals %d %d", events[0].Total(), events[1].Total())
	}
}

func TestUnexpectedThinkingStillEmits(t *testing.T) {
	events, bad := consumeFile(t, "unexpected.jsonl")
	if bad != 1 || len(events) != 1 {
		t.Fatalf("events=%d bad=%d", len(events), bad)
	}
	if events[0].Complete {
		t.Fatal("expected incomplete")
	}
	if events[0].Reasoning != nil {
		t.Fatalf("kept invalid thinking %+v", events[0].Reasoning)
	}
	if events[0].Input != 12 || events[0].Output != 10 {
		t.Fatalf("event %+v", events[0])
	}
	if err := events[0].Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSessionIDFromPath(t *testing.T) {
	got := SessionIDFromPath("/x/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.jsonl")
	if got != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("got %s", got)
	}
	if SessionIDFromPath("/x/agent-ae8d6b5266df6e302.jsonl") != "" {
		t.Fatal("agent file is not a session id")
	}
}

func FuzzConsume(f *testing.F) {
	f.Add([]byte(`{"type":"assistant","timestamp":"2026-08-16T20:00:03.000Z","sessionId":"s","cwd":"/w","message":{"id":"m","model":"x","usage":{"input_tokens":1,"output_tokens":1}}}`))
	f.Add([]byte(`{`))
	f.Add([]byte(`not json`))
	f.Fuzz(func(t *testing.T, line []byte) {
		p := Parser{SessionID: "s"}
		_ = p.Consume(line)
	})
}

func TestStableIDsAcrossRescan(t *testing.T) {
	a, _ := consumeFile(t, "normal.jsonl")
	b, _ := consumeFile(t, "normal.jsonl")
	if a[0].ID == "" || a[0].ID != b[0].ID || a[1].ID != b[1].ID {
		t.Fatalf("ids %q %q", a[0].ID, b[0].ID)
	}
}
