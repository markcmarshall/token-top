package ui

import (
	"context"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/markcmarshall/token-top/engine"
	"github.com/markcmarshall/token-top/telemetry"
)

type Model struct {
	poller            *Poller
	snap              engine.Snapshot
	interval          time.Duration
	color             bool
	width             int
	height            int
	attributionHeader string
}

// WithAttributionHeader changes the work-aggregate column label. Standalone
// Token Top defaults to PROJECT; private compositions may use another subject.
func (m Model) WithAttributionHeader(label string) Model {
	m.attributionHeader = label
	return m
}

type Poller struct {
	mu      sync.Mutex
	engine  *engine.Engine
	sources []telemetry.Source
	startup bool
}

func NewModel(eng *engine.Engine, sources []telemetry.Source, interval time.Duration, color bool, width, height int) Model {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	return Model{
		poller:   &Poller{engine: eng, sources: sources, startup: true},
		interval: interval,
		color:    color,
		width:    width,
		height:   height,
	}
}

func (m Model) Init() tea.Cmd {
	return pollCmd(m.poller, m.interval)
}

type tickMsg time.Time
type snapMsg engine.Snapshot

func pollCmd(p *Poller, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		return snapMsg(p.Snapshot(timeout))
	}
}

func (p *Poller) Snapshot(timeout time.Duration) engine.Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if p.startup {
		p.startup = false
		return p.engine.PollUntilToday(ctx, p.sources)
	}
	p.engine.Poll(ctx, p.sources)
	return p.engine.Snapshot()
}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tea.InterruptMsg:
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case snapMsg:
		m.snap = engine.Snapshot(msg)
		return m, tickCmd(m.interval)
	case tickMsg:
		return m, pollCmd(m.poller, m.interval)
	}
	return m, nil
}

func (m Model) View() tea.View {
	v := tea.NewView(Render(m.snap, Options{
		Width:             m.width,
		Height:            m.height,
		Now:               m.snap.GeneratedAt,
		Color:             m.color,
		Interval:          m.interval,
		AttributionHeader: m.attributionHeader,
	}))
	v.AltScreen = true
	return v
}
