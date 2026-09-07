# Shipcheck: harvia-pp-cli v0.1.0

Run 2026-09-07 on linux/amd64, Go 1.24. No live Harvia traffic.

| Gate | Result |
| --- | --- |
| `gofmt -l .` | clean |
| `go vet ./...` | clean |
| `golangci-lint run ./...` | 0 issues |
| `go test ./...` | actions, auth, cli, client, store all ok (26 tests) |
| `go build ./cmd/harvia-pp-cli` | ok |
| `--help` / `--version` | ok; version injectable via `-X .../internal/cli.version` |
| Cross-compile `CGO_ENABLED=0` | darwin/arm64, darwin/amd64, linux/arm64, windows/amd64 ok |
| `on </dev/null` without `--yes` | exit 2, no prompt, no `/devices/command` POST |
| Secret scan (tokens, passwords, emails, coordinates) | fixtures only |
| printing-press-library `verify_skill.py --dir .` | all 5 checks passed |

## Behaviours proven by tests

- Login writes `token.json` mode 0600; `auth status` / `doctor` never echo
  token or password values.
- Device UUID resolved from `name` when `deviceId` is missing or not a UUID.
- `on --yes --temp 82` write order: `PATCH /devices/target` (profile 3),
  `PATCH /devices/profile` (3), `POST /devices/command` (SAUNA on).
- Mid-session `temp` activates Custom slot; heater-off `temp` is a bare PATCH.
- Refresh response without `refreshToken` keeps the old one.
- 401 on an authed call: refresh, then re-login, then surface the recovery
  error rather than the original 401.
- `on` under `--no-input` or non-TTY stdin without `--yes` exits 2 with no
  command POST.
- `doctor` green after fixture login; fails on a 0644 token file.
- SQLite sample append, session open/close with peak temp, `history hours`
  sums duration.
