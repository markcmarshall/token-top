package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/markcmarshall/token-top/engine"
	"github.com/markcmarshall/token-top/telemetry"
)

type Tier int

const (
	TierNarrow Tier = iota
	TierNormal
	TierWide
)

type Options struct {
	Width    int
	Height   int
	Now      time.Time
	Color    bool
	Interval time.Duration
}

func tierFor(width int) Tier {
	switch {
	case width < 80:
		return TierNarrow
	case width < 120:
		return TierNormal
	default:
		return TierWide
	}
}

func Render(snap engine.Snapshot, opt Options) string {
	if opt.Width <= 0 {
		opt.Width = 80
	}
	if opt.Height <= 0 {
		opt.Height = 24
	}
	if opt.Now.IsZero() {
		opt.Now = snap.GeneratedAt
	}
	if opt.Interval <= 0 {
		opt.Interval = 2 * time.Second
	}
	tier := tierFor(opt.Width)
	s := initStyles(opt.Color)

	var b strings.Builder
	b.WriteString(renderTitle(opt, s))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(renderBurn(snap, s))
	b.WriteByte('\n')
	b.WriteString(renderActive(snap))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(renderHarness(snap, s))
	b.WriteByte('\n')
	b.WriteString(renderBar(snap, opt.Width, s))
	for _, line := range renderHealthDetails(snap, s) {
		b.WriteByte('\n')
		b.WriteString(line)
	}
	b.WriteByte('\n')
	b.WriteByte('\n')
	header := tableHeader(tier)
	b.WriteString(s.dim.Render(header))
	b.WriteByte('\n')

	used := 2 + 2 + 2 + 2 + len(renderHealthDetails(snap, s)) + 2 + 1 + 1
	rows, overflow := visibleRows(snap.Sessions, opt.Height-used)
	for _, row := range rows {
		b.WriteString(renderRow(row, tier, opt, s))
		b.WriteByte('\n')
	}
	if overflow > 0 {
		b.WriteString(s.dim.Render(fmt.Sprintf("+%d more recent", overflow)))
		b.WriteByte('\n')
	}
	b.WriteString(s.dim.Render(renderFooter(snap)))
	b.WriteByte('\n')
	return fitWidth(b.String(), opt.Width)
}

func renderTitle(opt Options, s styles) string {
	left := "TOKEN TOP"
	right := fmt.Sprintf("%s · refresh %s", opt.Now.Format("15:04:05"), opt.Interval)
	gap := opt.Width - 1 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return " " + s.title.Render(left) + strings.Repeat(" ", gap) + s.dim.Render(right)
}

func renderBurn(snap engine.Snapshot, s styles) string {
	return fmt.Sprintf(" BURN     1m %s    5m %s    15m %s    TODAY %s",
		s.bright.Render(formatRate(snap.Global.Rate1m)),
		formatRate(snap.Global.Rate5m),
		formatRate(snap.Global.Rate15m),
		formatCount(snap.Global.Today, false),
	)
}

func renderActive(snap engine.Snapshot) string {
	return fmt.Sprintf(" ACTIVE   %d burning    %d recent     %d quiet hidden",
		snap.Global.Burning, snap.Global.Recent, snap.QuietHidden)
}

func renderHarness(snap engine.Snapshot, s styles) string {
	parts := make([]string, 0, len(snap.Sources))
	for _, src := range snap.Sources {
		label := strings.ToUpper(string(src.Name))
		mark := healthMark(src.Health.State)
		share := fmt.Sprintf("%.0f%%", src.Share1m*100)
		part := fmt.Sprintf("%s %s  %s  %s",
			s.source(src.Name, true).Render(label),
			mark,
			formatRate(src.Rate1m),
			share,
		)
		parts = append(parts, part)
	}
	return " " + strings.Join(parts, "    ")
}

func renderBar(snap engine.Snapshot, width int, s styles) string {
	barW := width - 2
	if barW > 48 {
		barW = 48
	}
	if barW < 8 {
		barW = 8
	}
	if snap.Global.Rate1m <= 0 {
		return " " + s.dim.Render(strings.Repeat("░", barW))
	}
	var b strings.Builder
	b.WriteByte(' ')
	used := 0
	for i, src := range snap.Sources {
		n := int(src.Share1m * float64(barW))
		if src.Share1m > 0 && n == 0 {
			n = 1
		}
		if i == len(snap.Sources)-1 {
			n = barW - used
		}
		if n < 0 {
			n = 0
		}
		if used+n > barW {
			n = barW - used
		}
		used += n
		seg := strings.Repeat("█", n)
		b.WriteString(s.source(src.Name, true).Render(seg))
	}
	if used < barW {
		b.WriteString(s.dim.Render(strings.Repeat("░", barW-used)))
	}
	return b.String()
}

func renderHealthDetails(snap engine.Snapshot, s styles) []string {
	var lines []string
	for _, src := range snap.Sources {
		if src.Health.State != telemetry.HealthDegraded && src.Health.State != telemetry.HealthFailed {
			continue
		}
		if src.Health.Detail == "" {
			continue
		}
		name := strings.ToUpper(string(src.Name))
		lines = append(lines, " "+s.warn.Render(fmt.Sprintf("%s %s %s", name, healthMark(src.Health.State), src.Health.Detail)))
	}
	return lines
}

