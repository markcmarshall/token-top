package telemetry

import (
	"testing"
	"time"
)

func TestLocalDateUsesMachineLocalDay(t *testing.T) {
	now := time.Date(2026, 8, 16, 20, 30, 0, 0, time.Local)
	if LocalDate(now) != LocalDate(now.UTC()) {
		t.Fatalf("local %s utc-instant %s", LocalDate(now), LocalDate(now.UTC()))
	}
}

func TestRecordRefIsBasenameAndOffset(t *testing.T) {
	got := RecordRef("/Users/mark/.claude/projects/x/session.jsonl", 4096)
	if got != "session.jsonl:4096" {
		t.Fatalf("%s", got)
	}
	if RecordRef("/tmp/a.jsonl", 0) != "a.jsonl" {
		t.Fatal(RecordRef("/tmp/a.jsonl", 0))
	}
}
