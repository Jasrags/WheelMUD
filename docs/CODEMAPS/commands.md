<!-- Generated: 2026-05-02 | Files scanned: internal/cmd/*.go (18 files) | Token estimate: ~750 -->

# Command Catalog

Replaces the standard `frontend.md`: there is no UI tree, but the commands are the user-facing surface (post-login). Pre-login the surface is whichever `Mode` is on top of the stack — see `data.md` for the auth pipeline diagram.

## Wiring

`cmd/server/main.go::buildRegistry` constructs the registry once at boot. Closure-style commands (`NewHelp`, `NewLook`, `NewMoveFamily`, `NewTeleport`, `NewSay`, `NewTell`, `NewReply`, `NewWho`, `NewAlias`, `NewUnalias`, `NewChannel`, `NewChannelsList`) hold repo/session/bus references; singletons (`Quit`, `Colors`) are package-level.

```
buildRegistry(rooms, exits, items, mobs, characters, sessions, bus, channels)
  r := NewRegistry()
  r.Register(Quit, Colors)  // singletons
  r.Register(NewWho(sessions), NewSay(sessions), NewTell(sessions), 
             NewReply(sessions))  // closures
  for ch := range channels:
    r.Register(NewChannel(ch, sessions, characters))  // dynamic per channel
  r.Register(NewChannelsList(channels))  // catalog overview
  r.Register(NewAlias(), NewUnalias())
  r.Register(NewHelp(r))
  r.Register(NewLook(rooms, exits, items, mobs))
  r.Register(NewMoveFamily(rooms, exits, items, mobs, characters, bus)...)
  r.Register(NewTeleport(rooms, exits, items, mobs, characters, sessions))
  return r
```

Registry is shared across sessions, read-only at runtime. Mode wiring: `mode.NewGame(r)` is the in-world target; `mode.NewLogin(accounts, characters, sessions, gameMode)` is per-connection. Channel commands are registered dynamically from the DB catalog at boot.

## Catalog

| Verb | Aliases | AuthLevel | File | Purpose |
|---|---|---|---|---|
| `quit` | — | Player | `quit.go` | Closes the session. `Run` writes goodbye, closes `Conn`, returns `ErrSessionEnded`. |
| `who` | — | Player | `who.go` | `sessions.Registry.Snapshot()` → renders name + idle time (≥30s from `LastInputAt`). Pending: class/level/AFK columns. |
| `colors` | `colortest`, `palette` | Player | `colors.go` | Terminal info, 16-color palette, xterm-256 cube + grayscale, RGB ramp + style samples. |
| `help` | `?` | Guest | `help.go` | Lists registered verbs (matching AuthLevel); `help <verb>` shows `Help`/`Long`. |
| `look` | `l` | Player | `look.go` | Renders `Session.CurrentRoomID`: name, long desc, exits + directions, items, mobs. Empty subsections collapse. Reused by move. |
| `north` / `south` / `east` / `west` / `up` / `down` | `n` / `s` / `e` / `w` / `u` / `d` | Player | `move.go` | 8 move commands (+ 4 diagonals: `ne`/`nw`/`se`/`sw` via aliases). Each calls `moveDir`: resolve exit → update `CurrentRoomID` → `CharacterRepo.RecordRoom` → re-render via `RenderRoom`. Publishes `world.PlayerEntered/Left` to eventbus. |
| `teleport` | `tp` | Admin | `teleport.go` | `teleport <room_id>` jumps to target room, bypassing exits. Re-renders new room. Requires `AuthAdmin`. |
| `say` | — | Player | `comm.go` | Broadcasts to room: `You say, "<text>"` to sender, `<Name> says, "<text>"` to others. Sanitizes control bytes + caps text length. |
| `tell` / `whisper` | — | Player | `comm.go` | `tell <name> <text>`: private message via `sessions.Registry.FindByCharacterName`. Sets recipient's `LastTellFrom`. Sanitized like `say`. |
| `reply` | — | Player | `comm.go` | `reply <text>`: writes back to `Session.LastTellFrom` (the last player who `tell` to you). |
| `alias` | — | Player | `alias.go` | `alias <alias> <expansion>`: adds a per-session alias. `alias` (no args) lists active aliases. Single-pass expansion in `Registry.Dispatch`. |
| `unalias` | — | Player | `alias.go` | `unalias <alias>`: removes an alias. |
| `channels` | `ch` | Player | `channel.go` | Lists all channels with on/off mute state. Pending: level gating, auto-leave at level 10. |
| `<channel>` | — | Player | `channel.go` | Dynamic verb per channel row (e.g., `ooc`, `gossip`, `newbie`). No args toggles mute via `CharacterRepo.RecordChannelSettings`. Args broadcast `[<NAME>] <speaker>: <text>` to unmuted sessions. `channelMuted` bitmask on `Session` is crossMu-guarded. |

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

## Auth gating

- `AuthGuest` (default until login): `help` only.
- `AuthPlayer` (post-login): all game commands + comms + channels.
- `AuthAdmin`: OLC commands (future); `teleport`. Denials render as `"Unknown command"` so the prompt can't enumerate privileged verbs.

## Deferred

- Quoted-argument tokenization (e.g., `say "hello world"`).
- Per-command argument completion.
- Per-character aliases persisted to DB (today session-only).
- `whisper` (room-local variant), ignore-list filtering, `nochannels` flag.

See `command_input_followups.md` (auto-memory) for triggers and sketches.
