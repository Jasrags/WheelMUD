# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

WheelMUD is a Wheel of Time MUD server in Go (1.25). It listens on TCP `:2323`,
performs telnet option negotiation, and runs a per-connection line-based
command loop driven by a registry/mode-stack dispatcher. SQLite (modernc
pure-Go driver) backs accounts, characters, rooms, exits, items, mobs, and
chat channels. A YAML world loader (`internal/world/`) seeds the DB from
`data/world/` on first boot.

`ROADMAP.md` tracks what's done vs. pending and is the source of truth for
"what's done." `docs/PLAN.md` is the source of truth for "what's next" — it
sequences the roadmap into ordered phases (A: quick wins, B: equipment/
economy, C: combat, D: progression, E: NPCs/quests, F: OLC, G/H/I: comms/
network/ops, J: ops/CI/packaging — Phase J landed 2026-05-12). When ROADMAP
and PLAN disagree about *status*, ROADMAP wins; about *order*, PLAN wins.
Token-lean architecture maps live in `docs/CODEMAPS/`.

## Common commands

```bash
make build/server      # go build -o /tmp/bin/server cmd/server/main.go
make run/server        # build then run the binary
make run/live/server   # hot reload via cosmtrek/air
go test -race ./...    # full test suite with race detector
docker compose up      # build + run, exposes :2323
```

Connect with: `telnet localhost 2323`.

Configuration: pass `-config <path>` for YAML (see `config.example.yaml`) or
rely on env vars only — both work; env overrides file values. Env surface:
`LISTEN_ADDR` (default `:2323`), `METRICS_ADDR` (default `127.0.0.1:9090`;
empty disables metrics+pprof+healthz), `DB_DSN` (default `wheelmud.db`,
`:memory:` works), `BACKUP_DIR` (empty disables snapshots), `LOG_LEVEL`,
`WORLD_DIR` (default `./data/world`), `AUDIT_COMMANDS_ENABLED` +
`AUDIT_COMMANDS_EXCLUDE`. Catalog dirs (`CHARGEN_DIR` / `QUEST_DIR` /
`SCRIPT_DIR` / `EFFECTS_DIR`) are env-only and switch each embedded-FS
catalog to an on-disk override.

## Architecture

- **`cmd/server/main.go`** — entrypoint. Parses flags, loads YAML+env via
  `internal/config.Load`, opens the DB via `internal/db.Open` (runs embedded
  migrations), constructs every repo, loads catalogs (news/chargen/quest),
  runs `world.LoadAndSync`, builds the command registry on a `server`
  struct holding long-lived deps, starts `tick.Scheduler` + `tick.Buckets`
  and `persist.Manager`, conditionally starts the backup manager
  (`internal/backup`) and metrics HTTP server (`internal/metrics`), then
  accepts TCP. Each connection gets a `telnet.Session`; `telnet.RunSession`
  drives it. After login, `news.WriteMOTDBlock` renders unseen MOTD/news.
  New long-lived deps belong on the `server` struct. `buildVersion` /
  `buildCommit` / `buildDate` are populated by goreleaser ldflags.

- **`telnet/`** — protocol primitives + per-connection driver.
  - `session.go`: `Session` struct (conn, terminal type, width/height,
    line-edit buffer, history, password-mode flag, color level, write
    mutex, mode stack, AuthLevel, AccountID, CharacterID/Name,
    CurrentRoomID, alias table). `crossMu` guards cross-goroutine fields
    (`lastTellFrom`, `lastInputAt`, `channelMuted`); use the
    `Set/Get/Toggle/Snapshot` helpers. `WriteString` renders cfmt;
    `WriteWrapped` reflows to `Session.Width`. All writes serialize on
    `writeMu`.
  - `server.go`: `RunSession` plus the byte parser and the per-session
    dispatcher goroutine.
  - `iac.go`, `color.go`, `wrap.go`, `command.go`, `mode.go`,
    `completion.go`, `alias.go`, `tokenize.go`, `lineedit.go`,
    `history.go`, `ascii.go`.

- **`internal/cmd/`** — concrete commands wired in
  `main.go::buildRegistry`. See the directory for the full verb list
  (movement, comms/channels, inventory/equipment, shop/banker, combat,
  party, progression, dialogue, quest, admin, etc.). Conventions:
  - Commands take deps (repos, registry, sessions, bus) by parameter and
    return a `*telnet.Command`.
  - Required-args commands declare `MinArgs` + `Long`; dispatcher emits
    Long-aware usage on too-few-args.
  - Item/mob keyword resolution (incl. ordinal `2.sword`) goes through
    `keyword.go::MatchItem` / `MatchMob`.
  - Encumbrance bands: `encumbrance.go::LoadFor` (Str-keyed d20 table);
    sums recursive container contents via
    `ItemRepo.ListAllOwnedTransitive`.

