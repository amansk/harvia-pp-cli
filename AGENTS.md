# harvia-pp-cli Agent Guide

This directory is the `harvia-pp-cli` Printing Press-style CLI for Harvia
MyHarvia 2 / Fenix WiFi sauna heaters. Keep edits narrow; the wire shapes in
PLAN.md are verified against a live heater and the tests pin them.

## Operating contract

Read state before writing, and never turn the heater on without an explicit
user intent:

```bash
harvia-pp-cli doctor --agent
harvia-pp-cli status --json
harvia-pp-cli on --yes --temp 82 --duration 60 --agent
harvia-pp-cli off --agent
```

- `on` powers a real 240V heater. It requires `--yes`, `--agent`, or an
  interactive confirmation. Non-TTY stdin without `--yes` exits 2 and sends
  nothing.
- Never print `idToken`, `accessToken`, `refreshToken`, or passwords. Tests
  assert this; keep them passing.
- Temperatures are Celsius on the wire (32-90).
- `go test ./...` must never contact `api.harvia.io`. Use `internal/fixture`.

## Layout

| Path | Role |
| --- | --- |
| `cmd/harvia-pp-cli` | entry point |
| `internal/cli` | cobra commands, confirm gate, output modes |
| `internal/client` | endpoint discovery, Cognito login/refresh, REST calls |
| `internal/actions` | warmup order, Custom-slot temp, status summary |
| `internal/store` | optional SQLite samples and sessions |
| `internal/fixture` | mocked Harvia server for tests |
| `testdata/fixtures` | fake endpoints, tokens, devices, state, telemetry |

## Checks before pushing

```bash
gofmt -l .
go vet ./...
go test ./...
golangci-lint run ./...
```

For install, examples, and user-facing guidance read `README.md` and
`SKILL.md`. Record any customization intended to survive a future reprint in
`.printing-press-patches/`.
