# harvia-pp-cli

[![test](https://github.com/amansk/harvia-pp-cli/actions/workflows/test.yml/badge.svg)](https://github.com/amansk/harvia-pp-cli/actions/workflows/test.yml)

**Control a Harvia MyHarvia 2 / Fenix WiFi sauna heater from the terminal,
plus a local SQLite history of sessions and hours-on that Harvia's cloud
never keeps.**

Agent-native, [Printing Press](https://printingpress.dev/)-style Go CLI:
Cognito auth once, confirm-gated heat-on, mid-session temperature via the
Custom profile slot, lights, fan, live watch, and `--json` / `--agent` on
every command with typed exit codes.

This is a **local binary**. It is not a Cloudflare Worker, PWA, or hosted
scheduler. If you already run a separate sauna scheduler that writes the
heater's Custom profile slot, do **not** add schedule commands here. Two
writers race on the same slot.

See [PLAN.md](PLAN.md) for verified wire shapes and API traps. Agents:
[SKILL.md](SKILL.md).

## Install

Requires Go 1.22+.

```bash
go install github.com/amansk/harvia-pp-cli/cmd/harvia-pp-cli@latest
```

From a checkout:

```bash
go build -o harvia-pp-cli ./cmd/harvia-pp-cli
```

Verify: `harvia-pp-cli --version`.

## Account (password, not Apple-only SSO)

Harvia's REST wrapper exchanges an **email + password** for a Cognito token.
Apple-only or Google-only MyHarvia sign-in with no password set will 401.

1. In the MyHarvia 2 app, set a real account password for the same email.
2. Put credentials in `~/.config/harvia/env` (mode `600`), or pass `--env-file`:

```
HARVIA_USERNAME=you@example.com
HARVIA_PASSWORD=your-myharvia-2-password
```

These grant full heater control and can expose home coordinates on the device
list. Treat them like a house key. Never commit them.

```bash
harvia-pp-cli auth login --env-file ~/.config/harvia/env
harvia-pp-cli auth status
harvia-pp-cli doctor
```

Tokens are stored at `~/.config/harvia/token.json` mode `0600` and are **never
printed**. Override the directory with `--home` or `$HARVIA_PP_HOME`.

## Safety

`on` powers a real **240V** sauna. It requires `--yes`, `--agent`, or an
interactive confirmation. `--no-input` without `--yes` refuses with exit 2
and sends no command.

`doctor --live` is read-only (endpoints + device list). It never sends
`SAUNA on`. CI and unit tests use mocked HTTP only.

Temperatures are **Celsius** on the wire (82C = 180F, 88C = 190F, 90C max).

## Quick start

```bash
harvia-pp-cli status
harvia-pp-cli on --yes --temp 82 --duration 60
harvia-pp-cli temp 88                 # mid-session uses Custom slot 3
harvia-pp-cli off
```

## Commands

```bash
harvia-pp-cli status
harvia-pp-cli on --yes --temp 82 --duration 60
harvia-pp-cli off
harvia-pp-cli temp 88
harvia-pp-cli duration 60
harvia-pp-cli lights on
harvia-pp-cli fan off
harvia-pp-cli watch --interval 30
harvia-pp-cli devices
harvia-pp-cli raw endpoints
harvia-pp-cli history samples --json
harvia-pp-cli history sessions
harvia-pp-cli history hours --by week
```

`--json` / `--agent` emit `{ok, data, error?}`. `--agent` also implies
`--yes --no-input --no-color` and compact output.

Optional SQLite (`~/.config/harvia/harvia.db`) appends status samples and
session summaries. `--no-store` skips it.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | ok |
| 2 | usage / refused confirm |
| 3 | not found |
| 4 | auth / missing session |
| 5 | API / parse |
| 7 | transient |

## API traps (verified)

Documented in [PLAN.md](PLAN.md), measured against a live Fenix heater and
cross-checked with public Home Assistant clients:

1. Device UUID is in `name`, not `deviceId`, on `GET /devices`.
2. Mid-session temp changes write Custom profile slot **3** then activate it.
   A bare `PATCH /devices/target` is ignored while the heater runs.
3. `GET https://api.harvia.io/endpoints` (hosts rotate; not cached to disk).
4. Cognito refresh may omit a new refresh token. The clock does not reset.
5. Temps are Celsius on the wire.

## Development

```bash
go test ./...
go vet ./...
go build -o harvia-pp-cli ./cmd/harvia-pp-cli
harvia-pp-cli --home /tmp/harvia-pp --env-file testdata/fixtures/env auth login
harvia-pp-cli --home /tmp/harvia-pp doctor
```

Tests inject a mock Harvia server (`internal/fixture`) and never contact
`api.harvia.io`. Release builds use [`.goreleaser.yaml`](.goreleaser.yaml);
the Printing Press manifest is [`.printing-press.json`](.printing-press.json).

## Printing Press

This repo is laid out for contribution to the
[Printing Press Library](https://github.com/mvanhorn/printing-press-library)
under `library/devices/harvia`: CLI source, `.printing-press.json`,
`.goreleaser.yaml`, `SKILL.md`, and `.manuscripts/`. Until it lands there,
install from this module directly (above).

## Credits

Reverse-engineering references (read-only, not copied into this binary):

- [WiesiDeluxe/ha-harvia-sauna](https://github.com/WiesiDeluxe/ha-harvia-sauna)
- [TommyJuuti/harvia-home-assistant-plugin](https://github.com/TommyJuuti/harvia-home-assistant-plugin)
- The author's earlier private Python client and Worker notes, from which the
  wire shapes in PLAN.md were verified.

Harvia's cloud API is unofficial and can change without notice. This project
is not affiliated with or endorsed by Harvia.

## License

[MIT](LICENSE) © 2026 Amandeep Khurana
