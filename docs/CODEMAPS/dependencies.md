<!-- Generated: 2026-04-30 | Files scanned: go.mod, go.sum | Token estimate: ~250 -->

# Dependencies

## Module

`github.com/Jasrags/WheelMUD` — Go 1.24, single binary `cmd/server`.

## Direct

| Dep | Version | Used in | Purpose |
|---|---|---|---|
| `github.com/i582/cfmt` | v1.4.0 | `telnet/session.go` | Renders `{{text}}::style` tags inside `WriteString`/`WriteWrapped`. **Important:** treats input as a template — do not pass untrusted strings. Use `WriteRaw` for client-derived data. |
| `modernc.org/sqlite` | v1.50.0 | `internal/db/db.go` | Pure-Go SQLite driver (no CGO). `database/sql` driver name `sqlite`. Embedded migrations under `internal/db/migrations/`. |
| `golang.org/x/crypto/bcrypt` | v0.50.0 | `internal/auth/hash.go` | Password hashing. Default cost (10); tests drop to MinCost via `auth.SetCost`. |

## Indirect

| Dep | Version | Pulled by |
|---|---|---|
| `github.com/gookit/color` | v1.3.2 | cfmt |
| `modernc.org/libc` | — | sqlite |
| `modernc.org/memory` | — | sqlite |
| `modernc.org/mathutil` | — | sqlite |
| `golang.org/x/sys` | — | sqlite |
| `github.com/google/uuid`, `github.com/dustin/go-humanize`, `github.com/mattn/go-isatty`, `github.com/ncruces/go-strftime`, `github.com/remyoudompheng/bigfft` | — | sqlite |

`go.sum` also lists `davecgh/go-spew`, `pmezard/go-difflib`, `stretchr/testify`, `stretchr/objx` from the resolution graph but they are not imported by any source file in this module.

## Standard library only

The telnet protocol, ANSI handling, command registry, mode stack, completion column layout, and word-wrap are all built on stdlib (`bufio`, `net`, `sort`, `strings`, `sync`, `unicode/utf8`, `log/slog`, `time`, `errors`, `encoding/binary`).

## External services

None. The server has no outbound network calls, no DB, no cache, no queue, no auth provider. The single inbound surface is TCP `:2323` (default; override via `LISTEN_ADDR`).

## Configuration (env vars)

| Var | Default | Purpose |
|---|---|---|
| `LISTEN_ADDR` | `:2323` | TCP listen address. Use `127.0.0.1:0` for ephemeral port (smoke tests). |
| `DB_DSN` | `wheelmud.db` | SQLite DSN. `:memory:` for ephemeral; file paths are relative to CWD. WAL mode adds `*-wal` / `*-shm` siblings — gitignored. |

## Build / runtime tooling

| Tool | Where | Purpose |
|---|---|---|
| `make build/server` / `make run/server` | `Makefile` | Build to `/tmp/bin/server`, run |
| `make run/live/server` | `Makefile` | Hot reload via `cosmtrek/air@v1.43.0` |
| `docker compose up` | `Dockerfile` + `docker-compose.yml` | Containerized run, exposes :2323 |
| `go test -race ./...` | — | Full suite (only `telnet/` has tests) |
| `gofmt`, `go vet` | — | Required clean before commit |

## Pending integrations

When `ROADMAP.md` items land, expect this file to grow rows for: SQLite/Postgres driver, password-hashing (`bcrypt`/`argon2id`), embedded scripting (`gopher-lua` or starlark/risor), Prometheus client, and possibly `golang.org/x/text/width` for wide-glyph wrap.
