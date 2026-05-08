<!-- Generated: 2026-05-07 | Files scanned: telnet/*.go (16 source files) | Token estimate: ~1100 -->

# Telnet Package

Stand-in for `backend.md`: this is the only "service" the binary runs.
The package owns the wire protocol, per-session state, mode stack,
command registry, line editor + history, alias table, quoted-argument
tokenizer, prompt cache + async cross-session output, and rendering
helpers. Imports nothing from `internal/*` — every higher layer depends
on `telnet`.

## Public surface

```
telnet.Session                per-connection state + write/cross/edit locks
  .Conn, .RemoteAddress, .TerminalType, .Width, .Height
  .ColorLevel, .AuthLevel, .AccountID, .CharacterID, .CharacterName
  .CurrentRoomID
  .WriteRaw([]byte)           sole syscall path; holds writeMu
  .WriteString(s)              cfmt-rendered output
  .WriteWrapped(s)             ANSI-aware reflow to .Width
  .WriteAsync(s)               cross-session safe — wraps with CR+EL
                              prefix and replays cached prompt + Input
  .WritePrompt(s)              caches prompt for WriteAsync replay
  .ClearLastPrompt()           on PushMode/PopMode/ReplaceMode
  .EditAndWrite(fn)            read-goroutine wrapper: hold writeMu,
                              run Input mutation + return echo bytes
  .SetPasswordMode(bool)       under writeMu
  .Set/Get/Toggle/Snapshot helpers for crossMu-guarded fields:
      lastTellFrom, lastInputAt, channelMuted, followingID, wizinvis
  .PushMode/.PopMode/.ReplaceMode/.CurrentMode

telnet.NewSession(conn)        constructor
telnet.RunSession(s)           negotiate → readLoop + dispatcher → return
telnet.NegotiateTelnet         WILL SGA / DO SGA / DO TT / DO NAWS / WILL ECHO

telnet.AuthLevel               AuthGuest / AuthPlayer / AuthAdmin
telnet.Registry / Command / Context     command system
telnet.Mode / Completer / Candidate     mode stack + completion
telnet.Tokenize(line) []string ([]Quoted, error)  quoted-arg tokenizer
telnet.SplitOnSemicolon(line)  segment splitter (mirrors Tokenize quote rules)
telnet.WrapText / SGR / RenderRGBFG / RenderRGBBG / DetectColorLevel
telnet.History, LineEdit       command history + line editor
telnet.Alias                   per-session alias table
```

## Per-connection pipeline (`server.go`)

```
RunSession ─► NegotiateTelnet ─► RequestTerminalType
            ─► go runDispatcher (consumes Session.inbox)
            └► readLoop (until EOF / idle 10m / flood)
                  ReadByte ─► dispatchByte:
                     IAC      → ReadIAC; data byte? → bufferInput
                     ESC      → DiscardANSI / arrow keys → history / edit
                     CR/LF    → handleLineBreak (coalesces CRLF)
                     BS/DEL   → handleBackspace
                     HT       → handleTab
                     printable→ bufferInput (echoes '*' in password mode)
                     other    → log + drop
```

Backpressure: `inbox` cap = 16 (`inboxCap`). Overflow → `ErrInputFlooded`
from `readLoop`. `runDispatcher` drains on `ErrSessionEnded` /
`net.ErrClosed` / `io.EOF` and stops without drawing a prompt.

## Concurrency model

- `Session.Input` (line edit buffer) is **owned by the read goroutine**.
  Never mutated from elsewhere.
- All writes serialise on `writeMu` via `WriteRaw`.
- Read-goroutine echoes wrap "decide echo + mutate Input + emit echo" in
  `Session.EditAndWrite(fn)`. `fn` runs under `writeMu` and returns
  bytes to write; the wrapper writes them in the same critical section.
  This serialises against `WriteAsync`, `WritePrompt`, and
  `listAndRedraw` so a concurrent broadcast cannot observe a half-
  mutated Input or replay a stale prompt cache.
- Cross-session fields (`lastTellFrom`, `lastInputAt`, `channelMuted`,
  `followingID`, `wizinvis`) are guarded by `crossMu`; **always** use
  the typed helpers — never touch the unexported fields directly.
- In-world fields (`CharacterID`, `CharacterName`, `CurrentRoomID`,
  `AuthLevel`) are dispatcher-owned; treat snapshots from
  `session.Registry.Snapshot()` as values that can change underfoot.

## Cross-session output rule

Any write that targets a session other than the dispatcher's
`c.Session` MUST go through `WriteAsync`, NOT `WriteString`. `WriteAsync`
wraps the message with a CR + EL erase prefix and replays the cached
prompt + line-edit buffer afterward, so a mid-line broadcast doesn't
clobber the player's prompt or in-progress input. The dispatcher caches
each prompt via `WritePrompt`; mode transitions clear the cache via
`ClearLastPrompt`. Synchronous reply to `c.Session` keeps using
`WriteString` — the dispatcher repaints the prompt immediately after
`Mode.Handle` returns.

## Command registry (`command.go`)

```
Registry
  ├ sorted []*Command   (lex order, binary-search prefix)
  └ aliases map[string]*Command

Register(c)   validates verb (lowercase ASCII, no ws), inserts, indexes aliases
Lookup(v)    alias → exact → unique prefix
              (else ErrUnknownCommand / ErrAmbiguousPrefix)
Prefix(p)    [start..) slice of HasPrefix(p) — drives completion
Dispatch(ctx, s, line)
  segments := SplitOnSemicolon(line)   // top-level `;` chains; cap 16
  for each segment:
    expand alias (depth ≤ 3) → Lookup → AuthLevel check (mask as
    "Unknown command" on denial) → Tokenize → MinArgs check →
    build Context{Ctx, Session, Name, Args, Raw} → Command.Run
  first Run error returned; later segments still run
```

`Command.Auth` is checked at dispatch time, not mode entry. Denials
render as `"Unknown command"` so a guest probe can't enumerate
privileged verbs.

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

`Session.modes` is a slice protected by `modeMu`. `PushMode` appends
then calls `OnEnter`; on error, mode is removed immediately. `PopMode`
trims + calls `OnExit`; `ReplaceMode` = pop + push. All three transitions
clear the prompt cache. `Mode.Handle` receives a `ctx` canceled when the
read loop exits, so blocking handlers observe cancellation.

## Tokenizer + segments (`tokenize.go`)

`Tokenize(line)` walks the buffer respecting `"..."` quoting and `\\`
escapes; unbalanced quotes return an error surfaced as
`"Unbalanced quote"`. `SplitOnSemicolon(line)` mirrors the same quote
rules so `say "hello; world"` stays one command and `look; n; look`
chains cleanly.

## Tab completion (`completion.go`, `server.go`)

```
handleTab
  password mode? → BEL
  current mode is Completer? else BEL
  partial = last whitespace-delimited token
  cands = mode.Complete(s, buffer)
  applyCompletion:
    0     → BEL
    1     → extendBuffer(partial, text+" ")
    n with longer common prefix → extendBuffer to it
    n otherwise → listAndRedraw (columns via writeColumns)
```

Verb-only today; argument-side completion is deferred.

## Rendering (`color.go`, `wrap.go`, `session.go`)

- Color levels: `None / Basic(8) / 16 / 256 / TrueColor`. `DetectColorLevel`
  reads TERM (telnet has no COLORTERM hint).
- `WriteString` strips ANSI when `ColorLevel == None` (guards against
  cfmt leaking SGR to a no-color client).
- `RenderRGBFG/BG` always emit a valid SGR for `level >= 16`; `rgbTo256`
  uses the xterm 6×6×6 cube; `rgbTo16` picks bright/dim + hue from
  channel dominance.
- `WrapText` treats CSI/OSC escapes as zero-width, drops bare CR,
  overflows tokens longer than `width` rather than splitting (followup).
- `Session.WriteWrapped` short-circuits to `WriteString` when
  `Width <= 0` to avoid CRLF doubling.

## Files

| File | LOC | Purpose |
|---|---|---|
| `session.go` | 702 | Session struct, mutexes, mode stack, async write, prompt cache |
| `server.go` | 474 | RunSession, parser, dispatcher, line-edit handlers |
| `command.go` | 415 | Registry, Lookup, Dispatch (segment-aware), Context |
| `color.go` | 283 | SGR + RGB → level downsampling, ANSI strip |
| `iac.go` | 231 | IAC/SB protocol (TERM_TYPE, NAWS) |
| `tokenize.go` | 207 | Quoted-arg tokenizer + `SplitOnSemicolon` |
| `lineedit.go` | 198 | In-line editor + history integration |
| (plus) | | `wrap.go`, `completion.go`, `mode.go`, `alias.go`, `history.go`, `ascii.go` |

Tests cover registry, dispatcher, completion handler, IAC parser, color
helpers, word wrap, tokenizer, line editor, alias table, async-write
prompt-replay semantics, pager mode. `go test -race ./...` is the
suite.