- **`internal/mode/`** — login, character_select, character_create, game,
  postauth promotion. `promoteToGame` stamps `CharacterID`,
  `CharacterName`, `CurrentRoomID`, calls `Session.SetChannelMuted`, then
  ReplaceMode's into game.

- **`internal/repo/`** — typed repos with sqlite + memory implementations
  and a shared test suite. Every repo writes through on mutation; the
  `persist.Manager` Save bucket layers periodic + shutdown flushes for
  fields not otherwise covered (e.g. `last_played_at`).

- **`internal/db/migrations/`** — embedded forward-only migrations
  (0001–0052). Read the SQL directly when you need the schema; this file
  no longer narrates each migration. Key invariants codified by recent
  migrations:
  - `auth_level` lives on the character row, not the account (0019), and
    MUST stay the trailing column in `charPlayerColumns` /
    `charPlayerValues` / `charPlayerScanDest` because the SQLite
    first-character bootstrap CASE expression in `CharacterRepo.Create`
    consumes it as the trailing placeholder.
  - Items live in exactly one of three locations: `room_id`,
    `owner_character_id`, or `parent_item_id` (0017, 0028). Use
    `ItemRepo.SetOwner` / `SetRoom` / `SetParent` or the `Transfer*`
    family — they flip atomically and clear the other two.
  - `characters.coin_version` (0032) is an optimistic-lock token bumped
    by `RecordCoin`; mismatched writes return `ErrCoinConflict`.
  - `admin_audit` (0029) + `character_audit` (0052) +
    `account_logins` (0036) are append-only forensic logs.
  - `triggers.consecutive_faults` / `disabled` (0046) auto-disable a
    trigger after 5 faults; `world` loader resets via
    `TriggerRepo.ResetAllFaults` at boot.

- **`internal/scripts/`** — gopher-lua script catalog under
  `internal/scripts/default/` (embedded, `SCRIPT_DIR` override). Boot
  compiles every script; syntax errors fail boot loudly.

- **`internal/lua/`** — gopher-lua sandbox + runner. `NewSandboxedState()`
  strips dangerous globals (`os`, `io`, `debug`, `package`, `dofile`,
  `loadfile`, `loadstring`, `load`). `Runner` pre-allocates an LState
  pool (size 8) served via a buffered channel — NOT `sync.Pool`, which
  can synthesize states at Stop. `Runner.Run` wraps the parent ctx with
  `CallTimeout = 50ms` and propagates via `SetContext`. API surface:
  `say`, `emote`, `log`, read-only `ctx`; `quest.accept/advance`;
  `push_mode`; `apply_affect`, `give_item`; read-only `target` /
  `room` (resolved at bind time from `b.Ctx.RoomID`) / `clock`. nil-
  bound hooks register classified-error stubs so misuse trips the
  trigger fault budget instead of a generic nil call. `Runner.Stop()`
  closes every LState — must run BEFORE `bus.Stop()` in shutdown.

- **`internal/quest/`** — quest engine: catalog (`Tree`, `Step`,
  `Reward`), boot validator (cross-refs against world mob_template +
  room ExternalIDs + script catalog), event-driven engine.
  Subscribes to `combat.CombatDeath` (kill_n), `world.PlayerEntered`
  (reach_room); talk_to advances via the dialogue `advance_quest`
  effect; `script` steps wait for an external `quest.advance(id)`
  Lua call. Per-character state lives on
  `characters.quest_log_json` via `RecordQuestProgress`. Final-step
  XP via `RecordXP`, coin via `RecordCoin` with one optimistic-lock
  retry on `ErrCoinConflict`. All player-facing notifications go
  through `Session.WriteAsync` (engine runs on the eventbus
  goroutine). Dialogue effects (`accept_quest`, `advance_quest`,
  `script`) are wired via `mode.DialogueHooks` from
  `cmd/server/main.go` so `internal/cmd` and `internal/dialogue`
  stay free of `internal/quest` imports.

