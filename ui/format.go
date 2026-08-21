package ui

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/markcmarshall/token-top/telemetry"
)

func compact(n uint64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 10_000:
		return trimFloat(float64(n)/1000, "K")
	case n < 1_000_000:
		return fmt.Sprintf("%dK", n/1000)
	case n < 10_000_000:
		return trimFloat(float64(n)/1_000_000, "M")
	default:
		return trimFloat(float64(n)/1_000_000, "M")
	}
}

func trimFloat(v float64, suf string) string {
	s := fmt.Sprintf("%.1f", v)
	s = strings.TrimSuffix(s, ".0")
	return s + suf
}

func formatRate(v float64) string {
	if v == 0 {
		return "0"
	}
	if v == math.Trunc(v) && v < 1000 {
		return fmt.Sprintf("%.0f/m", v)
	}
	return compact(uint64(math.Round(v))) + "/m"
}

func formatCount(n uint64, approx bool) string {
	s := compact(n)
	if approx {
		return "~" + s
	}
	return s
}

func formatPct(p *float64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%.0f%%", *p*100)
}

func formatAge(now, then time.Time) string {
	if then.IsZero() || now.Before(then) {
		return "0s"
	}
	d := now.Sub(then)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func shortSession(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func displayModel(model string) string {
	model = strings.TrimPrefix(model, "claude-")
	return model
}

func harnessLabel(name telemetry.SourceName, wide bool) string {
	if wide {
		switch name {
		case telemetry.SourceClaude:
			return "claude"
		case telemetry.SourceCodex:
			return "codex"
		case telemetry.SourceGrok:
			return "grok"
		}
		return string(name)
	}
	switch name {
	case telemetry.SourceClaude:
		return "cld"
	case telemetry.SourceCodex:
		return "cdx"
	case telemetry.SourceGrok:
		return "grk"
	default:
		if len(name) > 3 {
			return string(name[:3])
		}
		return string(name)
	}
}

func healthMark(state telemetry.HealthState) string {
	switch state {
	case telemetry.HealthOK:
		return "✓"
	case telemetry.HealthNotDetected:
		return "—"
	case telemetry.HealthDegraded:
		return "!"
	case telemetry.HealthFailed:
		return "x"
	default:
		return "?"
	}
}

func padRight(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func padLeft(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return strings.Repeat(" ", n-w) + s
}

func trunc(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	for lipgloss.Width(s) > n-1 && s != "" {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s + "…"
}
