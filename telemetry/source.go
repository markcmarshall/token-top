package telemetry

import (
	"context"
	"time"
)

type SourceName string

const (
	SourceClaude SourceName = "claude"
	SourceCodex  SourceName = "codex"
	SourceGrok   SourceName = "grok"
)

// AllSources is the stable v1 harness order used by snapshots and the screen.
var AllSources = []SourceName{SourceClaude, SourceCodex, SourceGrok}

type Source interface {
	Name() SourceName
	Poll(context.Context, time.Time) Batch
}

type Batch struct {
	Events         []TokenEvent
	ContextSamples []ContextSample
	Health         SourceHealth
}

type ContextSample struct {
	ID        string
	Source    SourceName
	SessionID string
	Timestamp time.Time
	Model     string
	Occupied  uint64
	Maximum   uint64
}
