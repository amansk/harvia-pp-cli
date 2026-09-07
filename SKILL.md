---
name: harvia-pp-cli
description: Agent-native Harvia MyHarvia 2 / Fenix CLI. Cognito auth, heater control, optional SQLite telemetry, JSON output.
---

# harvia-pp-cli

Local Printing Press-style CLI. **Not** a Worker or hosted scheduler. Never
print passwords or tokens. `on` powers a real 240V heater.

## Install

```bash
go install github.com/amansk/harvia-pp-cli/cmd/harvia-pp-cli@latest
# or from a checkout:
go build -o harvia-pp-cli ./cmd/harvia-pp-cli
```

Verify: `harvia-pp-cli --help`

## Auth

User needs a MyHarvia **account password** (Apple-only SSO will fail).

```bash
harvia-pp-cli auth login --env-file ~/.config/harvia/env --json
# or env:
# HARVIA_USERNAME / HARVIA_PASSWORD
harvia-pp-cli auth status --json
harvia-pp-cli doctor --agent
```

`doctor` is green after login without `--live`. `--live` is read-only
(endpoints + devices). Never use `--live` as an excuse to turn the heater on.

If `auth status` shows `present: false`, stop and ask the user for credentials.
Do not invent tokens.

## Control

Temps are Celsius. Mid-session `temp` uses Custom slot 3 (required by the API).

```bash
harvia-pp-cli status --json
harvia-pp-cli on --yes --temp 82 --duration 60 --agent
harvia-pp-cli temp 88 --json
harvia-pp-cli duration 60 --json
harvia-pp-cli lights on --json
harvia-pp-cli fan off --json
harvia-pp-cli off --agent
harvia-pp-cli watch --interval 15 --count 3 --json
harvia-pp-cli devices --json
harvia-pp-cli raw state --json
```

`--agent` = compact JSON + no prompts + `--yes`.

For `on` without `--agent`, pass `--yes` after the user confirms they intend
to heat. `--no-input` without `--yes` must fail.

If they also run sauna-cloud, do **not** add or suggest schedule commands.

## History (local SQLite)

```bash
harvia-pp-cli history samples --limit 20 --json
harvia-pp-cli history sessions --agent
harvia-pp-cli history hours --by week --json
```

## CI / fixtures (no live Harvia)

```bash
harvia-pp-cli --home "$TMPDIR/harvia-pp" --env-file testdata/fixtures/env auth login --json
harvia-pp-cli --home "$TMPDIR/harvia-pp" doctor --agent
```

Unit tests inject a mock HTTP server. `go test ./...` must never contact
`api.harvia.io` or send a live `SAUNA on`.

## Rules

- Never print `idToken`, `accessToken`, `refreshToken`, or passwords.
- Exit 4 = auth, 3 = missing device, 5 = API, 2 = usage / refused `on`, 7 = transient.
- Device UUID is in `name` on `GET /devices`. Use `devices --json`.
- Wire temps are Celsius (32-90).
