package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/markcmarshall/token-top/engine"
	"github.com/markcmarshall/token-top/telemetry"
)

func testModel() Model {
	clk := &engine.FixedClock{T: time.Date(2026, 8, 16, 20, 32, 14, 0, time.UTC)}
	return NewModel(engine.New(clk, nil), nil, 2*time.Second, false, 80, 24)
}

func TestQuitOnQ(t *testing.T) {
	m := testModel()
	_, cmd := m.Update(tea.KeyPressMsg{Text: "q", Code: 'q'})
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected QuitMsg")
	}
}

func TestQuitOnCtrlC(t *testing.T) {
	m := testModel()
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected QuitMsg")
	}
}

func TestQuitOnInterrupt(t *testing.T) {
	m := testModel()
	_, cmd := m.Update(tea.InterruptMsg{})
	if cmd == nil {
		t.Fatal("expected quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected QuitMsg")
	}
}

func TestViewUsesAltScreen(t *testing.T) {
	m := testModel()
	m.snap = engine.Snapshot{
		GeneratedAt: time.Date(2026, 8, 16, 20, 32, 14, 0, time.UTC),
		Sources: []engine.SourceSnapshot{
			{Name: telemetry.SourceClaude, Health: telemetry.SourceHealth{State: telemetry.HealthNotDetected}},
		},
	}
	v := m.View()
	if !v.AltScreen {
		t.Fatal("expected alt screen")
	}
	if !strings.Contains(v.Content, "TOKEN TOP") {
		t.Fatalf("view %q", v.Content)
	}
}

func TestViewSupportsPrivateAttributionHeader(t *testing.T) {
	m := testModel().WithAttributionHeader("CLAIM")
	m.snap = engine.Snapshot{GeneratedAt: time.Date(2026, 8, 16, 20, 32, 14, 0, time.UTC)}
	if !strings.Contains(m.View().Content, "CLAIM") {
		t.Fatalf("view %q", m.View().Content)
	}
}

func TestResizeUpdatesBounds(t *testing.T) {
	m := testModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	got := next.(Model)
	if got.width != 140 || got.height != 40 {
		t.Fatalf("%dx%d", got.width, got.height)
	}
}