- **`internal/progression/`** — pure-function d20 XP curve + level-up
  math. `XPForLevel`, `LevelForXP`, `XPToNext` (MaxLevel=20).
  `ComputeLevelUp(ch, cat, classKey) → LevelGains` recomputes
  ClassLevels + HP/BAB/saves and per-pool deltas
  (Feat/Skill/Ability/Weave) the cmd-layer hands to
  `RecordLevelUp` via `LevelUpFields`. No DB, no session.

- **`internal/channeling/`** — per-tick driver for channeler state.
  `RefreshIfDue` refills `Slots[*].Cur` once 8h has elapsed;
  `AccrueMadness` adds `MadnessPerPulse` iff `Embraced` + drawing
  on `SourceSaidin`. Both no-op when `Stilled`. `SessionTicker` is
  subscribed to `tick.Buckets.Regen` (30s).

- **`internal/group/`** — in-memory party manager (`MaxGroupSize = 6`,
  leader-leaves-disbands). Wired into combat via
  `combat.GroupResolver` so `expandTallyByGroup` splits per-character
  damage across in-room party members at XP-award time. No
  persistence — server restart drops party state.

- **`internal/world/`** — YAML zone loader that syncs `WORLD_DIR` into
  the DB on startup. On-disk tree is hierarchical (continent → nation
  → region → settlement → building); see `data/world/README.md` for
  the full zone.yaml schema, optional `shop:` / `banker:` mob
  sub-blocks, and the room-id / currency-string / typed-item-stats
  conventions. `LoadAndSync` parses + validates YAML on every boot
  (even when DB is populated) and returns a `LoadedWorld` whose
  `ItemSpecsByZone` feeds the `ZoneResetter`. Also hosts the
  `Restocker` (refills sub-max `shop_stock`, on
  `tick.Buckets.AreaReset`), the `ZoneResetter` (mob respawn from
  anchored templates → door restoration via
  `ExitRepo.RestoreAuthored` → item respawn via
  `ItemRepo.FindByExternalID` + `Create`), and `Clock.HourOfDay`.

- **`internal/chargen/`** — YAML chargen catalog (backgrounds,
  classes, feats, skills, weaves) loaded once at boot from
  `internal/chargen/default/*.yaml` (or `CHARGEN_DIR` override).
  Cross-references validated at Load time; catalog typos fail boot.

- **`internal/session/`** — process-level registry enforcing single-
  session-per-account: `Bind` returns the displaced session;
  `Unbind` is compare-and-delete. `FindByCharacterName` and
  `Snapshot` power `tell`/`who`/channel broadcasts.

- **`internal/eventbus/`, `internal/tick/`, `internal/persist/`,
  `internal/safego/`** — typed pub/sub (`Publish`/`Subscribe[T]`),
  scheduler + named buckets, periodic+shutdown autosave, and
  `safego.Go("name", fn)` for panic-safe long-lived goroutines.

- **`internal/config/`** — YAML+env loader; precedence "struct defaults
  → YAML file → env". Empty path = env-only; missing/malformed file
  returns a wrapped error.

- **`internal/metrics/`** — Prometheus + pprof + healthz. Fresh
  `prometheus.Registry`, `Handler()` mounts `/metrics`, `/healthz`,
  `/debug/pprof/*`. Collectors: `wheelmud_commands_total{verb,result}`,
  `wheelmud_sessions_active`, `wheelmud_db_open_conns`,
  `wheelmud_build_info`, plus Go + Process. `SetReady` atomic drives
  healthz (200 once ready AND DB ping passes within 500ms). Default
  bind is loopback so pprof never leaks. `mode.Game` takes a
  `CommandMetricFn` hook so `internal/mode` doesn't import metrics.

- **`internal/backup/`** — wall-clock `VACUUM INTO` + retention pruning.
  `Manager.Run(ctx)` takes an initial snapshot then loops on a
  `time.Ticker` (decoupled from `tick.Buckets`). Pruning calls
  `os.Lstat` before `os.Remove` (refuses symlinks). Snapshot errors
  log warn and loop continues.

- **`internal/auth/`** — bcrypt password hashing (tunable cost; see
  `auth_followups.md` memory).

- **`internal/creature/`** — `Core` stat block (abilities, HP, saves,
  speed, conditions, position flags, DR/resists) shared by characters
  and mob_templates, plus the `Channeling` weave model.

### Things to watch when editing

- The protocol parser lives in `telnet/server.go`.
- `Session.Input` is owned by the read goroutine inside `RunSession`. Do
  not mutate it from another goroutine.
