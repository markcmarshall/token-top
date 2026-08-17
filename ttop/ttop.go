// Package ttop is the stable v1 consumer API.
//
// FounderOS (or any other integrator) should import this package, inject an
// [attribution.Attributor], and compose snapshots or a live loop from the
// returned engine. The public module has no FounderOS, Postgres, or claim types.
package ttop

import (
	"context"
	"time"

	"github.com/markcmarshall/token-top/attribution"
	"github.com/markcmarshall/token-top/engine"
	"github.com/markcmarshall/token-top/sources/claude"
	"github.com/markcmarshall/token-top/sources/codex"
	"github.com/markcmarshall/token-top/sources/grok"
	"github.com/markcmarshall/token-top/telemetry"
)

// Options configure a consumer-built engine.
type Options struct {
	// Attributor labels events. Nil uses standalone git-root / CWD basename.
	Attributor attribution.Attributor
	// Clock is the injected clock. Nil uses the system clock.
	Clock engine.Clock
}

// Sources returns the v1 local adapters in snapshot order: Claude, Codex, Grok.
func Sources() []telemetry.Source {
	return []telemetry.Source{
		claude.New(claude.Options{}),
		codex.New(codex.Options{}),
		grok.New(grok.Options{}),
	}
}

// New builds an engine that FounderOS can poll with [Sources] or its own set.
func New(opt Options) *engine.Engine {
	return engine.New(opt.Clock, opt.Attributor)
}

// Snapshot drains today's files up to ctx and returns one immutable snapshot.
func Snapshot(ctx context.Context, opt Options) engine.Snapshot {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
	}
	return New(opt).PollUntilToday(ctx, Sources())
}
