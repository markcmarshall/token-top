package main

import (
	"context"
	"fmt"
	"io"
	"math"

	"github.com/markcmarshall/token-top/engine"
	"github.com/markcmarshall/token-top/sources/claude"
	"github.com/markcmarshall/token-top/sources/codex"
	"github.com/markcmarshall/token-top/telemetry"
)

func runOnce(stdout io.Writer) int {
	clk := engine.SystemClock{}
	eng := engine.New(clk, nil)
	eng.Poll(context.Background(), []telemetry.Source{
		claude.New(claude.Options{}),
		codex.New(codex.Options{}),
	})
	printSnapshot(stdout, eng.Snapshot())
	return 0
}

func printSnapshot(w io.Writer, snap engine.Snapshot) {
	fmt.Fprintf(w, "TOKEN TOP  %s\n", snap.GeneratedAt.Format("15:04:05"))
	fmt.Fprintf(w, "BURN  1m %s  5m %s  15m %s  TODAY %s\n",
		formatRate(snap.Global.Rate1m),
		formatRate(snap.Global.Rate5m),
		formatRate(snap.Global.Rate15m),
		formatCount(snap.Global.Today),
	)
	fmt.Fprintf(w, "ACTIVE  %d burning  %d recent  %d quiet hidden\n",
		snap.Global.Burning, snap.Global.Recent, snap.QuietHidden)
	for _, src := range snap.Sources {
		mark := healthMark(src.Health)
		fmt.Fprintf(w, "%s %s  %s\n", srcLabel(src.Name), mark, formatRate(src.Rate1m))
		if src.Health.Detail != "" && src.Health.State != telemetry.HealthOK && !src.Health.Indexing {
			fmt.Fprintf(w, "  %s\n", src.Health.Detail)
		}
		if src.Health.Indexing {
			fmt.Fprintf(w, "  indexing\n")
		}
	}
	for _, row := range snap.Sessions {
		state := "·"
		if row.Activity == engine.ActivityBurning {
			state = "●"
		}
		fmt.Fprintf(w, "%s  %s  %s  %s  %s  %s\n",
			state,
			row.Source,
			row.Model,
			row.ProjectLabel,
			formatRate(row.Rate1m),
			formatCount(row.Total),
		)
	}
}

func srcLabel(name telemetry.SourceName) string {
	switch name {
	case telemetry.SourceClaude:
		return "CLAUDE"
	case telemetry.SourceCodex:
		return "CODEX"
	case telemetry.SourceGrok:
		return "GROK"
	default:
		return string(name)
	}
}

func healthMark(h telemetry.SourceHealth) string {
	switch h.State {
	case telemetry.HealthOK:
		return "ok"
	case telemetry.HealthNotDetected:
		return "—"
	case telemetry.HealthDegraded:
		return "!"
	case telemetry.HealthFailed:
		return "x"
	default:
		return string(h.State)
	}
}

func formatRate(v float64) string {
	if v == 0 {
		return "0"
	}
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.0f/m", v)
	}
	return fmt.Sprintf("%.1f/m", v)
}

func formatCount(n uint64) string {
	return fmt.Sprintf("%d", n)
}
