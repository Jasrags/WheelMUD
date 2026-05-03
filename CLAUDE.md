# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

WheelMUD is a Wheel of Time MUD server in Go (1.25). It listens on TCP `:2323`,
performs telnet option negotiation, and runs a per-connection line-based
command loop driven by a registry/mode-stack dispatcher. SQLite (modernc
pure-Go driver) backs accounts, characters, rooms, exits, items, mobs, and
chat channels. A YAML world loader (`internal/world/`) seeds the DB from
`data/world/` on first boot.

`ROADMAP.md` at the repo root tracks what's done vs. pending across the major
MUD subsystems and is the source of truth for "what's next." Token-lean
architecture maps for AI context live in `docs/CODEMAPS/`.

## Common commands

```bash
make build/server      # go build -o /tmp/bin/server cmd/server/main.go
make run/server        # build then run the binary
make run/live/server   # hot reload via cosmtrek/air (runs go mod tidy first)
go test -race ./...    # full test suite with race detector
docker compose up      # build + run, exposes :2323
```

Connect with: `telnet localhost 2323` (or `nc localhost 2323`).

Environment: `LISTEN_ADDR` (default `:2323`), `DB_DSN` (default `wheelmud.db`,
`:memory:` works), `LOG_LEVEL` (`debug`/`info`/`warn`/`error`, default
`debug`), `WORLD_DIR` (default `./data/world`).

## Architecture

- **`cmd/server/main.go`** — entrypoint. Reads env, opens the DB via
  `internal/db.Open` (runs embedded migrations 0001–0016), constructs every
  repo (accounts, characters, rooms, exits, items, mob_instances, zones,
  channels), runs `world.LoadAndSync` to seed the DB from `WORLD_DIR`,
  builds the command registry plus a `server` struct holding long-lived
  deps, starts `tick.Scheduler` + `tick.Buckets` and the `persist.Manager`
  autosaver, then accepts TCP connections. Each connection gets a
  `telnet.Session`, the initial login mode is pushed, and
  `telnet.RunSession` drives it. New long-lived dependencies belong on
  the `server` struct.

- **`telnet/`** — protocol primitives + per-connection driver.
  - `session.go`: `Session` struct (conn, terminal type, width/height,
    line-edit buffer, history, password-mode flag, color level, write
    mutex, mode stack, AuthLevel, AccountID, CharacterID/Name,
    CurrentRoomID, alias table). `crossMu` guards fields written by one
    goroutine and read by another (`lastTellFrom`, `lastInputAt`,
    `channelMuted`); use the `Set/Get/Toggle/Snapshot` helpers, never
    touch the unexported fields directly. `WriteString` renders cfmt
    tags; `WriteWrapped` reflows to `Session.Width`. All writes
    serialize on `writeMu`.
  - `server.go`: `RunSession` plus the byte parser (`readLoop`,
    `dispatchByte`, `bufferInput`, `handleLineBreak`, `handleBackspace`,
    `handleTab`) and the per-session dispatcher goroutine
    (`runDispatcher`).
  - `iac.go`, `color.go`, `wrap.go`, `command.go`, `mode.go`,
    `completion.go`, `alias.go`, `tokenize.go`, `lineedit.go`,
    `history.go`, `ascii.go` — protocol/IAC, ANSI color downsampling,
    word wrap, command registry + `Mode` interface, tab completion,
    aliases, quoted tokenizer, line editor, history ring.

- **`internal/cmd/`** — concrete commands wired in `main.go::buildRegistry`:
  `quit`, `colors`, `who`, `say`, `tell`, `reply`, `alias`/`unalias`, one
  verb per channel catalog row plus a `channels` overview, `help`, `look`,
  `examine`, the move family (`n`/`s`/`e`/`w`/`u`/`d`/etc.), `teleport`,
  the door verbs (`open`/`close`/`lock`/`unlock`/`pick`), the inventory
  verbs (`inventory`/`get`/`drop`/`give`), and the admin inspectors
  (`whereami`, `zones`). New commands take their dependencies (repos,
  registry, sessions, bus) by parameter and return a `*telnet.Command`.
  Item/mob keyword resolution (including ordinal `2.sword`) goes through
  `keyword.go::MatchItem` / `MatchMob`; encumbrance bands come from
  `encumbrance.go::LoadFor` (Str-keyed d20 carrying-capacity table).

- **`internal/mode/`** — login, character_select, character_create, game,
  postauth promotion. `promoteToGame` stamps `CharacterID`,
  `CharacterName`, `CurrentRoomID`, calls `Session.SetChannelMuted` from
  the loaded character, then ReplaceMode's into game.

- **`internal/repo/`** — typed repos with sqlite + memory implementations
  and a shared test suite. Every repo writes through on mutation; the
  `persist.Manager` Save bucket layers periodic + shutdown flushes for
  fields that aren't covered (e.g. `last_played_at`).

- **`internal/db/migrations/`** — embedded migrations 0001–0017. Each
  migration is forward-only (no down). 0008 introduced the polymorphic
  creature/mob_template/mob_instance/channeling tables; 0010 dropped
  the legacy `mobs` table; 0011 added the chat-channel catalog +
  `channels.channel_settings_json`; 0012 added room flags + sector;
  0013 added room extra-descs JSON; 0014 added exit door flags + key
  + lock difficulty + description; 0015 added the item taxonomy
  columns; 0016 added the `zones` table + `rooms.zone_id` (soft FK,
  default 0; loader stamps real ids); 0017 added
  `items.owner_character_id` (nullable, soft FK) so items can sit on
  a room floor or in a character's inventory but never both.

