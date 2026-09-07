# Research brief: Harvia MyHarvia 2 / Fenix cloud API

Status: verified on a live Fenix WiFi heater (2026-07) and cross-checked
against two public Home Assistant integrations. Harvia publishes no official
API; everything below is observed behaviour and can change without notice.

## Secret identity

MyHarvia is a remote control app. The CLI's secret identity is a **sauna
ledger**: every `status` and `watch` sample lands in local SQLite, heater
on/off edges become sessions with peak temperature, and `history hours`
answers "how many sauna hours this month" which the app cannot.

## Discovery and auth

| Step | Call | Notes |
| --- | --- | --- |
| 1 | `GET https://api.harvia.io/endpoints` | Returns `endpoints.RestApi.{generics,device,data}.https`. Hosts rotate; fetch once per process, never persist. |
| 2 | `POST {generics}/auth/token` `{username,password}` | Cognito (eu-central-1) behind a REST wrapper. Returns `idToken` (Bearer, ~1h), `accessToken`, `refreshToken`, `expiresIn`. |
| 3 | `POST {generics}/auth/refresh` `{refreshToken,email}` | Returns new `idToken`/`accessToken` and **no** `refreshToken`. Keep the old one. Refresh validity is measured from original issuance. |
| 4 | On HTTP 401 | Retry once after refresh, then after a full re-login if a password is available. |

Apple-only / Google-only MyHarvia accounts have no password to exchange and
cannot use this path. The user must set an account password in the app.

## Devices and control

| Op | Method | Path |
| --- | --- | --- |
| Devices | GET | `{device}/devices?maxResults=100` (+`nextToken`) |
| State | GET | `{device}/devices/state?deviceId=&subId=C1` |
| Telemetry | GET | `{data}/data/latest-data?deviceId=&cabinId=C1` |
| Command | POST | `{device}/devices/command` `{deviceId, cabin:{id:"C1"}, command:{type,state}}` |
| Target | PATCH | `{device}/devices/target` `{deviceId, cabin:{id:"C1"}, temperature?, humidity?, profile?}` |
| Profile | PATCH | `{device}/devices/profile` `{deviceId, profile}` |

Command types: `SAUNA`, `LIGHTS`, `FAN`, `STEAMER` (`"on"`/`"off"`),
`ADJUST_DURATION` (minutes as a number).

## Traps

1. **Device UUID lives in `name`.** Fenix rows on `GET /devices` put the UUID
   in `name`, not `deviceId`. Prefer a UUID-shaped `name`, then `deviceId`,
   `id`, `name`.
2. **Mid-session target is ignored.** A running session follows its active
   profile. To change temperature while on: PATCH target with
   `profile:"3"` (Custom slot, preserving its `targetHum`), then PATCH
   profile `"3"`, then optional `ADJUST_DURATION`, and only then `SAUNA on`
   for a warmup. When the heater is off, a bare target PATCH is enough.
3. **Endpoint hosts rotate.** Do not cache the map to disk.
4. **Refresh does not rotate the refresh token.** Persist the previous one.
5. **Wire temperatures are Celsius.** 32-90C. 82C = 180F, 88C = 190F.

## Safety model

`SAUNA on` powers a 240V heater. The CLI requires `--yes`, `--agent`, or an
interactive `y` on a real TTY. `--no-input` or a non-TTY stdin without
`--yes` refuses with exit 2 and sends no command. `doctor --live` is
read-only. Tests use a mocked server and never contact `api.harvia.io`.

## Sources

- [WiesiDeluxe/ha-harvia-sauna](https://github.com/WiesiDeluxe/ha-harvia-sauna)
- [TommyJuuti/harvia-home-assistant-plugin](https://github.com/TommyJuuti/harvia-home-assistant-plugin)
- Author's earlier private Python client and Worker notes (live measurements).
