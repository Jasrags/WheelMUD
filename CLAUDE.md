# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

WheelMUD is an early-stage MUD server written in Go (1.24). It listens on TCP `:2323`, performs telnet option negotiation, and runs a per-connection line-based command loop driven by a registry/mode-stack dispatcher. There is no persistence, world model, or auth yet — the codebase is focused on the telnet/ANSI transport layer and the command-input plumbing on top of it.

`ROADMAP.md` at the repo root tracks what's done vs. pending across the major MUD subsystems and is the source of truth for "what's next."

## Common commands

```bash
make build/server      # go build -o /tmp/bin/server cmd/server/main.go
make run/server        # build then run the binary
make run/live/server   # hot reload via cosmtrek/air (runs go mod tidy first)
go test -race ./...    # full test suite with race detector
docker compose up      # build + run, exposes :2323
```

Connect with: `telnet localhost 2323` (or `nc localhost 2323`).

## Architecture

- **`cmd/server/main.go`** — entrypoint. Accepts TCP connections, constructs a `telnet.Session` per connection, builds the command registry, pushes the initial `Game` mode, and hands the session to `telnet.RunSession`.

- **`telnet/`** — protocol primitives and per-connection driver.
  - `session.go`: `Session` struct (conn, terminal type, width/height, input buffer, password-mode flag, color level, write mutex, mode stack). `WriteString` renders cfmt tags; `WriteWrapped` additionally reflows the result via `WrapText` to `Session.Width`. All writes serialize on `writeMu`.
  - `server.go`: `RunSession` plus the byte parser (`readLoop`, `dispatchByte`, `bufferInput`, `handleLineBreak`, `handleBackspace`, `handleTab`) and the per-session dispatcher goroutine (`runDispatcher`).
  - `iac.go`: telnet IAC/option constants, `NegotiateTelnet`, `RequestTerminalType`, `ReadIAC` (handles escaped `IAC IAC` as a literal data byte, standalone `GA`/`NOP`/`AYT`/etc. as no-ops, full WILL/WONT/DO/DONT negotiation, and bounded subnegotiation), `HandleSubnegotiation` (`TERM_TYPE`, `NAWS`), and `DescribeByte`/`DescribeIAC`.
  - `color.go`: ANSI SGR constants, color-level enum (`None`/`Basic`/`16`/`256`/`TrueColor`), `DetectColorLevel(term)`, `SGR(...)` helper, and `RenderRGBFG`/`RenderRGBBG` that downsample 24-bit color to the session's advertised level.
  - `wrap.go`: `WrapText` — ANSI-aware word wrap that treats CSI/OSC escapes as zero-width, drops bare CRs, and overflows tokens longer than `width` rather than splitting them.
  - `command.go`, `mode.go`, `completion.go`: command registry with alias + prefix lookup, `Mode` interface and `Session.PushMode`/`PopMode`/`ReplaceMode`, and verb-only tab completion. `Game` mode in `internal/mode/game.go` wraps the registry.
  - `ascii.go`: ASCII control-character constants used by the input parser (BS/DEL/etc.).

- **`internal/cmd/`** — concrete commands registered in `main.go::buildRegistry` (`quit`, `who`, `help`, `togglepassword`).

### Things to watch when editing

- The protocol parser lives in `telnet/server.go` (the older note about it living in `main.go` is obsolete).
- `Session.InputBuffer` is owned by the read goroutine inside `RunSession`. Do not mutate it from another goroutine, including the dispatcher.
- `Session.WriteRaw` is the only safe write path; it holds `writeMu`. New helpers should layer on top of it rather than calling `Conn.Write` directly.
- `Mode.Handle` is invoked synchronously by `runDispatcher`; a slow handler stalls input for that session. If a Handle implementation needs blocking I/O, plumb a `context.Context` through (tracked in the deferred-work memory).
- `Command.Auth` (`AuthLevel`) is stored but not enforced yet — the check waits for the login/account subsystem.
- Logging uses `slog` at `LevelDebug` set globally in `main.go`; verbose by design during protocol bring-up.

## Tests

`go test -race ./...` covers the registry, mode dispatcher, completion handler, IAC parser, color helpers, and word wrap. New telnet-package tests reuse `newPipeSession(t)` from `telnet/command_test.go` to get a `Session` backed by `net.Pipe`.

## Module path

`github.com/Jasrags/WheelMUD` — only third-party dep is `github.com/i582/cfmt` (which pulls in `gookit/color`).