- **`internal/world/`** — YAML zone loader that syncs `WORLD_DIR` into the
  DB on startup (zones/rooms/exits/items/mob_templates/mob_instances).
  The on-disk tree is hierarchical (continent → nation → region →
  settlement → building); see `data/world/README.md` for the full
  zone.yaml schema (id, name, builder, level_range, reset_interval_s,
  reset_mode, climate, ambient) and the room-id / currency-string /
  typed-item-stats conventions builders need to know.

- **`internal/session/`** — process-level registry that enforces
  single-session-per-account: `Bind` returns the displaced session;
  `Unbind` is compare-and-delete so a stale teardown defer can't blow
  away a newer session. `FindByCharacterName` and `Snapshot` power
  `tell`/`who`/channel broadcasts.

- **`internal/eventbus/`**, **`internal/tick/`**, **`internal/persist/`**,
  **`internal/safego/`** — typed pub/sub (`Publish`/`Subscribe[T]`),
  scheduler + named buckets, periodic+shutdown autosave manager, and a
  panic-recovery goroutine wrapper (`safego.Go("name", fn)`) used for
  every long-lived goroutine.

- **`internal/auth/`** — bcrypt password hashing (with a tunable cost
  knob; see `auth_followups.md` memory for the SetCost issues).

- **`internal/creature/`** — `Core` stat block (abilities, HP, saves,
  speed, conditions, position flags, DR/resists) shared by characters
  and mob_templates, plus the `Channeling` weave model.

### Things to watch when editing

- The protocol parser lives in `telnet/server.go`.
- `Session.Input` (line edit buffer) is owned by the read goroutine
  inside `RunSession`. Do not mutate it from another goroutine.
- `Session.WriteRaw` is the only safe write path; it holds `writeMu`.
  Layer new helpers on top of it rather than calling `Conn.Write` directly.
- Cross-goroutine session fields (`lastTellFrom`, `lastInputAt`,
  `channelMuted`) MUST go through the helpers — they take `crossMu`.
  In-world fields (`CharacterID`, `CharacterName`, `CurrentRoomID`) are
  dispatcher-owned; treat snapshots from `session.Registry.Snapshot()`
  as values that can change underfoot.
- `Mode.Handle(ctx, *Session, line)` is invoked synchronously by
  `runDispatcher`. The ctx is canceled when the read loop exits
  (EOF / idle / flood); handlers doing blocking I/O must observe it.
  A slow handler stalls input for that session.
- `Registry.Dispatch` enforces `Command.Auth` against `Session.AuthLevel`.
  Privilege-denied lookups return the same `Unknown command` text as a
  missing verb so the prompt can't enumerate privileged commands.
- Logging uses `slog`; level is set in `main.go` from `LOG_LEVEL`.
- Spawn long-lived goroutines via `safego.Go("name", fn)` so panics
  surface as warnings instead of taking down the process.
- New columns on `characters` need to land in BOTH `charPlayerColumns`
  AND `charPlayerValues` AND `charPlayerScanDest` in lock-step
  (`internal/repo/character_sql.go`); ordering is load-bearing.
- New columns on `rooms` need to land in BOTH `roomSelectCols` AND the
  `insertCols`/`insertVals` lists in `internal/repo/room_sqlite.go`
  AND the `cols`/`vals` materialized in `internal/world/loader.go::
  roomInsertValues`; the loader writes raw SQL inside one transaction
  rather than going through `RoomRepo.Create`, so the column lists are
  duplicated and must move in lock-step.
- New columns on `items` need to land in `itemSelectCols`,
  `scanItemRow`, the `Create` INSERT, AND the loader-side INSERT in
  `internal/world/loader.go::insertItems` (raw SQL, single transaction).
  Same lock-step rule as rooms.
- Items live in exactly one location: either `room_id` is set (on the
  floor) or `owner_character_id` is set (in someone's inventory), never
  both. `ItemRepo.SetOwner` / `SetRoom` flip both columns atomically;
  do not write the columns directly. `Character.Inventory` (JSON id
  list on `inventory_json`) is just the display ordering — SQL
  `owner_character_id` is the source of truth, and `inventory.go::
  orderInventory` self-heals stale or missing JSON entries.

## Tests

`go test -race ./...` covers the registry, mode dispatcher, completion
handler, IAC parser, color helpers, word wrap, tokenizer, line editor,
alias table, every repo (memory + sqlite, including the new ZoneRepo),
the world loader (including zone metadata + room.zone_id linkage), the
session registry, the eventbus, the tick scheduler, the persist
manager, and the concrete commands (look / move / say / tell / reply /
channel / teleport / alias / examine / door verbs / inventory verbs /
whereami / zones).
Telnet-package tests reuse `newPipeSession(t)` / `bufSession(t)` /
`bufConn` from `telnet/command_test.go`. Cmd-package tests reuse
`commPair` / `runCmd` from `internal/cmd/comm_test.go`.

## Module

`github.com/Jasrags/WheelMUD`. Direct deps: `github.com/i582/cfmt`
(styling), `golang.org/x/crypto` (bcrypt), `gopkg.in/yaml.v3` (world
loader), `modernc.org/sqlite` (pure-Go SQLite). See
`docs/CODEMAPS/dependencies.md` for the full picture.
