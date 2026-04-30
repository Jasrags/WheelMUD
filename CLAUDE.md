# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

WheelMUD is an early-stage MUD server written in Go (1.24). It listens on TCP `:2323`, performs telnet option negotiation, and provides a simple line-based command loop. There is no persistence, world model, or auth yet — the codebase is currently focused on the telnet/ANSI transport layer.

## Common commands

```bash
make build/server      # go build -o /tmp/bin/server cmd/server/main.go
make run/server        # build then run the binary
make run/live/server   # hot reload via cosmtrek/air (runs go mod tidy first)
go test ./...          # no tests exist yet, but this is the standard entrypoint
docker compose up      # build + run, exposes :2323
```

Connect with: `telnet localhost 2323` (or `nc localhost 2323`).

## Architecture

Two binaries' worth of code live here, but only one is wired up:

- **`cmd/server/main.go`** — entrypoint. Accepts TCP connections, constructs a `telnet.Session` per connection, runs `NegotiateTelnet` + `RequestTerminalType`, then enters a byte-at-a-time read loop in `handleConnection`. The read loop is the protocol parser: it dispatches `IAC` sequences (including `SB ... SE` subnegotiation) to `telnet.HandleSubnegotiation`, swallows inbound ANSI escapes, accumulates printable bytes into `Session.InputBuffer`, and on CR/LF calls `processCommand`. **The protocol parser currently lives in main.go, not in the telnet package** — keep this in mind when extending; moving it into `telnet/` is a natural refactor but hasn't been done.

- **`telnet/`** — protocol primitives.
  - `session.go`: `Session` struct (conn, terminal type, width/height, input buffer, password-mode flag, color level). `WriteString` runs output through `cfmt.Sprint` so `{{text}}::style` tags get rendered.
  - `iac.go`: telnet IAC/option constants, `NegotiateTelnet`, `RequestTerminalType`, `HandleSubnegotiation` (handles `TERM_TYPE` and `NAWS`), and `DescribeByte`/`DescribeIAC` for logging.
  - `color.go`: ANSI SGR constants and color-level enum (`ColorLevelNone`/`Basic`/`16`/`256`). `DetectColorLevel(term)` maps terminal type strings to a level.
  - `ascii.go`: ASCII control-character constants used by the input parser (BS/DEL/etc.).
  - `server.go`, `telnet.go`, `ansi.go`: stubs / minimal constants — placeholders for future refactor.

- **`color/`** — a *separate*, unused color renderer with its own `{tag}` regex-based templating and truecolor support. Currently the project uses `i582/cfmt` (via `Session.WriteString`) for color output instead. If you touch color rendering, decide intentionally which of the two systems to extend; do not duplicate work across both.

### Things to watch when editing

- `main.go` contains a large block of commented-out duplicates of functions that now live in `telnet/iac.go` (`negotiateTelnet`, `handleSubnegotiation`, `detectColorLevel`, `describeIAC`, `describeByte`). Treat them as historical noise — delete rather than revive.
- The input loop assumes the byte after `IAC` is either `SB` or a 2-byte negotiation (`WILL/WONT/DO/DONT + opt`). It does not handle `IAC IAC` (literal 0xFF) or standalone commands like `IAC GA`.
- `Session.WriteString` writes directly to the connection — there is no output buffering or write lock. Concurrent writes from multiple goroutines on the same session would race.
- Logging uses `slog` with `LevelDebug` set globally in `main.go`; output is verbose by design during protocol bring-up.

## Module path

`github.com/Jasrags/WheelMUD` — only third-party dep is `github.com/i582/cfmt` (which pulls in `gookit/color`).
