# harvia-pp-cli

Agent-native Harvia **MyHarvia 2 / Fenix** CLI (Printing Press style): Cognito
auth once, local heater control, optional SQLite telemetry, JSON for agents.

This is a **local binary**. It is not a Cloudflare Worker, D1 app, PWA, or
hosted scheduler. If you already run
[sauna-cloud](https://github.com/amansk/sauna-cloud), do **not** add schedule
commands here. Two writers race on the heater's Custom profile slot.

See [PLAN.md](PLAN.md) for verified wire shapes and traps. Agents: [SKILL.md](SKILL.md).

## Install

Requires Go 1.22+.

```bash
go install github.com/amansk/harvia-pp-cli/cmd/harvia-pp-cli@latest
```

From a checkout:

```bash
go build -o harvia-pp-cli ./cmd/harvia-pp-cli
```

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
printed**. The same directory is used by sauna-cloud's Python CLI, so a cached
token can be reused. Override with `--home` or `$HARVIA_PP_HOME`.

## Safety

`on` powers a real **240V** sauna. It requires `--yes`, `--agent`, or an
interactive confirmation. `--no-input` without `--yes` refuses.

`doctor --live` is read-only (endpoints + device list). It never sends
`SAUNA on`. CI and unit tests use mocked HTTP only.

Temperatures are **Celsius** on the wire (82C = 180F, 88C = 190F, 90C max).

## Commands

```bash
harvia-pp-cli status
harvia-pp-cli on --yes --temp 82 --duration 60
harvia-pp-cli off
harvia-pp-cli temp 88                 # mid-session uses Custom slot 3
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
`--yes --no-input --no-color`.

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

Documented in PLAN.md, measured in sauna-cloud:

1. Device UUID is in `name`, not `deviceId`, on `GET /devices`.
2. Mid-session temp changes write Custom profile slot **3** then activate it.
   A bare `PATCH /devices/target` is ignored while the heater runs.
3. `GET https://api.harvia.io/endpoints` (hosts rotate; not cached to disk).
4. Cognito refresh may omit a new refresh token. The clock does not reset.
5. Temps are Celsius on the wire.

## Future Printing Press library

This repo is the standalone proving ground. A later contribution to Printing
Press should land under `library/devices/harvia` (client, fixtures, traps)
so other PP CLIs can reuse the same device module. v1 stays
`github.com/amansk/harvia-pp-cli`.

## Credits

Reverse-engineering (read-only reference, not copied into this binary):

- [amansk/sauna-cloud](https://github.com/amansk/sauna-cloud) (`cli/harvia`, Worker `src/harvia` notes)
- [WiesiDeluxe/ha-harvia-sauna](https://github.com/WiesiDeluxe/ha-harvia-sauna)
- [TommyJuuti/harvia-home-assistant-plugin](https://github.com/TommyJuuti/harvia-home-assistant-plugin)

Harvia's cloud API is unofficial and can change without notice.
