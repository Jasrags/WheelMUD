<!-- Generated: 2026-04-30 | Files scanned: ~30 (.go) | Token estimate: ~850 -->

# Architecture

WheelMUD is a single-binary Go MUD server. One TCP listener fans out to a goroutine-per-connection model. The auth layer is wired end-to-end (accounts + bcrypt + login + multi-session policy + characters), and the first slice of the world model has landed — rooms, exits, items, and mobs as read-only seeded aggregates plus `look` and `n/s/e/w/u/d` movement that persists across reconnects.

## Layers

```
┌───────────────────────────────────────────────────────────────────┐
│ cmd/server/main.go         listener + DI wiring                   │
├───────────────────────────────────────────────────────────────────┤
│ internal/mode/             Mode implementations                   │
│   game / login / create / character_select / character_create     │
│ internal/cmd/              Concrete commands                      │
│   quit / who / help / colors / look / move-family (n/s/e/w/u/d)   │
├───────────────────────────────────────────────────────────────────┤
│ internal/auth/             bcrypt Hash / Verify                   │
│ internal/repo/             Account + Character + Room/Exit/Item/  │
│                             Mob repos (sqlite + memory impls;     │
│                             shared contract tests)                │
│ internal/session/          Process-level session.Registry         │
├───────────────────────────────────────────────────────────────────┤
│ internal/db/               SQLite Open + embedded migrations      │
├───────────────────────────────────────────────────────────────────┤
│ telnet/                    protocol + I/O + dispatch core         │
│   ├ server.go              RunSession, readLoop, dispatcher       │
│   ├ session.go             Session, write lock, mode stack        │
│   ├ iac.go                 IAC/SB negotiation                     │
│   ├ command.go             Registry, Command, Dispatch (Auth chk) │
│   ├ mode.go                Mode interface + ErrNoMode             │
│   ├ completion.go          Tab-completion column layout           │
│   ├ wrap.go                ANSI-aware word wrap                   │
│   ├ color.go               SGR + RGB downsampling                 │
│   └ ascii.go               Control-byte constants                 │
└───────────────────────────────────────────────────────────────────┘
```

Dependency direction (no cycles): `cmd/server` → `internal/{mode,cmd,session,repo,db,auth}` → `telnet`. `telnet` is the foundational package and imports nothing internal.

## Boot

```
main ─► db.Open(DB_DSN)                           runs embedded migrations
     ─► repo.NewSQLite{Account,Character,Room,    wraps *sql.DB
                       Exit,Item,Mob}Repo
     ─► session.NewRegistry()                     process-level
     ─► buildRegistry(rooms, exits, items, mobs,  closures over world repos
                      characters)                  for look + move family
     ─► mode.NewGame(registry)                    stateless, shared
     ─► server{ accounts, characters, world repos,
                sessions, newInitial: …NewLogin } per-conn factory
     ─► net.Listen → Accept loop ─► srv.handleConnection per conn
```

## Per-connection lifecycle

```
Accept ─► NewSession ─► writeBanner
                   ─► PushMode(srv.newInitial())   = fresh Login
                   ─► RunSession
                                  │
                       ┌──────────┴──────────┐
                       ▼                     ▼
                 readLoop (1 g/r)      runDispatcher (1 g/r)
                 bytes ─► dispatchByte    inbox ─► Mode.Handle(ctx, ...)
                       ─► inbox          ─► WriteRaw(prompt)

teardown defer:
  - if s.AccountID != 0: sessions.Unbind(s.AccountID, s)  (compare-and-delete)
  - s.Conn.Close()
```

- `readLoop` owns `Session.InputBuffer`. Parses IAC, ANSI, CR/LF, BS/DEL, HT, printable. Logged input is **redacted** when `s.InPasswordMode` is true.
- `runDispatcher` consumes `s.inbox`, calls `mode.Handle(ctx, s, line)`. `ctx` is canceled by `RunSession` after the read loop exits, before the inbox is closed, so blocking handlers see cancellation.
- Writes serialize on `Session.writeMu`; both goroutines (and any future emitter) go through `WriteRaw`.

## Auth pipeline

```
Login.handleUsername:
  "new" → ReplaceMode(Create)
  else  → FindByUsername (cache l.account; nil if not found)
        → InPasswordMode = true; advance to password step

Login.handlePassword:
  re-fetch account (lockout TOCTOU defense)
  IsLockedAt(now)? → "Account temporarily locked." reset
  Verify? no:
    RecordLoginFailure (+ locked_until if threshold hit)
    "Login failed." reset
  yes:
    RecordLoginSuccess (clears counters)
    s.AccountID = …; s.AuthLevel = AuthPlayer
    sessions.Bind(accountID, s) → kick prior occupant if any
    postAuth(ctx, s, characters, game)
       0 chars → ReplaceMode(CharacterCreate)
       1 char  → promoteToGame (auto-pick)
       2+ chars→ ReplaceMode(CharacterSelect)
```

`promoteToGame` stamps `CharacterID`, `CharacterName`, and `CurrentRoomID` onto the session (defaulting to `repo.StarterRoomID` if the row has none) so the first `look` resolves immediately. Movement commands write `CurrentRoomID` back via `CharacterRepo.RecordRoom` so a reconnect picks up where the player left off.

Create mode mirrors Login on the success path (insert account, Bind, postAuth → CharacterCreate since 0 chars).

## Input → command path

```
TCP byte ─► dispatchByte ─► bufferInput ─► CR/LF ─► inbox ─► Mode.Handle(ctx)
                                                                │
                                                  Game.Handle ─► Registry.Dispatch(ctx)
                                                                │
                                                  Lookup(verb) ─► Auth check ─► Command.Run(*Context)
```

Verb resolution: alias → exact name → unique prefix. `MinArgs` enforced before `Run`. `Command.Auth` is checked against `Session.AuthLevel`; denials render as `"Unknown command"` so privileged verbs can't be enumerated.

## Tab completion

`handleTab` consults the current `Mode` if it implements `Completer`. `Game.Complete` returns registry verb candidates. Single match → in-place extension; multiple → list above the prompt and redraw. Argument-side completion is deferred. Tab in password mode is a hard bell.

## What's missing on purpose

- No game loop / tick scheduler (§8 of ROADMAP).
- World model is read-only and tiny — 3 seeded rooms, no authoring loader, no spawn/despawn lifecycle, no item/mob template-vs-instance split (§9).
- No combat, skills, economy, quests, OLC, channels.
- No `who`-across-the-server — needs `session.Registry.Snapshot` iteration; currently shows only the caller.
- See `ROADMAP.md` for the full ledger.

## Entry points

- `cmd/server/main.go::main` — listener, DI wiring, accept loop.
- `cmd/server/main.go::handleConnection` — per-connection setup + handoff to `telnet.RunSession`. Defers `sessions.Unbind` on teardown.
- `telnet.RunSession` — drives the read + dispatch goroutines.

## Data flow at a glance

```
client ──telnet──► readLoop ──inbox──► dispatcher ──Mode.Handle──► WriteRaw ──telnet──► client
                       │                                              ▲
                       └──────── inline echo / completion ────────────┘

Mode.Handle (Login/Create) ──repo──► SQLite (accounts, characters)
                          ──auth──► bcrypt
                          ──sessions──► Registry (kick prior)

Command.Run (look/move)   ──repo──► SQLite (rooms, exits, items, mobs)
                          ──repo──► SQLite (characters.RecordRoom on move)
```