func tableHeader(tier Tier) string {
	switch tier {
	case TierNarrow:
		return fmt.Sprintf(" %s  %s  %s  %s  %s  %s",
			"S", padRight("H", 3), padRight("PROJECT", 12), padLeft("1M", 8), padLeft("TOTAL", 8), padLeft("LAST", 5))
	case TierWide:
		return fmt.Sprintf(" %s  %s  %s  %s  %s  %s  %s  %s  %s  %s  %s  %s  %s",
			"S", padRight("HARNESS", 7), padRight("MODEL", 14), padRight("PROJECT", 16),
			padLeft("1M", 8), padLeft("5M", 8), padLeft("15M", 8), padLeft("TOTAL", 8),
			padLeft("IN", 8), padLeft("OUT", 8), padLeft("CACHE", 6), padLeft("AGE", 5), padLeft("SID", 8))
	default:
		return fmt.Sprintf(" %s  %s  %s  %s  %s  %s  %s  %s  %s",
			"S", padRight("HARNESS", 7), padRight("MODEL", 14), padRight("PROJECT", 16),
			padLeft("1M", 8), padLeft("5M", 8), padLeft("TOTAL", 8), padLeft("CACHE", 6), padLeft("LAST", 5))
	}
}

func renderRow(row engine.Session, tier Tier, opt Options, s styles) string {
	state := "·"
	if row.Activity == engine.ActivityBurning {
		state = "●"
	}
	hs := s.source(row.Source, row.Activity == engine.ActivityBurning)
	project := row.ProjectLabel
	if project == "" {
		project = "unknown"
	}
	last := formatAge(opt.Now, row.LastEvent)
	switch tier {
	case TierNarrow:
		return fmt.Sprintf(" %s  %s  %s  %s  %s  %s",
			state,
			hs.Render(padRight(harnessLabel(row.Source, false), 3)),
			padRight(trunc(project, 12), 12),
			padLeft(formatRate(row.Rate1m), 8),
			padLeft(formatCount(row.Total, row.TotalApprox), 8),
			padLeft(last, 5),
		)
	case TierWide:
		model := displayModel(row.Model)
		if row.ModelCount > 1 {
			model = fmt.Sprintf("%s +%d", model, row.ModelCount-1)
		}
		return fmt.Sprintf(" %s  %s  %s  %s  %s  %s  %s  %s  %s  %s  %s  %s  %s",
			state,
			hs.Render(padRight(harnessLabel(row.Source, true), 7)),
			padRight(trunc(model, 14), 14),
			padRight(trunc(project, 16), 16),
			padLeft(formatRate(row.Rate1m), 8),
			padLeft(formatRate(row.Rate5m), 8),
			padLeft(formatRate(row.Rate15m), 8),
			padLeft(formatCount(row.Total, row.TotalApprox), 8),
			padLeft(formatCount(row.Input, false), 8),
			padLeft(formatCount(row.Output, false), 8),
			padLeft(formatPct(row.CacheRatio), 6),
			padLeft(formatAge(opt.Now, row.FirstEvent), 5),
			padLeft(shortSession(row.SessionID), 8),
		)
	default:
		return fmt.Sprintf(" %s  %s  %s  %s  %s  %s  %s  %s  %s",
			state,
			hs.Render(padRight(harnessLabel(row.Source, true), 7)),
			padRight(trunc(displayModel(row.Model), 14), 14),
			padRight(trunc(project, 16), 16),
			padLeft(formatRate(row.Rate1m), 8),
			padLeft(formatRate(row.Rate5m), 8),
			padLeft(formatCount(row.Total, row.TotalApprox), 8),
			padLeft(formatPct(row.CacheRatio), 6),
			padLeft(last, 5),
		)
	}
}

func renderFooter(snap engine.Snapshot) string {
	return fmt.Sprintf(" trailing completed usage · %d quiet hidden · q quit", snap.QuietHidden)
}

func visibleRows(rows []engine.Session, max int) ([]engine.Session, int) {
	if max < 1 {
		max = 1
	}
	if len(rows) <= max {
		return rows, 0
	}
	return rows[:max], len(rows) - max
}

func fitWidth(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if lipgloss.Width(line) > width {
			lines[i] = trunc(line, width)
		}
	}
	return strings.Join(lines, "\n")
}

type styles struct {
	color  bool
	title  lipgloss.Style
	dim    lipgloss.Style
	bright lipgloss.Style
	warn   lipgloss.Style
}

func (s styles) source(name telemetry.SourceName, hot bool) lipgloss.Style {
	st := lipgloss.NewStyle()
	if !s.color {
		return st
	}
	switch name {
	case telemetry.SourceClaude:
		st = st.Foreground(lipgloss.Color("#C4785A"))
	case telemetry.SourceCodex:
		st = st.Foreground(lipgloss.Color("#6B8F71"))
	case telemetry.SourceGrok:
		st = st.Foreground(lipgloss.Color("#8B7EC8"))
	}
	if !hot {
		st = st.Faint(true)
	}
	return st
}

func (s styles) withColor() styles {
	s.title = lipgloss.NewStyle()
	s.dim = lipgloss.NewStyle()
	s.bright = lipgloss.NewStyle()
	s.warn = lipgloss.NewStyle()
	if s.color {
		s.title = s.title.Bold(true)
		s.dim = s.dim.Faint(true)
		s.bright = s.bright.Bold(true)
		s.warn = s.warn.Foreground(lipgloss.Color("#C9A227"))
	}
	return s
}

func initStyles(color bool) styles {
	return styles{color: color}.withColor()
}