- **Telnet option negotiation responses** go through
  `telnet/iac.go::handleOptionNegotiation` (WILL/WONT/DO/DONT) and
  the `HandleSubnegotiation` switch (SB…SE). New options follow the
  CHARSET / MSSP pattern: add the option constant + sub-codes,
  append a `WILL <opt>` to `NegotiateTelnet`, and write the response
  via `s.WriteRaw` from `handleOptionNegotiation`. Session-state
  toggles set by subnegotiation (e.g. `Session.charset`) use the
  `crossMu` accessor pair (`SetCharset` / `Charset`). MSSP variables
  are produced by a `Session.MSSPProvider` closure wired in
  `cmd/server/main.go::msspVars`; provider == nil silently no-ops.
- `Session.WriteRaw` is the only safe write path; it holds `writeMu`.
  Layer helpers on top of it rather than calling `Conn.Write` directly.
- **Cross-session output** (broadcasts to peers, channel fanout, mob
  arrival/departure, phase ambients, anything writing to a session
  that isn't the current dispatcher's `c.Session`) MUST use
  `Session.WriteAsync`, not `WriteString`. WriteAsync wraps with a
  CR+EL erase prefix and replays the cached prompt + line-edit buffer
  so a mid-line broadcast doesn't clobber input. The dispatcher caches
  prompts via `WritePrompt`; mode transitions clear via
  `ClearLastPrompt` (handled by `PushMode`/`PopMode`). Synchronous
  dispatcher output (command's own response to `c.Session`) uses
  `WriteString` because the dispatcher repaints the prompt immediately
  after `Mode.Handle` returns.
- Read-goroutine keystroke handlers in `telnet/server.go` wrap
  "decide echo + mutate Input + emit echo" in
  `Session.EditAndWrite(fn)`. `fn` runs under `writeMu` and returns
  the bytes to write. This serializes against `WriteAsync`,
  `WritePrompt`, and `listAndRedraw`. `InPasswordMode` writes from
  mode handlers go through `Session.SetPasswordMode(bool)` (also under
  `writeMu`).
- Cross-goroutine session fields (`lastTellFrom`, `lastInputAt`,
  `channelMuted`) MUST go through the helpers — they take `crossMu`.
  In-world fields (`CharacterID`, `CharacterName`, `CurrentRoomID`)
  are dispatcher-owned; treat snapshots from
  `session.Registry.Snapshot()` as values that can change underfoot.
- `Mode.Handle(ctx, *Session, line)` is invoked synchronously by
  `runDispatcher`. ctx is canceled on EOF / idle / flood; handlers
  doing blocking I/O must observe it. A slow handler stalls input for
  that session.
- `Registry.Dispatch` enforces `Command.Auth` against
  `Session.AuthLevel`. Privilege-denied lookups return the same
  `Unknown command` text as a missing verb (no enumeration).
- `Command.Lag` is the per-verb global cooldown stamped on
  `Session.nextReady` via `s.StampLag(cmd.Lag)` after a successful
  `cmd.Run`. The gate lives in `Registry.dispatchOne` (per-segment)
  so chained `;` inputs gate independently. Stamp on success only.
- `Registry.Dispatch` is segment-aware: top-level `;` outside quotes
  splits via `telnet.SplitOnSemicolon` (mirrors `Tokenize`'s
  quote/escape rules). Hard cap `maxSegmentsPerLine = 16`; alias
  expansion bounded at `maxAliasDepth = 3`.
- Logging uses `slog`; level set in `main.go` from `LOG_LEVEL`.
- Spawn long-lived goroutines via `safego.Go("name", fn)`.
- `shutdown` / `reboot` drive teardown by calling the same `stop`
  cancel `signal.NotifyContext` returns; the watcher goroutine then
  closes the listener and runs `srv.shutdown()` → `persist.FlushAll`.
  `reboot` flips `srv.rebootOnExit`; `main()` ends with
  `syscall.Exec` (POSIX-only). Countdown goroutine uses
  `Session.WriteAsync`; interruptible via `RequestAbort`.
- **Admin audit rule**: privileged verbs (`spawn`, `teleport`, `goto`,
  `transfer`, `summon`, `wizinvis`, `shutdown`, `reboot`) record one
  `admin_audit` row per successful invocation via
  `internal/audit.Record(c.Ctx, audits, c.Session, verb, target,
  args)`. **Refusal paths MUST NOT audit** — the row represents
  "this side effect actually happened." Synchronous by design so
  `shutdown` rows commit before drain begins.
- **`characters` column lock-step**: new columns land in BOTH
  `charPlayerColumns` AND `charPlayerValues` AND `charPlayerScanDest`
  (`internal/repo/character_sql.go`); ordering is load-bearing.
  `auth_level` MUST stay the very last entry (bootstrap CASE
  consumes it as the trailing placeholder). JSON columns also need a
  `characterJSON` field plus marshal/unmarshal lines in
  `character_sqlite.go::marshalCharacterJSON` and
  `(characterJSON).unmarshalInto`.
- **Loader lock-step**: new columns on `rooms` / `items` / `exits`
  land in the repo's `*SelectCols` + `Create` INSERT + scan path AND
  in the loader's raw-SQL INSERT in `internal/world/loader.go`
  (`roomInsertValues` / `insertItems` / `insertExits`). Loader writes
  raw SQL inside one transaction rather than going through repo
  `Create`, so the column lists are duplicated.
- **Progression spend verb pattern** (`learn`, `feat`, `bump`,
  `learn weave`): per-verb repo method `RecordX` takes the absolute
  new pending value + per-pool upsert entry (mirrors `RecordCoin`/
  `RecordXP`, not `RecordLevelUp`-widening). Cmd-layer computes cap
  + budget guards before the call; refusals do NOT mutate or audit;
  success writes one `audit.Record(verb=X, target=<id>, args=<n>)`.
  Catalog string ids → int32 via `chargen.HashID(id)`.
- AuthLevel lives on the character row, not the account. Session
  stays at AuthGuest through login + account-create; stamped by
  `mode/postauth.promoteToGame` from `Character.AuthLevel` once a
  character is selected. `CharacterRepo.Create` atomically promotes
  the very first character to AuthAdmin (fresh-deploy bootstrap).
- **Items 3-location invariant**: exactly one of `room_id`,
  `owner_character_id`, or `parent_item_id` is set. Use
  `SetOwner` / `SetRoom` / `SetParent` or the `Transfer*` family —
  they flip atomically and clear the other two. `Transfer*` guards
  on prior location so concurrent `get`/`give`/`put` surfaces as
  `ErrItemMoved` instead of silent overwrite. `Character.Inventory`
  (`inventory_json`) is display ordering only — SQL
  `owner_character_id` is the source of truth;
  `inventory.go::orderInventory` self-heals. Items inside containers
  are NOT in `inventory_json`; encumbrance reads them via
  `ListAllOwnedTransitive` (BFS through `parent_item_id`).

## Tests

`go test -race ./...` covers the registry, mode dispatcher, completion
handler, IAC parser, color helpers, word wrap, tokenizer, line editor,
alias table, every repo (memory + sqlite, including coin_version
optimistic-lock contract + RecordLevelUp pending-pool accumulation +
RecordSkillRank atomic upsert), the world loader (zone metadata,
container/dark-room fixtures, shop/banker round-trip), the session
registry, eventbus, tick scheduler, persist manager, world Restocker,
`internal/combat`, `internal/group`, `internal/progression`,
`internal/config`, `internal/backup`, `internal/metrics`, and the
concrete commands. Telnet-package tests reuse `newPipeSession(t)` /
`bufSession(t)` / `bufConn` from `telnet/command_test.go`; cmd-package
tests reuse `commPair` / `runCmd` from `internal/cmd/comm_test.go`.

Fuzz targets in `telnet/iac_fuzz_test.go` + `telnet/tokenize_fuzz_test.go`
(`FuzzReadIAC`, `FuzzTokenize`, `FuzzSplitOnSemicolon`) run nightly via
`.github/workflows/fuzz.yml`. `test/integration/` (build-tag
`integration`) boots a real `cmd/server` binary against `./data/world`,
waits for `/healthz=200`, and exercises telnet handshake + IAC. Run:
`go test -tags=integration -timeout=120s ./test/integration/...`.

## Module

`github.com/Jasrags/WheelMUD`. Direct deps: `github.com/i582/cfmt`
(styling), `golang.org/x/crypto` (bcrypt), `gopkg.in/yaml.v3` (world +
config loader), `modernc.org/sqlite` (pure-Go SQLite),
`github.com/yuin/gopher-lua` (sandboxed Lua runtime for triggers /
dialogue scripts), `github.com/prometheus/client_golang` (metrics
endpoint). See `docs/CODEMAPS/dependencies.md` for the full picture.
