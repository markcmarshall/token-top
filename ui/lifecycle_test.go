package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type panicModel struct{}

func (panicModel) Init() tea.Cmd {
	return func() tea.Msg { panic("boom") }
}

func (m panicModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }

func (panicModel) View() tea.View {
	v := tea.NewView("panic")
	v.AltScreen = true
	return v
}

func TestProgramRecoversPanic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p := tea.NewProgram(panicModel{}, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithContext(ctx))
	_, err := p.Run()
	if err == nil {
		t.Fatal("expected panic or kill after source panic")
	}
	if !errors.Is(err, tea.ErrProgramPanic) && !errors.Is(err, tea.ErrProgramKilled) {
		t.Fatalf("err %v", err)
	}
}

func TestQuitCommandIsQuitMsg(t *testing.T) {
	m := testModel()
	_, cmd := m.Update(tea.KeyPressMsg{Text: "q", Code: 'q'})
	if cmd == nil {
		t.Fatal("expected quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q must produce QuitMsg so the alt screen is left")
	}
	_, cmd = m.Update(tea.InterruptMsg{})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("interrupt must produce QuitMsg")
	}
}

func TestNonColorSnapshotHasNoCSI(t *testing.T) {
	var buf bytes.Buffer
	out := Render(testSnap(), Options{Width: 80, Height: 24, Now: time.Date(2026, 8, 16, 20, 32, 14, 0, time.UTC), Color: false, Interval: 2 * time.Second})
	buf.WriteString(out)
	if strings.ContainsRune(buf.String(), '\x1b') {
		t.Fatal("control sequences in no-color snapshot")
	}
}
