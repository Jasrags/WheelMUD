<!-- Generated: 2026-04-30 | Files scanned: 23 (.go) | Token estimate: ~700 -->

# Architecture

WheelMUD is a single-binary Go MUD server. One TCP listener fans out to a goroutine-per-connection model; SQLite-backed accounts and login/account-create modes are wired, but there is no world state yet — the surface area is the telnet/ANSI transport, a command-registry/mode-stack input pipeline, and the auth layer (accounts + bcrypt + mode-driven login flow).

## Layers

```
┌─────────────────────────────────────────────────────────────┐
│ cmd/server/main.go        listener + registry wiring        │
├─────────────────────────────────────────────────────────────┤
│ internal/mode/            Mode implementations (Game)       │
│ internal/cmd/             Concrete commands                 │
├─────────────────────────────────────────────────────────────┤
│ telnet/                   protocol + I/O + dispatch core    │
│   ├ server.go             RunSession, readLoop, dispatcher  │
│   ├ session.go            Session, write lock, mode stack   │
│   ├ iac.go                IAC/SB negotiation                │
│   ├ command.go            Registry, Command, Dispatch       │
│   ├ mode.go               Mode interface + ErrNoMode        │
│   ├ completion.go         Tab-completion column layout      │
│   ├ wrap.go               ANSI-aware word wrap              │
│   ├ color.go              SGR + RGB downsampling            │
│   └ ascii.go              Control-byte constants            │
└─────────────────────────────────────────────────────────────┘
```

`telnet/` knows nothing about specific commands. `internal/cmd/` and `internal/mode/` import `telnet`, never the reverse.

## Boot

```
main ─► open SQLite (db.Open runs embedded migrations)
     ─► build Registry (commands)
     ─► server{ accounts: SQLiteAccountRepo,
                 newInitial: () => Login(accounts, Game(registry)) }
     ─► net.Listen → Accept loop ─► srv.handleConnection per conn
```

## Per-connection lifecycle

```
Accept ─► NewSession ─► writeBanner ─► PushMode(srv.initial) ─► RunSession
                                                            │
                                          ┌─────────────────┴──────────────────┐
                                          ▼                                    ▼
                                    readLoop (1 goroutine)              runDispatcher (1 goroutine)
                                    bytes ─► dispatchByte ─► inbox ─►  Mode.Handle ─► WriteRaw(prompt)
```

- `readLoop` owns `Session.InputBuffer`. It parses IAC, ANSI, CR/LF, BS/DEL, HT, and printable bytes. CRLF lines flow through `Session.inbox` (cap = 16; overflow returns `ErrInputFlooded` and tears down).
- `runDispatcher` pops lines, calls the top mode's `Handle`, and writes the next prompt. `ErrSessionEnded` from a `Handle` (e.g. `quit`) terminates the dispatcher without drawing the prompt.
- Writes serialize on `Session.writeMu`; both goroutines (and any future emitter) go through `WriteRaw`.

## Input → command path

```
TCP byte ─► dispatchByte ─► bufferInput ─► CR/LF ─► inbox ─► Mode.Handle
                                                                │
                                                  Game.Handle ─► Registry.Dispatch
                                                                │
                                                  Lookup(verb) ─► Command.Run(*Context)
```

Verb resolution: alias → exact name → unique prefix. `MinArgs` enforced before `Run`. `Command.Auth` is checked against `Session.AuthLevel`; denials render as `"Unknown command"` so privileged verbs can't be enumerated.

## Tab completion

`handleTab` consults the current `Mode` if it implements `Completer`. `Game.Complete` returns registry verb candidates. Single match → in-place extension; multiple → list above the prompt and redraw. Argument-side completion is deferred.

## What's missing on purpose

- No character model yet — login/create handle accounts only; characters land in a later slice.
- No game loop / tick scheduler.
- No world model (rooms, items, mobs).
- See `ROADMAP.md` for the full ledger.

## Entry points

- `cmd/server/main.go::main` — listener, registry, accept loop.
- `cmd/server/main.go::handleConnection` — per-connection setup + handoff to `telnet.RunSession`.
- `telnet.RunSession` — drives the read + dispatch goroutines.

## Data flow at a glance

```
client ──telnet──► readLoop ──inbox──► dispatcher ──Mode.Handle──► WriteRaw ──telnet──► client
                       │                                              ▲
                       └──────── inline echo / completion ────────────┘
```
