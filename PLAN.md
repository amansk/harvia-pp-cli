# PLAN.md — harvia-pp-cli v1

Agent-native Printing Press–style CLI for **Harvia MyHarvia 2 / Fenix WiFi**
sauna heaters. Local binary: Cognito auth once, control + optional SQLite
telemetry, JSON for agents.

This is **not** a Cloudflare Worker, D1 app, PWA, or hosted scheduler. The
author runs a separate private stack for that. This repo is the independent
second opinion: a Go CLI that talks to Harvia directly, in the standard
Printing Press `*-pp-cli` shape.

Wire shapes below are **verified** against the author's earlier Python client
and Worker notes (measured on a live Fenix heater), and cross-checked with
public Home Assistant clients. Do not invent extra GraphQL or LAN paths for v1.

## Goals

- Auth once (MyHarvia email + password), cache Cognito tokens under
  `~/.config/harvia/` mode `0600`, never print secrets.
- Control and inspect one heater: status, on/off, temp, duration, lights, fan,
  watch, devices, raw diagnostics.
- `--json` / `--agent` on every command.
- `doctor` green after fixture auth (no live Harvia, no live heat-on).
- Optional SQLite samples + session summaries for offline `history`.

## Non-goals (v1)

- Cloudflare Workers, D1, wrangler, PWA, Siri, hosted scheduling.
- A second cloud scheduler. If the user also runs a separate sauna
  scheduler, **do not add `schedule` / `serve` / `launchd` commands**. Two
  writers race on Custom profile slot 3.
- Incidental live `SAUNA on` in CI or `doctor --live`.
- Apple/Google SSO-only accounts (no password to exchange). Document the
  MyHarvia account-password requirement.
- Official public API (there is none). LAN control (Fenix is cloud-only).

---

## Auth

Harvia fronts AWS Cognito in `eu-central-1` behind a REST wrapper.

### Credential sources (first hit wins per key; real env vars win)

| Source | Notes |
| --- | --- |
| Process env | `HARVIA_USERNAME`, `HARVIA_PASSWORD` |
| `--env-file PATH` / `$HARVIA_ENV_FILE` | `KEY=value` lines, `#` comments |
| `~/.config/harvia/env` (or `$HARVIA_PP_HOME/env`) | Preferred long-term home; mode `0600` |
| TTY prompt | Email, then silent password. `--no-input` / `--agent` refuse prompts |

Apple-only or Google-only MyHarvia sign-in with **no password set** cannot
authenticate. The user must set a real password in the MyHarvia 2 app.

### Token flow (verified)

1. Unauthenticated `GET https://api.harvia.io/endpoints` (override:
   `$HARVIA_ENDPOINTS_URL`). Hosts rotate; re-fetch every process start, do
   not persist the map.
2. `POST {RestApi.generics.https}/auth/token` body
   `{"username","password"}` → `{idToken, accessToken, refreshToken, expiresIn}`.
   `idToken` is the Bearer (~1h).
3. `POST {RestApi.generics.https}/auth/refresh` body
   `{"refreshToken","email"}` → `{accessToken, idToken, expiresIn}` **and no
   new refresh token**. Measured 2026-07-24 on a live account. Keep the
   old refresh token. Cognito measures refresh
   validity from issuance, so the clock never resets.
4. On HTTP 401, retry the request **once** after a forced re-login.
   If refresh fails, re-login from the env/prompt password. Do not store
   the password in `token.json`.

### Storage

- Home: `$HARVIA_PP_HOME` or `--home`, else `~/.config/harvia`.
- `token.json` mode `0600`. Compatible fields: `idToken`, `accessToken`,
  `refreshToken`, `expiresIn`, `fetchedAt` (unix seconds), `username`.
- Optional `harvia.db` (SQLite).
- CLI output never includes password, idToken, accessToken, or refreshToken
  values (or their lengths).

`auth status` reports username, present flags, expiry remaining, path. Never
values.

---

## Device and control APIs (verified)

Base URLs come from `endpoints.RestApi.{generics,device,data}.https`.

