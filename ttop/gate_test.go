//go:build !race

package ttop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/markcmarshall/token-top/engine"
	"github.com/markcmarshall/token-top/telemetry"
)

func TestFirstSnapshotUnderTwoSeconds(t *testing.T) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	snap := Snapshot(ctx, Options{})
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("first snapshot %s", elapsed)
	}
	t.Logf("first snapshot %s today=%d approx=%v %s", elapsed, snap.Global.Today, snap.Global.TodayApprox, formatHealth(snap))
}

func TestSteadyRefreshWellWithinInterval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	eng := New(Options{})
	src := Sources()
	eng.PollUntilToday(ctx, src)

	start := time.Now()
	eng.Poll(context.Background(), src)
	elapsed := time.Since(start)
	if elapsed > time.Second {
		t.Fatalf("steady poll %s (must finish well inside 2s)", elapsed)
	}
	t.Logf("steady poll %s", elapsed)
}

func TestOnceBinaryGates(t *testing.T) {
	bin := buildTtop(t)
	start := time.Now()
	out, err := exec.Command(bin, "--once", "--no-color").CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("ttop --once %s", elapsed)
	}
	if !strings.Contains(string(out), "TOKEN TOP") {
		t.Fatalf("output %s", out)
	}
	rss := childMaxRSS(t, bin)
	const limit = 50 << 20
	if rss > limit {
		t.Fatalf("ttop --once RSS %d (%.1f MiB) exceeds 50 MiB", rss, float64(rss)/(1<<20))
	}
	t.Logf("ttop --once %s RSS %d (%.1f MiB)", elapsed, rss, float64(rss)/(1<<20))
}

func TestRealCorpusEventsValidate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	eng := New(Options{})
	eng.PollUntilToday(ctx, Sources())
	snap := eng.Snapshot()
	seen := 0
	for _, src := range snap.Sources {
		if src.Health.State == telemetry.HealthNotDetected {
			continue
		}
		seen++
		if src.Health.State == telemetry.HealthFailed {
			t.Errorf("%s failed: %s", src.Name, src.Health.Detail)
		}
	}
	if seen == 0 {
		t.Skip("no local Claude/Codex/Grok logs")
	}
	t.Logf("%s sessions=%d", formatHealth(snap), len(snap.Sessions))
}

func formatHealth(snap engine.Snapshot) string {
	parts := make([]string, 0, len(snap.Sources))
	for _, s := range snap.Sources {
		parts = append(parts, fmt.Sprintf("%s=%s", s.Name, s.Health.State))
	}
	return strings.Join(parts, " ")
}

func buildTtop(t *testing.T) string {
	t.Helper()
	mod, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "ttop")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/ttop")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if body, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, body)
	}
	return out
}

func childMaxRSS(t *testing.T, bin string) uint64 {
	t.Helper()
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("/usr/bin/time", "-l", bin, "--once", "--no-color")
		body, _ := cmd.CombinedOutput()
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "maximum resident set size") {
				f := strings.Fields(line)
				if len(f) == 0 {
					break
				}
				n, err := strconv.ParseUint(f[0], 10, 64)
				if err != nil {
					t.Fatalf("parse rss %q: %v", line, err)
				}
				return n
			}
		}
		t.Fatalf("no RSS in time output:\n%s", body)
	case "linux":
		cmd := exec.Command("/usr/bin/time", "-f", "%M", bin, "--once", "--no-color")
		body, _ := cmd.CombinedOutput()
		lines := strings.Split(strings.TrimSpace(string(body)), "\n")
		kb, err := strconv.ParseUint(strings.TrimSpace(lines[len(lines)-1]), 10, 64)
		if err != nil {
			t.Fatalf("parse rss %q: %v\n%s", lines[len(lines)-1], err, body)
		}
		return kb * 1024
	default:
		t.Skip("no RSS helper")
	}
	return 0
}
