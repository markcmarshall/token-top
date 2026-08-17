package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	xterm "github.com/charmbracelet/x/term"

	"github.com/markcmarshall/token-top/engine"
	"github.com/markcmarshall/token-top/ttop"
	"github.com/markcmarshall/token-top/ui"
)

func snapshotOnce(stdout io.Writer, color bool, interval time.Duration) int {
	clk := engine.SystemClock{}
	width, height := termSize(stdout)
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), interval)
	defer cancel()
	snap := ttop.Snapshot(ctx, ttop.Options{Clock: clk})
	fmt.Fprint(stdout, ui.Render(snap, ui.Options{
		Width:    width,
		Height:   height,
		Now:      clk.Now(),
		Color:    color,
		Interval: interval,
	}))
	return 0
}

func runLive(stdout, stderr io.Writer, color bool, interval time.Duration) int {
	clk := engine.SystemClock{}
	eng := ttop.New(ttop.Options{Clock: clk})
	width, height := termSize(stdout)
	m := ui.NewModel(eng, ttop.Sources(), interval, color, width, height)
	p := tea.NewProgram(m, tea.WithOutput(stdout))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(stderr, "ttop: %v\n", err)
		return 1
	}
	return 0
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

func termSize(w io.Writer) (int, int) {
	f, ok := w.(*os.File)
	if !ok {
		return 80, 24
	}
	width, height, err := xterm.GetSize(f.Fd())
	if err != nil || width <= 0 {
		return 80, 24
	}
	if height <= 0 {
		height = 24
	}
	return width, height
}