| Op | Method | Path / notes |
| --- | --- | --- |
| Discover | GET | `https://api.harvia.io/endpoints` |
| Login | POST | `{generics}/auth/token` |
| Refresh | POST | `{generics}/auth/refresh` |
| Devices | GET | `{device}/devices?maxResults=100` (+ `nextToken`) |
| State | GET | `{device}/devices/state?deviceId=&subId=C1` |
| Telemetry | GET | `{data}/data/latest-data?deviceId=&cabinId=C1` |
| Command | POST | `{device}/devices/command` body `{deviceId, cabin:{id:"C1"}, command:{type,state}}` |
| Target | PATCH | `{device}/devices/target` body `{deviceId, cabin:{id:"C1"}, temperature?, humidity?, profile?}` |
| Profile | PATCH | `{device}/devices/profile` body `{deviceId, profile}` (string slot) |

Command types: `SAUNA`, `LIGHTS`, `FAN`, `STEAMER`, `ADJUST_DURATION`
(minutes as a number, not `"on"`/`"off"`).

Temperatures are **Celsius on the wire**. Unit range used as last-resort
clamp: 32C–90C (90F–194F). 82C = 180F, 88C = 190F.

### Trap 1: device UUID is in `name`

On `GET /devices`, Fenix rows put the UUID in `name`, not `deviceId`.
Resolution order for v1:

1. If `name` looks like a UUID, use `name`.
2. Else `deviceId`, then `id`, then `name`.

`--device` selects among multiple; default is the first resolved id.

### Trap 2: mid-session temp needs Custom slot 3

A running session follows its **active profile** target. A bare
`PATCH /devices/target` is ignored while the heater is on.

Slots: `0` Mild, `1` Cozy, `2` Hot, `3` Custom (editable; name often empty).

Warmup / mid-session temp order (do not reorder):

1. Read state; copy Custom slot `targetHum` (default 0) so humidity is not
   clobbered.
2. `PATCH /devices/target` with `temperature`, `humidity`, `profile: "3"`.
3. `PATCH /devices/profile` with `profile: "3"`.
4. Optional `ADJUST_DURATION`.
5. Only then `SAUNA on` (for `on`). Lights and fan are plain commands and
   need no profile dance.

When the heater is **off**, a bare target patch is enough for `temp`.
`on --temp` still uses the Custom-slot path so a subsequent session
honors the requested temperature.

### Trap 3: endpoint hosts rotate

Do not cache the endpoint map to disk. One GET per process.

### Trap 4: refresh does not rotate

See auth. Persist the previous `refreshToken`.

---

## Confirm gates (240V)

`on` physically powers a sauna heater. Require one of:

- `--yes`
- `--agent` (implies `--yes --no-input --json --no-color`)
- interactive `y` confirmation on a TTY

`--no-input` without `--yes` refuses `on`. `off`, lights, fan, temp, and
duration do not prompt. `doctor --live` is **read-only** (endpoints +
devices). Tests and CI never call a real `SAUNA on`.

---

## SQLite (optional, local)

File: `$HOME/harvia.db`. WAL. Additive `CREATE TABLE IF NOT EXISTS`.

```sql
CREATE TABLE samples (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  recorded_at TEXT NOT NULL,
  device_id TEXT NOT NULL,
  heater_on INTEGER,
  temp_c REAL,
  target_temp_c REAL,
  light_on INTEGER,
  fan_on INTEGER,
  door_closed INTEGER,
  auto_off_min REAL,
  profile TEXT,
  raw_state_json TEXT,
  raw_telemetry_json TEXT
);

CREATE TABLE sessions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  device_id TEXT NOT NULL,
  started_at TEXT NOT NULL,
  ended_at TEXT,
  peak_temp_c REAL,
  source TEXT NOT NULL DEFAULT 'external'
);

CREATE TABLE meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
```

`status`, `watch`, and post-control reads append a sample. A rising
`heater_on` opens a session; a falling edge closes it and records peak
temp. `history samples|sessions|hours` is the spend-style query surface
(hours on, not money). `--no-store` skips SQLite.

