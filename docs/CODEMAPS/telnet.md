<!-- Generated: 2026-05-02 | Files scanned: telnet/*.go (12 files) | Token estimate: ~950 -->

# Telnet Package

Stand-in for `backend.md`: this is the only "service" the binary runs. The package owns the wire protocol, per-session state, the mode stack, the command registry, and the rendering helpers.

## Public surface

```
telnet.Session            per-connection state + write lock
  .Conn, .RemoteAddress, .TerminalType, .Width, .Height
  .InputBuffer, .InPasswordMode, .ColorLevel
  .AuthLevel, .AccountID, .CharacterID, .CharacterName
  .CurrentRoomID, .LastTellFrom, .LastInputAt, .channelMuted (crossMu-guarded)
  .WriteRaw([]byte), .WriteString, .WriteWrapped
  .PushMode/.PopMode/.ReplaceMode/.CurrentMode
  .SetChannelMuted / .IsChannelMuted / .ToggleChannelMuted / .ChannelMutedSnapshot
telnet.NewSession(conn)   constructor

telnet.RunSession(s)      negotiate → readLoop + runDispatcher → return
telnet.NegotiateTelnet    WILL SGA / DO SGA / DO TT / DO NAWS / WILL ECHO
telnet.RequestTerminalType  TERM_TYPE subnegotiation
telnet.ReadIAC            parse IAC bytes: IAC IAC literal, IAC SB/WILL/DO/DONT/etc.
telnet.HandleSubnegotiation  TERM_TYPE → color detection; NAWS → width/height

telnet.AuthLevel enum     AuthGuest / AuthPlayer / AuthAdmin (checked at dispatch)
telnet.Registry / Command / Context     command system
telnet.Mode / Completer / Candidate     mode stack + completion

telnet.DetectColorLevel(term) int
telnet.SGR(codes...) string  (ANSI SGR codes)
telnet.RenderRGBFG / RenderRGBBG  24-bit → 256 → 16 downsampling
telnet.WrapText(text, width)  ANSI-aware word wrap
telnet.History, LineEdit (stub)  command history + in-line editing
```

## Per-connection pipeline (`server.go`)

```
RunSession ─► NegotiateTelnet ─► RequestTerminalType
            ─► go runDispatcher  (consumes Session.inbox)
            └► readLoop          (until EOF / idle 10m / flood)
                  ReadByte ─► dispatchByte:
                     IAC      → ReadIAC; data byte? → bufferInput
                     ESC      → DiscardANSI (bounded 64 bytes)
                     CR/LF    → handleLineBreak (coalesces CRLF)
                     BS/DEL   → handleBackspace
                     HT       → handleTab
                     printable→ bufferInput (echoes '*' in password mode)
                     other    → log + drop
```

Backpressure: `inbox` cap = 16 (`inboxCap`). Overflow → `ErrInputFlooded` from `readLoop`. `runDispatcher` drains on `ErrSessionEnded` / `net.ErrClosed` / `io.EOF` and stops without drawing a prompt.

## Command registry (`command.go`)

```
Registry
  ├ sorted []*Command   (lex order, binary-search prefix)
  └ aliases map[string]*Command

Register(c)   validates verb (lowercase ASCII, no ws), inserts sorted, indexes aliases
Lookup(v)    alias → exact → unique prefix (else ErrUnknownCommand / ErrAmbiguousPrefix)
Prefix(p)    [start..) slice of HasPrefix(p) hits — drives completion
Dispatch(ctx, s, line)
  TrimSpace → splitVerb(line) → Lookup(verb)
  err? → writeLookupError(s, err)   (Unknown / err.Error / Command error)
  s.AuthLevel < cmd.Auth → "Unknown command" (does NOT leak existence)
  args := strings.Fields(rest)      (quoted-arg tokenizer deferred)
  len(args) < MinArgs → "Usage: ..."
  build Context{Ctx, Session, Name, Args, Raw} → Command.Run
```

`Command.Auth` (`AuthGuest/AuthPlayer/AuthAdmin`) is checked against `Session.AuthLevel`. Login mode is what bumps `AuthLevel` from the default `AuthGuest` — until login lands, all commands run at guest level.

## Mode stack (`mode.go`, `session.go`)

```go
type Mode interface {
  Handle(ctx context.Context, s *Session, line string) error
  Prompt(*Session) string
  OnEnter(*Session) error
  OnExit(*Session) error
}
type Completer interface {
  Complete(s *Session, buffer string) []Candidate
}
```

`Session.modes` is a slice protected by `modeMu`. `PushMode` appends then calls `OnEnter`; on error, mode is removed immediately (fixed). `PopMode` trims + calls `OnExit`; `ReplaceMode` = pop + push. `Mode.Handle` receives `ctx` canceled when read loop exits, so blocking handlers observe cancellation. `AuthLevel` is checked at dispatch time (per-command gating), not mode entry.

## Tab completion (`completion.go`, `server.go`)

```
handleTab
  password mode? → BEL
  current mode is Completer? else BEL
  partial = completionPartial(buffer)        (last whitespace-delimited token)
  cands = mode.Complete(s, buffer)
  applyCompletion:
    0 → BEL
    1 → extendBuffer(partial, text+" ")
    n with longer common prefix → extendBuffer to it
    n otherwise → listAndRedraw (columns via writeColumns; helpColumnThreshold guards single-col)
```

Verb-only today; argument-side completion is deferred.

## Rendering (`color.go`, `wrap.go`, `session.go`)

- Color levels: `None / Basic(8) / 16 / 256 / TrueColor`. `DetectColorLevel` reads TERM (telnet has no COLORTERM hint).
- `RenderRGBFG/BG` always emit a valid SGR for `level >= 16`; `rgbTo256` uses the xterm 6×6×6 cube; `rgbTo16` picks bright/dim + hue from channel dominance.
- `WrapText` treats CSI/OSC escapes as zero-width, drops bare CR, overflows tokens longer than `width` rather than splitting (deferred — see memory).
- `Session.WriteWrapped` short-circuits to `WriteString` when `Width <= 0` to avoid CRLF doubling.

## Files

| File | LOC | Purpose |
|---|---|---|
| `server.go` | 275 | RunSession, parser, dispatcher |
| `command.go` | 259 | Registry, Lookup, Dispatch |
| `iac.go` | 253 | IAC/SB protocol |
| `color.go` | 198 | SGR + RGB → level downsampling |
| `session.go` | 147 | Session, mode stack, write lock |
| `wrap.go` | 135 | Word-wrap |
| `completion.go` | 125 | Column layout + common-prefix |
| `ascii.go` | 116 | Control-byte names |
| `mode.go` | 21 | Mode + Completer interfaces |

Tests cover registry, dispatcher, completion handler, IAC parser, color helpers, and word wrap. `go test -race ./...` is the suite.
