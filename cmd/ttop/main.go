package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"
)

// Set by release builds via -ldflags "-X main.version=...".
var version = "dev"
var readBuildInfo = debug.ReadBuildInfo

func resolvedVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := readBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ttop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	interval := fs.Duration("interval", 2*time.Second, "refresh interval")
	noColor := fs.Bool("no-color", false, "disable ANSI color")
	once := fs.Bool("once", false, "print one snapshot and exit")
	showVersion := fs.Bool("version", false, "build version")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: ttop [flags]\n\n")
		fmt.Fprintf(stderr, "top for token usage across local Claude Code, Codex, and Grok sessions.\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "ttop: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, resolvedVersion())
		return 0
	}

	tty := isTTY(stdout)
	color := !*noColor && tty
	if *once || !tty {
		return snapshotOnce(stdout, color, *interval)
	}
	return runLive(stdout, stderr, color, *interval)
}