---

## Command surface

Global flags: `--json`, `--agent`, `--quiet`, `--home`, `--env-file`,
`--no-color`, `--no-input`, `--yes`, `--device`.

| Command | Behavior |
| --- | --- |
| `auth login` | Exchange creds, write `token.json` 0600 |
| `auth status` | Username / present / expiry; no secrets |
| `auth logout` | Delete `token.json` |
| `doctor` | Home, token file, mode, SQLite. `--live` read-only ping |
| `status` | Heater, profile, temp C, lights, fan, door, session, errors |
| `on [--temp C] [--duration MIN]` | Confirm gate, then warmup order |
| `off` | `SAUNA off` |
| `temp C` | Mid-session: Custom slot + activate. Off: bare target |
| `duration MIN` | `ADJUST_DURATION` |
| `lights on\|off` | `LIGHTS` |
| `fan on\|off` | `FAN` |
| `watch [--interval S] [--count N]` | Poll telemetry; `--count` for tests |
| `devices` | List + resolved UUID |
| `raw state\|telemetry\|endpoints` | Raw JSON |
| `history samples\|sessions\|hours` | Offline SQLite queries |

`--json` / `--agent` emit `{ok, data, error?}`. Human mode is a compact
table or a few lines.

Typed exit codes (Printing Press): `0` ok, `2` usage, `3` not found,
`4` auth, `5` API, `7` transient.

---

## Layout

```
cmd/harvia-pp-cli/main.go
internal/auth/          token file, env, redaction
internal/client/        HTTP + traps
internal/actions/       warmup, temp, summarize
internal/store/         SQLite
internal/cli/           cobra
internal/output/        JSON envelope + tables
internal/exitcode/
testdata/fixtures/      endpoints, auth, devices, state, telemetry
```

Module: `github.com/amansk/harvia-pp-cli`. Go 1.22, Cobra,
`modernc.org/sqlite` (pure Go).

---

## Tests (mocked HTTP only)

Fixture server rewrites `RestApi.*.https` to itself. Assertions:

- Login stores 0600 `token.json`; status/doctor never echo secrets.
- `on </dev/null` (non-TTY stdin) without `--yes` refuses; `/dev/null` is a
  character device, so TTY detection must use `term.IsTerminal`.
- Device UUID comes from `name` when `deviceId` is missing or not a UUID.
- `on --yes --temp 82` request order: target(profile=3) → profile 3 → SAUNA on.
- Mid-session `temp` uses Custom slot; heater-off `temp` is a bare PATCH.
- Refresh response without `refreshToken` keeps the old one.
- `on` without `--yes`/`--agent` under `--no-input` exits usage, no command POST.
- `doctor` green after fixture login without `--live`.
- SQLite sample + session open/close; `history hours` sums duration.
- `go test ./...` never contacts `api.harvia.io`.

---

## Credits and Printing Press path

Reverse-engineering credit (README):

- [WiesiDeluxe/ha-harvia-sauna](https://github.com/WiesiDeluxe/ha-harvia-sauna)
- [TommyJuuti/harvia-home-assistant-plugin](https://github.com/TommyJuuti/harvia-home-assistant-plugin)
- The author's earlier private Python client and Worker notes.

This CLI is packaged for the Printing Press Library at
`library/devices/harvia` (`.printing-press.json`, `.goreleaser.yaml`,
`SKILL.md`, `.manuscripts/`). v1 stays importable as a standalone module.

## v1 cut line

**In:** PLAN, README, SKILL, Go+Cobra binary, mocked tests, doctor after
fixture auth, confirm-gated `on`, Custom-slot temp, optional SQLite history.

**Out:** schedule commands, Workers/D1, live CI heat-on, SSO-only login,
GraphQL/websocket push.

**Success:**

```
go test ./...
go build -o harvia-pp-cli ./cmd/harvia-pp-cli
harvia-pp-cli --home /tmp/harvia-pp auth login --env-file testdata/fixtures/env
harvia-pp-cli --home /tmp/harvia-pp doctor
```
