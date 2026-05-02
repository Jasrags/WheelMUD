<!-- Generated: 2026-05-02 | Files scanned: ~45 (.go) | Token estimate: ~950 -->

# Architecture

WheelMUD is a single-binary Go MUD server. One TCP listener fans out to a goroutine-per-connection model. The auth layer is wired end-to-end (accounts + bcrypt + login + multi-session policy + characters); the world layer ships rooms/exits/items/mobs as YAML-authored aggregates loaded into SQLite at boot, with `look`, movement (`n/s/e/w/u/d`/`ne`/`nw`/`se`/`sw`), and room persistence across reconnects. The scheduler manages heartbeat ticks, persistence autosave, and event dispatch. Session registry enables multi-connection awareness (`who`, `tell`, channels with per-character mute toggles).

## Layers

```
┌────────────────────────────────────────────────────────────────────┐
│ cmd/server/main.go         listener + DI wiring + graceful shutdown│
├────────────────────────────────────────────────────────────────────┤
│ internal/tick/             Scheduler (1 Hz) + named Buckets        │
│ internal/persist/          Autosave manager + Save bucket sub      │
│ internal/eventbus/         Typed pub/sub (PlayerEntered/Left)      │
│ internal/session/          Process-level session.Registry          │
│ internal/safego/           panic-recovery wrapper + LOG_LEVEL env  │
├────────────────────────────────────────────────────────────────────┤
│ internal/mode/             Mode stack implementations               │
│   login / create / character_select / character_create / game      │
│ internal/cmd/              Concrete commands (closures + singletons)
│   quit / who / colors / help / look / move-family (n/s/e/w/u/d/   │
│   ne/nw/se/sw) / teleport / say / tell / reply / channels /alias  │
├────────────────────────────────────────────────────────────────────┤
│ internal/auth/             bcrypt Hash / Verify                    │
│ internal/repo/             Account + Character + Room/Exit/Item/   │
│                             MobTemplate/MobInstance/Channeling/    │
│                             Channel repos (sqlite + memory + test) │
│ internal/creature/         Core + Abilities + Channeling models    │
│ internal/currency/         Amount type + denomination conversions  │
│ internal/world/            YAML loader: parse → validate → tx-sync │
│                             into rooms/exits/items/mobs. Embedded  │
│                             default; WORLD_DIR env overrides.      │
├────────────────────────────────────────────────────────────────────┤
│ internal/db/               SQLite Open + 11 embedded migrations     │
├────────────────────────────────────────────────────────────────────┤
│ telnet/                    protocol + I/O + mode/registry/dispatch │
│   ├ server.go              RunSession, readLoop, dispatcher        │
│   ├ session.go             Session, write lock, mode stack, auth   │
│   ├ iac.go                 IAC/SB negotiation (TERM_TYPE/NAWS)     │
│   ├ command.go             Registry, Lookup, Dispatch (Auth check) │
│   ├ mode.go / completion   Mode interface + Tab completion         │
│   ├ wrap.go / color.go     ANSI-aware wrap, SGR, RGB downsampling │
│   └ ascii.go               Control-byte constants                  │
└────────────────────────────────────────────────────────────────────┘
```

Dependency direction (no cycles): `cmd/server` → `internal/{tick,persist,eventbus,session,safego,mode,cmd,auth,repo,creature,currency,world,db}` → `telnet`. `telnet` is foundational and imports nothing internal.

## Boot

```
main ─► db.Open(DB_DSN)                      runs embedded migrations 0001-0011
     ─► repo.NewSQLite{Account,Character,    wraps *sql.DB
        Room,Exit,Item,MobTemplate,
        MobInstance,Channeling,Channel}Repo
     ─► channelRepo.List(ctx)                load channel catalog (ooc/gossip/newbie)
     ─► world.LoadAndSync(ctx, conn,         parses YAML, validates,
        world.SourceFS())                    tx-inserts all world aggregates
                                             (no-op if rows exist). Aborts
                                             boot on validation fail.
     ─► session.NewRegistry()                process-level; tracks bind/unbind
     ─► buildRegistry(…, channels)           closures + singletons; dynamic
                                             channel commands registered from
                                             catalog; registers say/tell/reply/who
     ─► tick.New() + tick.NewBuckets        scheduler (1 Hz) + named buckets
        (combat/regen/areaReset/Save)       (Save default 30s, calls persist.FlushAll)
     ─► persist.New()                        autosave manager; saver registered
        → saves.Register("character.lastPlayed", savePlayTimes) → buckets.Save.Subscribe
     ─► eventbus.New()                       typed pub/sub (PlayerEntered/Left)
     ─► mode.NewGame(registry)               stateless, shared
     ─► server{…}                            all repos, registry, scheduler,
                                             buckets, bus, saves, sessions,
                                             newInitial factory
     ─► scheduler.Start(ctx)                 heartbeat goroutine
     ─► net.Listen → Accept loop             per-conn goroutines via safego.Go
```

## Per-connection lifecycle

```
Accept ─► NewSession ─► writeBanner ─► PushMode(srv.newInitial())  = fresh Login
                                    ─► RunSession
                                               │
                                   ┌───────────┴──────────┐
                                   ▼                      ▼
                              readLoop (1 g/r)      runDispatcher (1 g/r)
                              dispatchByte:         inbox ─► Mode.Handle(ctx)
                                ├─ IAC/ANSI        ─► Registry.Dispatch(ctx)
                                ├─ CR/LF           ─► Auth check + Run
                                ├─ BS/DEL/HT       ─► WriteRaw(prompt)
                                └─ printable
                                   └─ inbox (cap 16)

teardown:
  ├─ readLoop exits on: EOF / idle 10m / flood (ErrInputFlooded)
  ├─ runDispatcher drains inbox and stops without prompting
  ├─ safego.Go wrapper recovers panics from either goroutine
  ├─ if s.AccountID != 0: sessions.Unbind(s.AccountID, s)  (compare-and-delete)
  └─ s.Conn.Close()
```

- `readLoop` owns `Session.InputBuffer`. Parses IAC, ANSI, CR/LF, BS/DEL, HT, printable, history (↑/↓), cursor motion (←/→/Home/End). **Redacts logged input** when `s.InPasswordMode` is true.
- `runDispatcher` consumes `s.inbox`, calls `mode.Handle(ctx, s, line)`. `ctx` is canceled after readLoop exits, before inbox closes, so blocking handlers observe cancellation.
- Writes serialize on `Session.writeMu`; both goroutines go through `WriteRaw`. `AuthLevel` (Guest/Player/Admin) checked at dispatch time, not mode entry.
- Session tracks: `AccountID`, `CharacterID`, `CharacterName`, `CurrentRoomID` (persisted via `CharacterRepo.RecordRoom`), `LastTellFrom`, `LastInputAt` (for idle), `channelMuted` (crossMu-guarded bitmask).

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
- World loader is boot-time only — no hot-reload yet (§7), no spawn/despawn lifecycle, no item/mob template-vs-instance split (§9).
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
