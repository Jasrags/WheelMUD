<!-- Generated: 2026-05-10 | Files scanned: go.mod, go.sum | Token estimate: ~380 -->

# Dependencies

## Module

`github.com/Jasrags/WheelMUD` — Go 1.25, single binary `cmd/server`.

## Direct

| Dep | Version | Used in | Purpose |
|---|---|---|---|
| `github.com/i582/cfmt` | v1.4.0 | `telnet/session.go`, every render path | Renders `{{text}}::style` tags inside `WriteString`/`WriteWrapped`. **Important:** treats input as a template — do not pass untrusted strings. Use `WriteRaw`, or run user-derived strings through `display.Defang` first. |
| `gopkg.in/yaml.v3` | v3.0.1 | `internal/world/yaml.go`, `internal/chargen/catalog.go`, `internal/news/`, `internal/help/`, `internal/effects/`, `internal/quest/` | YAML decoder for the world loader, chargen catalog, news catalog, help catalog, effects catalog, quest catalog. All catalogs validate at boot — typos fail loud. |
| `modernc.org/sqlite` | v1.50.0 | `internal/db/db.go` | Pure-Go SQLite driver (no CGO). `database/sql` driver name `sqlite`. 48 embedded migrations under `internal/db/migrations/`. |
| `golang.org/x/crypto/bcrypt` | v0.50.0 | `internal/auth/hash.go` | Password hashing. Default cost (10); tests drop to MinCost via `auth.SetCost`. |
| `github.com/yuin/gopher-lua` | v1.1.2 | `internal/lua/`, `internal/scripts/` | Embedded Lua 5.1 runtime backing the §15 trigger/dialogue/quest scripting surface (Phase F #32). Sandboxed (dangerous globals stripped), 50ms ctx timeout per call via `SetContext`, LState pool of 8. |

## Indirect

| Dep | Pulled by |
|---|---|
| `github.com/gookit/color` | cfmt |
| `modernc.org/{libc,memory,mathutil}`, `golang.org/x/sys`, `github.com/google/uuid`, `github.com/dustin/go-humanize`, `github.com/mattn/go-isatty`, `github.com/ncruces/go-strftime`, `github.com/remyoudompheng/bigfft` | sqlite |

## Standard library

The telnet protocol, ANSI handling, command registry, mode stack, completion, word wrap, pub/sub, scheduler, FNV-32a hashing for catalog ids, panic recovery, and JSON serialisation are all stdlib (`bufio`, `net`, `sort`, `strings`, `sync`, `unicode/utf8`, `log/slog`, `time`, `errors`, `encoding/binary`, `encoding/json`, `hash/fnv`, `database/sql`, `context`).

## External services

None. The server has no outbound network calls, no external DB, no cache, no queue, no auth provider. Single inbound surface: TCP `:2323` (default; override via `LISTEN_ADDR`).

## Configuration (env vars)

| Var | Default | Purpose |
|---|---|---|
| `LISTEN_ADDR` | `:2323` | TCP listen address. Use `127.0.0.1:0` for ephemeral port (smoke tests). |
| `DB_DSN` | `wheelmud.db` | SQLite DSN. `:memory:` works for ephemeral runs. WAL mode adds `*-wal`/`*-shm` siblings — gitignored. |
| `LOG_LEVEL` | `debug` | `debug`/`info`/`warn`/`error`. Wired to `slog`. |
| `WORLD_DIR` | `./data/world` | Override the embedded world tree. Relative paths resolve from CWD. |
| `CHARGEN_DIR` | (embedded) | Override the embedded chargen catalogs. |
| `EFFECTS_DIR` | (embedded) | Override the embedded effects catalog (Phase E #25). |
| `QUEST_DIR` | (embedded) | Override the embedded quest catalog (Phase F #31). |
| `SCRIPT_DIR` | (embedded) | Override the embedded Lua script catalog (Phase F #32). |
| `NEWS_DIR` | (embedded) | Override the embedded MOTD/news catalog. |

## Build / runtime tooling

| Tool | Where | Purpose |
|---|---|---|
| `make build/server` / `make run/server` | `Makefile` | Build to `/tmp/bin/server`, run |
| `make run/live/server` | `Makefile` | Hot reload via `cosmtrek/air@v1.43.0` (runs `go mod tidy` first) |
| `docker compose up` | `Dockerfile` + `docker-compose.yml` | Containerized run, exposes :2323 |
| `go test -race ./...` | — | Full suite — every package under `internal/` plus `telnet` |
| `gofmt`, `go vet` | — | Required clean before commit |

## Pending integrations

When ROADMAP items land, expect rows for: Prometheus client for §19 metrics, `golang.org/x/text/unicode/norm` for NFKC username normalization (`persistence_followups.md`), and `golang.org/x/text/width` for wide-glyph wrap (`terminal_rendering_followups.md`). Embedded scripting (gopher-lua) landed for §15 / Phase F #32 across slices 1–4.
