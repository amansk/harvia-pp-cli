---
name: pp-harvia
description: "Control a Harvia MyHarvia 2 / Fenix WiFi sauna heater from the terminal — status, confirm-gated heat-on, mid-session temperature via the Custom profile slot, lights, fan, live watch, and a local SQLite history of sessions and hours-on. Trigger phrases: `turn on the sauna`, `heat the sauna to 82`, `what's my sauna doing`, `turn the sauna off`, `sauna lights on`, `how many sauna hours this month`, `use harvia`, `run harvia-pp-cli`."
author: "Amandeep Khurana"
license: "Apache-2.0"
argument-hint: "<command> [args]"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - harvia-pp-cli
    install:
      - kind: go
        bins: [harvia-pp-cli]
        module: github.com/amansk/harvia-pp-cli/cmd/harvia-pp-cli
---

# Harvia MyHarvia 2 / Fenix — Printing Press CLI

Local Go binary. **Not** a Cloudflare Worker, PWA, or hosted scheduler. Never
print passwords or tokens. `on` powers a real **240V** heater.

## Prerequisites: Install the CLI

This skill drives the `harvia-pp-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install harvia --cli-only
   ```
2. Verify: `harvia-pp-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.6 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/devices/harvia/cmd/harvia-pp-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Until the library entry lands, the same binary installs from the source repo: `go install github.com/amansk/harvia-pp-cli/cmd/harvia-pp-cli@latest`.

## Auth

The user needs a MyHarvia **account password**. Apple-only or Google-only
sign-in with no password set will fail with exit 4; tell them to set a
password in the MyHarvia 2 app.

```bash
harvia-pp-cli auth login --env-file ~/.config/harvia/env --json
# or env: HARVIA_USERNAME / HARVIA_PASSWORD
harvia-pp-cli auth status --json
harvia-pp-cli doctor --agent
```

`doctor` is green after login without `--live`. `--live` is read-only
(endpoints + devices). Never use `--live` as an excuse to turn the heater on.

If `auth status` shows `present: false`, stop and ask the user for
credentials. Do not invent tokens.

## Control

Temperatures are **Celsius** (82C = 180F, 88C = 190F, max 90C). Mid-session
`temp` uses Custom profile slot 3, which the API requires.

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

`--agent` = compact JSON + no prompts + no color + `--yes`.

For `on` without `--agent`, pass `--yes` only after the user has confirmed
they intend to heat. `--no-input` without `--yes` fails with exit 2 and
sends no command.

If the user also runs a separate sauna scheduler that writes the Custom
profile slot, do **not** add or suggest schedule commands here. Two writers
race on slot 3.

## History (local SQLite)

```bash
harvia-pp-cli history samples --limit 20 --json
harvia-pp-cli history sessions --agent
harvia-pp-cli history hours --by week --json
```

Samples are appended by `status`, `watch`, and post-control reads. A rising
`heater_on` opens a session; a falling edge closes it with peak temperature.
`--no-store` skips SQLite.

## Output and exit codes

`--json` / `--agent` emit `{ok, data, error?}`. Errors go to stderr as
`{ok:false, error}`.

| Code | Meaning |
| --- | --- |
| 0 | ok |
| 2 | usage / refused `on` |
| 3 | no device |
| 4 | auth / missing session |
| 5 | API / parse |
| 7 | transient (network, HTTP 5xx) |

## Rules

- Never print `idToken`, `accessToken`, `refreshToken`, or passwords.
- Device UUID is in `name` on `GET /devices`. Use `devices --json`.
- Wire temps are Celsius (32-90).
- `go test ./...` never contacts `api.harvia.io` and never sends `SAUNA on`.

## CI / fixtures (no live Harvia)

```bash
harvia-pp-cli --home "$TMPDIR/harvia-pp" --env-file testdata/fixtures/env auth login --json
harvia-pp-cli --home "$TMPDIR/harvia-pp" doctor --agent
```
