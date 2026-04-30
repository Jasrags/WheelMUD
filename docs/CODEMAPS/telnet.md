<!-- Generated: 2026-04-30 | Files scanned: telnet/*.go (10 files, ~1.6k LOC) | Token estimate: ~900 -->

# Telnet Package

Stand-in for `backend.md`: this is the only "service" the binary runs. The package owns the wire protocol, per-session state, the mode stack, the command registry, and the rendering helpers.

## Public surface

```
telnet.Session            per-connection state + write lock
  .Conn, .RemoteAddress, .TerminalType, .Width, .Height
  .InputBuffer (read-goroutine-owned), .InPasswordMode, .ColorLevel
  .WriteRaw([]byte)         serialized via writeMu
  .WriteString(text)        cfmt.Sprint → WriteRaw
  .WriteWrapped(text)       cfmt.Sprint → WrapText → CRLF → WriteRaw
  .PushMode/.PopMode/.ReplaceMode/.CurrentMode
telnet.NewSession(conn)   constructor (returns nil on nil conn)

telnet.RunSession(s)      negotiate → readLoop + runDispatcher → return on EOF/idle
telnet.NegotiateTelnet    sends WILL SGA / DO SGA / DO TT / DO NAWS / WILL ECHO
telnet.RequestTerminalType  IAC SB TT SEND IAC SE
telnet.ReadIAC            IAC IAC → literal 0xFF; IAC SB → readSubnegotiation;
                          IAC WILL/WONT/DO/DONT → consume opt; standalone GA/NOP/AYT/etc → no-op
telnet.HandleSubnegotiation  TERM_TYPE → DetectColorLevel; NAWS → Width/Height

telnet.Registry / Command / Context     command system
telnet.Mode / Completer / Candidate     mode stack + tab completion

telnet.DetectColorLevel(term) int
telnet.SGR(codes...) string
telnet.RenderRGBFG(level,r,g,b) / RenderRGBBG  truecolor → 256 → 16 fallback
telnet.WrapText(text, width)              ANSI-aware word wrap
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
Dispatch(s,line)
  TrimSpace → splitVerb(line) → Lookup(verb)
  err? → writeLookupError(s, err)   (Unknown / err.Error / Command error)
  args := strings.Fields(rest)      (quoted-arg tokenizer deferred)
  len(args) < MinArgs → "Usage: ..."
  build Context{Session, Name, Args, Raw} → Command.Run
```

`Command.Auth` (`AuthGuest/AuthPlayer/AuthAdmin`) is stored but not enforced.

## Mode stack (`mode.go`, `session.go`)

```go
type Mode interface {
  Handle(*Session, string) error
  Prompt(*Session) string
  OnEnter(*Session) error
  OnExit(*Session)  error
}
type Completer interface { Complete(*Session, buffer string) []Candidate }
```

`Session.modes` is a slice protected by `modeMu`. `PushMode` appends then calls `OnEnter`; `PopMode` trims then calls `OnExit`; `ReplaceMode` = pop + push. **Known issue:** `OnEnter` failure leaves the mode on the stack (tracked in `code_review_open_items.md`).

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
