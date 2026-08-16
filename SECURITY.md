# Security

Token Top reads local harness logs and never opens a network connection at runtime. There is no account, no telemetry, and no writable application database.

Report vulnerabilities privately through [GitHub security advisories](https://github.com/markcmarshall/token-top/security/advisories/new). Do not open a public issue for an exploitable parser, path, or terminal-escape bug.

Please include:

- affected version or commit
- source involved, if any (Claude Code, Codex, Grok)
- a sanitized fixture or reproduction that contains no prompt, completion, tool, credential, or source-code content

The parsers must not panic on malformed or adversarial JSON, must not echo raw source records in errors, and must not treat negative or reset-derived deltas as usage.
