package codex

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"

	"github.com/markcmarshall/token-top/telemetry"
)

func consumeFile(t *testing.T, name, sessionFromPath string) (events []telemetry.TokenEvent, samples []telemetry.ContextSample, bad int) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p := Parser{SessionID: sessionFromPath}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		ev, sample, malformed := p.Consume(sc.Bytes())
		if malformed {
			bad++
			continue
		}
		if ev != nil {
			events = append(events, *ev)
		}
		if sample != nil {
			samples = append(samples, *sample)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return events, samples, bad
}

func TestNormalCumulativeDeltas(t *testing.T) {
	events, samples, bad := consumeFile(t, "normal.jsonl", "")
	if bad != 0 || len(samples) != 0 {
		t.Fatalf("bad=%d samples=%d", bad, len(samples))
	}
	if len(events) != 2 {
		t.Fatalf("events %d", len(events))
	}
	if events[0].SessionID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("session %s", events[0].SessionID)
	}
	if events[0].Model != "gpt-5.6-sol" || events[0].CWD != "/work/FounderOS" {
		t.Fatalf("meta %+v", events[0])
	}
	if events[0].Input != 100 || events[0].Output != 10 || events[0].Total() != 110 {
		t.Fatalf("first %+v", events[0])
	}
	if events[0].CacheRead == nil || *events[0].CacheRead != 20 {
		t.Fatalf("cache %+v", events[0].CacheRead)
	}
	if events[0].Reasoning == nil || *events[0].Reasoning != 4 {
		t.Fatalf("reasoning %+v", events[0].Reasoning)
	}
	if events[1].Input != 50 || events[1].Output != 5 || events[1].Total() != 55 {
		t.Fatalf("second %+v", events[1])
	}
	if events[1].CacheRead == nil || *events[1].CacheRead != 40 {
		t.Fatalf("second cache %+v", events[1].CacheRead)
	}
	if err := events[0].Validate(); err != nil {
		t.Fatal(err)
	}
	if err := events[1].Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRepeatedLastTokenUsageSuppressed(t *testing.T) {
	events, _, bad := consumeFile(t, "repeated_last.jsonl", "")
	if bad != 0 || len(events) != 1 {
		t.Fatalf("events=%d bad=%d", len(events), bad)
	}
	if events[0].Total() != 88 {
		t.Fatalf("total %d", events[0].Total())
	}
}

func TestCounterResetStartsNewSegment(t *testing.T) {
	events, _, bad := consumeFile(t, "reset.jsonl", "")
	if bad != 0 || len(events) != 2 {
		t.Fatalf("events=%d bad=%d", len(events), bad)
	}
	if events[0].Total() != 220 || events[1].Total() != 48 {
		t.Fatalf("totals %d %d", events[0].Total(), events[1].Total())
	}
}

func TestContextSnapshotSeparated(t *testing.T) {
	events, samples, bad := consumeFile(t, "context.jsonl", "")
	if bad != 0 {
		t.Fatalf("bad %d", bad)
	}
	if len(events) != 2 {
		t.Fatalf("events %d", len(events))
	}
	if events[0].Total() != 96 || events[1].Total() != 34 {
		t.Fatalf("burn %d %d", events[0].Total(), events[1].Total())
	}
	if len(samples) != 1 {
		t.Fatalf("samples %d", len(samples))
	}
	if samples[0].Occupied != 21445 || samples[0].Maximum != 258400 {
		t.Fatalf("sample %+v", samples[0])
	}
	if samples[0].SessionID != events[0].SessionID {
		t.Fatal("sample session")
	}
}

func TestMalformedSurroundedByValid(t *testing.T) {
	events, _, bad := consumeFile(t, "malformed.jsonl", "")
	if bad != 1 || len(events) != 2 {
		t.Fatalf("events=%d bad=%d", len(events), bad)
	}
	if events[0].Total() != 12 || events[1].Total() != 8 {
		t.Fatalf("totals %d %d", events[0].Total(), events[1].Total())
	}
}

func TestSessionIDFromPath(t *testing.T) {
	got := SessionIDFromPath("/x/rollout-2026-08-16T12-50-56-01a00c20-9476-7f51-bbb4-1a537b6e2de1.jsonl")
	if got != "01a00c20-9476-7f51-bbb4-1a537b6e2de1" {
		t.Fatalf("got %s", got)
	}
}

func TestStableIDsAcrossRescan(t *testing.T) {
	a, _, _ := consumeFile(t, "normal.jsonl", "")
	b, _, _ := consumeFile(t, "normal.jsonl", "")
	if a[0].ID == "" || a[0].ID != b[0].ID || a[1].ID != b[1].ID {
		t.Fatalf("ids %q %q", a[0].ID, b[0].ID)
	}
}
