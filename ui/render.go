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
	Width             int
	Height            int
	Now               time.Time
	Color             bool
	Interval          time.Duration
	AttributionHeader string
}

func tierFor(width int) Tier {
	switch {
	case width < 72:
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
	if opt.AttributionHeader == "" {
		opt.AttributionHeader = "PROJECT"
	}

	tier := tierFor(opt.Width)
	s := initStyles(opt.Color)
	compactHeight := opt.Height < 16
	lines := []string{" " + s.title.Render("TOKEN TOP")}
	if !compactHeight {
		lines = append(lines, "")
	}
	lines = append(lines, renderToday(snap, s))
	lines = append(lines, renderAccounting(snap, tier, s)...)
	lines = append(lines, renderHealthDetails(snap, s)...)
	if !compactHeight {
		lines = append(lines, "")
	}
	lines = append(lines, renderRecent(snap, tier, s))
	if !compactHeight {
		lines = append(lines, "")
	}
	lines = append(lines, renderHarness(snap, opt.Width, tier, compactHeight, s)...)
	if !compactHeight {
		lines = append(lines, "")
	}

	header := attributionHeader(opt.AttributionHeader, opt.Width, tier)
	lines = append(lines, s.dim.Render(header))
	footerLines := 1
	if compactHeight {
		footerLines = 0
	}
	available := opt.Height - len(lines) - footerLines
	if available < 0 {
		available = 0
	}
	rowLimit := available
	if len(snap.Attributions) > rowLimit && rowLimit > 0 {
		rowLimit--
	}
	rows, overflow := visibleAttributions(snap.Attributions, rowLimit)
	for _, row := range rows {
		lines = append(lines, renderAttribution(row, opt.Width, tier, opt, s))
	}
	if overflow > 0 {
		lines = append(lines, s.dim.Render(fmt.Sprintf(" +%d more", overflow)))
	}
	if !compactHeight {
		lines = append(lines, s.dim.Render(" q quit"))
	}

	return fitWidth(strings.Join(lines, "\n")+"\n", opt.Width)
}

func renderToday(snap engine.Snapshot, s styles) string {
	return fmt.Sprintf(" TODAY  %s processed", s.bright.Render(formatCount(snap.Global.Today, snap.Global.TodayApprox)))
}

func renderAccounting(snap engine.Snapshot, tier Tier, s styles) []string {
	input := formatCount(snap.Global.TodayInput, snap.Global.TodayApprox)
	output := formatCount(snap.Global.TodayOutput, snap.Global.TodayApprox)
	detail := cacheDetail(snap.Global)
	if tier == TierNarrow && detail != "" {
		return []string{
			fmt.Sprintf(" INPUT  %s", input),
			"        " + s.dim.Render(detail),
			fmt.Sprintf(" OUTPUT %s", output),
		}
	}
	line := fmt.Sprintf(" INPUT  %s", input)
	if detail != "" {
		line += " = " + s.dim.Render(detail)
	}
	return []string{line, fmt.Sprintf(" OUTPUT %s", output)}
}

func cacheDetail(global engine.Global) string {
	known := global.TodayCacheKnownInput
	if global.TodayInput == 0 {
		return ""
	}
	if known == 0 {
		return fmt.Sprintf("%s unknown · cache telemetry unavailable", formatCount(global.TodayInput, global.TodayApprox))
	}
	read := global.TodayCacheRead
	if read > known {
		read = known
	}
	uncached := known - read
	parts := []string{
		fmt.Sprintf("%s cached", formatCount(read, global.TodayApprox)),
		fmt.Sprintf("%s uncached", formatCount(uncached, global.TodayApprox)),
	}
	if known < global.TodayInput {
		parts = append(parts, fmt.Sprintf("%s unknown", formatCount(global.TodayInput-known, global.TodayApprox)))
	}
	ratio := float64(read) / float64(known)
	percentLabel := fmt.Sprintf("%.1f%% cached", ratio*100)
	if known < global.TodayInput {
		percentLabel += " where known"
	}
	return strings.Join(parts, " + ") + " · " + percentLabel
}

func renderRecent(snap engine.Snapshot, tier Tier, s styles) string {
	if tier == TierNarrow {
		return fmt.Sprintf(" RECENT  5M %s · %s   15M %s · %s",
			formatCount(snap.Global.Tokens5m, false), formatRate(snap.Global.Rate5m),
			formatCount(snap.Global.Tokens15m, false), formatRate(snap.Global.Rate15m))
	}
	return fmt.Sprintf(" RECENT       5M %s · %s       15M %s · %s",
		s.bright.Render(formatCount(snap.Global.Tokens5m, false)), formatRate(snap.Global.Rate5m),
		s.bright.Render(formatCount(snap.Global.Tokens15m, false)), formatRate(snap.Global.Rate15m))
}

func renderHarness(snap engine.Snapshot, width int, tier Tier, compactHeight bool, s styles) []string {
	if width > 100 {
		width = 100
	}
	labelW := 7
	headerLabel := "HARNESS"
	if tier == TierNarrow {
		labelW = 3
		headerLabel = "H"
	}
	todayW, recentW, pctW := 8, 8, 6
	barW := width - 1 - labelW - 2 - 1 - pctW - 2 - todayW - 2 - recentW
	if barW < 4 {
		barW = 4
	}
	if barW > 48 {
		barW = 48
	}
	header := fmt.Sprintf(" %s  %s  %s  %s",
		padRight(headerLabel, labelW),
		padRight("TODAY SHARE", barW+pctW+1),
		padLeft("TODAY", todayW),
		padLeft("15M", recentW))
	lines := []string{s.dim.Render(header)}
	for _, src := range snap.Sources {
		if compactHeight && src.Today == 0 && src.Tokens15m == 0 && src.Health.State != telemetry.HealthDegraded && src.Health.State != telemetry.HealthFailed {
			continue
		}
		label := strings.ToUpper(harnessLabel(src.Name, tier != TierNarrow))
		bar := renderShareBar(src.ShareToday, barW, src.Name, s)
		line := fmt.Sprintf(" %s  %s %s  %s  %s",
			s.source(src.Name, src.Today > 0 || src.Tokens15m > 0).Render(padRight(label, labelW)),
			bar,
			padLeft(formatShare(src.ShareToday), pctW),
			padLeft(formatCount(src.Today, src.Health.TodayIncomplete), todayW),
			padLeft(formatCount(src.Tokens15m, false), recentW))
		lines = append(lines, line)
	}
	return lines
}

func renderShareBar(share float64, width int, name telemetry.SourceName, s styles) string {
	if share <= 0 {
		return strings.Repeat(" ", width)
	}
	filled := int(share*float64(width) + 0.5)
	if filled < 1 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return s.source(name, true).Render(strings.Repeat("█", filled)) + s.dim.Render(strings.Repeat("░", width-filled))
}

func formatShare(share float64) string {
	if share <= 0 {
		return "0%"
	}
	if share >= 1 {
		return "100%"
	}
	return fmt.Sprintf("%.1f%%", share*100)
}

func renderHealthDetails(snap engine.Snapshot, s styles) []string {
	var lines []string
	for _, src := range snap.Sources {
		if src.Health.State != telemetry.HealthDegraded && src.Health.State != telemetry.HealthFailed {
			continue
		}
		name := strings.ToUpper(string(src.Name))
		detail := src.Health.Detail
		if detail == "" {
			detail = string(src.Health.State)
		}
		lines = append(lines, " "+s.warn.Render(fmt.Sprintf("%s ! %s", name, detail)))
	}
	return lines
}

func attributionHeader(label string, width int, tier Tier) string {
	if width > 100 {
		width = 100
	}
	sourceW := 7
	sourceLabel := "HARNESS"
	if tier == TierNarrow {
		sourceW = 3
		sourceLabel = "H"
	}
	todayW, recentW, lastW := 8, 8, 5
	labelW := width - 1 - 2 - sourceW - 2 - todayW - 2 - recentW - 2 - lastW
	if labelW < 8 {
		labelW = 8
	}
	return fmt.Sprintf(" %s  %s  %s  %s  %s",
		padRight(strings.ToUpper(label), labelW),
		padRight(sourceLabel, sourceW),
		padLeft("TODAY", todayW),
		padLeft("15M", recentW),
		padLeft("LAST", lastW))
}

func renderAttribution(row engine.AttributionSnapshot, width int, tier Tier, opt Options, s styles) string {
	if width > 100 {
		width = 100
	}
	sourceW := 7
	if tier == TierNarrow {
		sourceW = 3
	}
	todayW, recentW, lastW := 8, 8, 5
	labelW := width - 1 - 2 - sourceW - 2 - todayW - 2 - recentW - 2 - lastW
	if labelW < 8 {
		labelW = 8
	}
	label := row.Label
	if label == "" {
		label = "unattributed"
	}
	harness := harnessLabel(row.Source, tier != TierNarrow)
	hot := row.Tokens15m > 0
	return fmt.Sprintf(" %s  %s  %s  %s  %s",
		padRight(trunc(label, labelW), labelW),
		s.source(row.Source, hot).Render(padRight(harness, sourceW)),
		padLeft(formatCount(row.Today, row.TodayApprox), todayW),
		padLeft(formatCount(row.Tokens15m, false), recentW),
		padLeft(formatAge(opt.Now, row.LastEvent), lastW))
}

func visibleAttributions(rows []engine.AttributionSnapshot, max int) ([]engine.AttributionSnapshot, int) {
	if max < 0 {
		max = 0
	}
	if len(rows) <= max {
		return rows, 0
	}
	return rows[:max], len(rows) - max
}

func fitWidth(value string, width int) string {
	lines := strings.Split(value, "\n")
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
