# Contributing

Token Top is a small, opinionated utility. The product law is:

> Open a terminal tab, glance, know what is burning.

If a change does not improve that answer, it does not belong here.

## Fit

Welcome:

- correctness in Claude Code, Codex, or Grok telemetry
- fixture coverage for known harness anomalies
- parser hardening and privacy gaps
- renderer bugs that break the glance

Not welcome:

- new providers
- costing, history, dashboards, alerts, or session control
- configuration, themes, or interactive navigation
- Windows, remote collection, or custom log-path flags
- FounderOS-specific catalog or claim logic

## Checks

```text
gofmt -l .
go vet ./...
go test -race ./...
CGO_ENABLED=0 go build -o /tmp/ttop ./cmd/ttop
```

Consumer API: `github.com/markcmarshall/token-top/ttop`. Do not add FounderOS types there.

Do not add linter or release frameworks. Keep fixtures free of real prompt, completion, tool, credential, and source-code content.
