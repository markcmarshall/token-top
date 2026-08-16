package telemetry

import (
	"math"
	"testing"
	"time"
)

func TestTotalIsInputPlusOutput(t *testing.T) {
	e := TokenEvent{Input: 10, Output: 7}
	if e.Total() != 17 {
		t.Fatalf("total %d", e.Total())
	}
}

func TestUnknownOptionalsAreAbsent(t *testing.T) {
	e := TokenEvent{
		ID: "1", Source: SourceCodex, SessionID: "s",
		Timestamp: time.Unix(1, 0).UTC(),
		Input:     4, Output: 1,
	}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	if e.CacheRead != nil || e.CacheWrite != nil || e.Reasoning != nil {
		t.Fatal("optionals should be absent")
	}
}

func TestZeroOptionalIsKnownZero(t *testing.T) {
	e := TokenEvent{
		ID: "1", Source: SourceCodex, SessionID: "s",
		Timestamp:  time.Unix(1, 0).UTC(),
		Input:      4,
		Output:     1,
		CacheRead:  Uint64Ptr(0),
		CacheWrite: Uint64Ptr(0),
		Reasoning:  Uint64Ptr(0),
	}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSubsetInvariants(t *testing.T) {
	base := TokenEvent{
		ID: "1", Source: SourceClaude, SessionID: "s",
		Timestamp: time.Unix(1, 0).UTC(),
		Input:     10, Output: 8,
	}
	bad := []TokenEvent{
		with(base, func(e *TokenEvent) { e.CacheRead = Uint64Ptr(11) }),
		with(base, func(e *TokenEvent) { e.CacheWrite = Uint64Ptr(11) }),
		with(base, func(e *TokenEvent) { e.Reasoning = Uint64Ptr(9) }),
	}
	for i, e := range bad {
		if err := e.Validate(); err == nil {
			t.Fatalf("case %d: expected invalid", i)
		}
	}
	ok := with(base, func(e *TokenEvent) {
		e.CacheRead = Uint64Ptr(10)
		e.CacheWrite = Uint64Ptr(3)
		e.Reasoning = Uint64Ptr(8)
	})
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestTotalOverflowRejected(t *testing.T) {
	e := TokenEvent{
		ID: "1", Source: SourceGrok, SessionID: "s",
		Timestamp: time.Unix(1, 0).UTC(),
		Input:     math.MaxUint64, Output: 1,
	}
	if err := e.Validate(); err == nil {
		t.Fatal("expected overflow")
	}
	if e.Total() != math.MaxUint64 {
		t.Fatalf("overflow total %d", e.Total())
	}
}

func TestAddUint64(t *testing.T) {
	if _, ok := AddUint64(math.MaxUint64, 1); ok {
		t.Fatal("expected overflow")
	}
	sum, ok := AddUint64(math.MaxUint64-1, 1)
	if !ok || sum != math.MaxUint64 {
		t.Fatalf("sum %d ok %v", sum, ok)
	}
}

func with(e TokenEvent, fn func(*TokenEvent)) TokenEvent {
	fn(&e)
	return e
}
