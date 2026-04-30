<!-- Generated: 2026-04-30 | Files scanned: internal/cmd/*.go | Token estimate: ~600 -->

# Command Catalog

Replaces the standard `frontend.md`: there is no UI tree, but the commands are the user-facing surface (post-login). Pre-login the surface is whichever `Mode` is on top of the stack — see `data.md` for the auth pipeline diagram.

## Wiring

`cmd/server/main.go::buildRegistry` takes the world + character repos so closure-style commands (`NewHelp`, `NewLook`, `NewMoveFamily`) can hold references. Variable-style commands (`Quit`, `Who`, `Colors`) are package-level singletons.

```
buildRegistry(rooms, exits, items, mobs, characters)
  r := NewRegistry()
  r.Register(Quit, Who, Colors)
  r.Register(NewHelp(r))
  r.Register(NewLook(rooms, exits, items, mobs))
  r.Register(NewMoveFamily(rooms, exits, items, mobs, characters)...)
```

The registry is shared across sessions (read-only at runtime). Mode wiring: `mode.NewGame(r)` is the in-world target; `mode.NewLogin(accounts, characters, sessions, gameMode)` is what `srv.newInitial()` returns for each new connection.

## Catalog

| Verb | Aliases | File | Purpose |
|---|---|---|---|
| `quit` | — | `internal/cmd/quit.go` | Closes the session. `Run` writes goodbye, closes `Conn`, returns `ErrSessionEnded` so the dispatcher stops without a prompt. |
| `who` | — | `internal/cmd/who.go` | Reports the caller's character name (falls back to `RemoteAddress` if no character is selected). Server-wide listing pending — needs `session.Registry.Snapshot` iteration. |
| `colors` | `colortest`, `palette` | `internal/cmd/colors.go` | Prints terminal info, 16-color palette, xterm-256 cube + grayscale, RGB ramp via `RenderRGBBG` (so the downsampling path is exercised), and style samples via `SGR`. |
| `help` | `?` | `internal/cmd/help.go` | `help` lists registered commands; `help <verb>` prints `Help`/`Long`. Built via `NewHelp(r)` so it sees the live registry. |
| `look` | `l` | `internal/cmd/look.go` | Renders the room at `Session.CurrentRoomID`: name, long description, exits (with long-name translation), items, mobs. Empty subsections collapse; missing-room writes a soft "tell an admin" line. The internal `RenderRoom` helper is reused by every move so movement implicitly shows the new room. |
| `north` / `south` / `east` / `west` / `up` / `down` | `n` / `s` / `e` / `w` / `u` / `d` | `internal/cmd/move.go` | Six commands built by `NewMoveFamily`. Each calls `moveDir`, which: (1) resolves the exit via `ExitRepo.FindByDirection`; (2) on miss, writes `"You can't go that way."`; (3) on hit, updates `Session.CurrentRoomID`, persists via `CharacterRepo.RecordRoom` (best-effort), and re-renders the new room via `RenderRoom`. |

## Adding a command

```go
// internal/cmd/foo.go
var Foo = &telnet.Command{
    Name:    "foo",
    Aliases: []string{"f"},
    Help:    "one-liner shown in help index",
    Long:    "optional multi-line body",
    MinArgs: 0,
    Auth:    telnet.AuthPlayer, // checked by Registry.Dispatch
    Run: func(c *telnet.Context) error {
        // c.Ctx, c.Session, c.Name, c.Args, c.Raw
        // c.Session.AccountID / CharacterID / CharacterName are set
        // post-login.
        return c.Session.WriteRaw([]byte("ok\r\n"))
    },
}
```

Register it in `cmd/server/main.go::buildRegistry`. Verb names must be lowercase ASCII with no whitespace (`validVerb` rejects others at registration time).

## Conventions

- Output uses `\r\n` line breaks (telnet wire convention).
- Prefer `WriteRaw` for client-derived strings; `WriteString`/`WriteWrapped` run input through `cfmt.Sprint` and treat `{{...}}::style` as markup.
- Long/columnar output that should reflow → `WriteWrapped` (uses `Session.Width`).
- A `Run` that ends the session must close `Conn` and return `ErrSessionEnded`.
- Errors returned from `Run` are logged at debug level; the user does not see them unless `Run` writes the message itself.
- Set `Auth: AuthAdmin` for staff-only verbs. `Registry.Dispatch` masks denied verbs as `"Unknown command"` so a guest probe can't enumerate them.
- Long blocking work in `Run` should respect `c.Ctx` — it's canceled when the session ends.

## Deferred

- Quoted-argument tokenization (`say "hello world"` today splits on whitespace).
- Per-command argument completion (`Completer` field on `Command`).
- User-defined aliases distinct from registry aliases.

See `command_input_followups.md` (auto-memory) for triggers and sketches.
