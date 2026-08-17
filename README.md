# Token Top

`top` for token usage across local Claude Code, Codex, and Grok sessions.

Open a terminal tab, glance, know what is burning.

This started as the internal token monitor used while building FounderOS, then was carved out as a small standalone MIT tool. It is not a ccusage competitor, a hosted service, or a rewrite of the old FounderOS console. FounderOS may consume this module privately; this repository knows nothing about FounderOS, claims, or Postgres.

![Token Top snapshot](docs/screenshot.png)

```text
go install github.com/markcmarshall/token-top/cmd/ttop@v1.0.0
```

Prebuilt `darwin`/`linux` `amd64`/`arm64` binaries are on the [releases](https://github.com/markcmarshall/token-top/releases) page. Binary: `ttop`. Screen title: `TOKEN TOP`.

## What it shows

Live trailing completed token usage for local agent sessions:

- 1-minute, 5-minute, and 15-minute burn
- which harness, model, and project owns each session
- input / output / cache composition
- whether the source telemetry is healthy enough to trust

Rates come from event timestamps in harness logs, not from screen refresh deltas. Tokens are directional workload counts. They are not money, quota, or normalized compute.

## Limitations

- Claude Code, Codex, and Grok only. No Windows, remote hosts, or custom log paths.
- Passive: `q` and Ctrl-C quit. No sorting, filters, drill-down, or session control.
- No cost, history, alerts, config file, or daemon.
- Grok updates when a turn completes; missing update logs degrade that source instead of inventing zeros.
- First-render lifetime totals may show `~` while older files are still indexing.

## Privacy

`ttop` is local and read-only. It makes no network requests, stores no database, and sends no telemetry.

It reads only these default directories:

| Source | Paths |
| --- | --- |
| Claude Code | `~/.claude/projects/` |
| Codex | `~/.codex/sessions/`, `~/.codex/archived_sessions/` |
| Grok | `~/.grok/sessions/` |

Parsers extract identity, timestamp, model, CWD, and usage metadata. Prompt, completion, tool argument, and source-code content is never rendered, persisted, transmitted, or logged. Full absolute paths are not shown by default.

## Usage

```text
ttop
ttop --once
ttop --interval 2s --no-color
```

| Flag | Meaning |
| --- | --- |
| `--interval DURATION` | refresh interval; default `2s` |
| `--no-color` | disable ANSI color |
| `--once` | print one snapshot and exit |
| `--help` | usage |
| `--version` | build version |

On a TTY, `ttop` runs a live alternate-screen monitor. `q` or Ctrl-C quits and restores the terminal. Non-TTY stdout is a plain snapshot, equivalent to `--once --no-color`.

## Library

The module is the canonical core. Inject attribution; do not fork the engine.

```go
import "github.com/markcmarshall/token-top/ttop"

eng := ttop.New(ttop.Options{Attributor: myAttributor})
snap := ttop.Snapshot(ctx, ttop.Options{Attributor: myAttributor})
```

`myAttributor` implements `attribution.Attributor`. Nil uses standalone git-root / CWD basename. Public types know nothing about FounderOS, Postgres, or claims.

## Architecture

```text
source adapters
    ↓
canonical token events
    ↓
event attribution
    ↓
session reducer
    ↓
immutable snapshot
    ↓
responsive renderer
```

## License

MIT. See [LICENSE](LICENSE).

Security reports: [SECURITY.md](SECURITY.md).
Contributions: [CONTRIBUTING.md](CONTRIBUTING.md).
