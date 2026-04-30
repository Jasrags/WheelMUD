<!-- Generated: 2026-04-30 | Files scanned: go.mod, go.sum | Token estimate: ~250 -->

# Dependencies

## Module

`github.com/Jasrags/WheelMUD` — Go 1.24, single binary `cmd/server`.

## Direct

| Dep | Version | Used in | Purpose |
|---|---|---|---|
| `github.com/i582/cfmt` | v1.4.0 | `telnet/session.go` | Renders `{{text}}::style` tags inside `WriteString`/`WriteWrapped`. **Important:** treats input as a template — do not pass untrusted strings. Use `WriteRaw` for client-derived data. |

## Indirect

| Dep | Version | Pulled by |
|---|---|---|
| `github.com/gookit/color` | v1.3.2 | cfmt |

`go.sum` also lists `davecgh/go-spew`, `pmezard/go-difflib`, `stretchr/testify`, `stretchr/objx` from the resolution graph but they are not imported by any source file in this module.

## Standard library only

The telnet protocol, ANSI handling, command registry, mode stack, completion column layout, and word-wrap are all built on stdlib (`bufio`, `net`, `sort`, `strings`, `sync`, `unicode/utf8`, `log/slog`, `time`, `errors`, `encoding/binary`).

## External services

None. The server has no outbound network calls, no DB, no cache, no queue, no auth provider. The single inbound surface is TCP `:2323` (default; override via `LISTEN_ADDR`).

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
