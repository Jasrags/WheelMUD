# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

WheelMUD is a Wheel of Time MUD server in Go (1.25). It listens on TCP `:2323`,
performs telnet option negotiation, and runs a per-connection line-based
command loop driven by a registry/mode-stack dispatcher. SQLite (modernc
pure-Go driver) backs accounts, characters, rooms, exits, items, mobs, and
chat channels. A YAML world loader (`internal/world/`) seeds the DB from
`data/world/` on first boot.

## Source-of-truth map

- **`ROADMAP.md`** — what's done vs. pending. Authoritative for *status*.
- **`docs/PLAN.md`** — sequenced phases (A–J). Authoritative for *order*.
  When the two disagree about status, ROADMAP wins; about order, PLAN wins.
- **`docs/CODEMAPS/`** — token-lean architecture maps (`architecture.md`,
  `commands.md`, `data.md`, `dependencies.md`, `telnet.md`). Read these
  before diving into a package.
- **`docs/CONVENTIONS.md`** — code-level recipes and invariants (write
  paths, GMCP wiring, items 3-location, characters column lock-step,
  loader lock-step, admin audit, progression spend pattern, migrations,
  Lua sandbox, world loader rules). Consult before editing any of those
  surfaces.
- **`data/world/README.md`** — zone.yaml schema.

## Common commands

```bash
make build/server      # go build -o /tmp/bin/server cmd/server/main.go
make run/server        # build then run the binary
make run/live/server   # hot reload via cosmtrek/air
go test -race ./...    # full test suite with race detector
docker compose up      # build + run, exposes :2323
make mudlet-package    # build dist/mudlet/wheelmud.{mpackage,profile}
```

Connect with: `telnet localhost 2323`.

## Configuration

Pass `-config <path>` for YAML (see `config.example.yaml`) or rely on env
vars only — both work; env overrides file values. Env surface: `LISTEN_ADDR`
(default `:2323`), `METRICS_ADDR` (default `127.0.0.1:9090`; empty disables
metrics+pprof+healthz), `DB_DSN` (default `wheelmud.db`, `:memory:` works),
`BACKUP_DIR` (empty disables snapshots), `LOG_LEVEL`, `WORLD_DIR` (default
`./data/world`), `AUDIT_COMMANDS_ENABLED` + `AUDIT_COMMANDS_EXCLUDE`.
Catalog dirs (`CHARGEN_DIR` / `QUEST_DIR` / `SCRIPT_DIR` / `EFFECTS_DIR`
/ `HELP_DIR`) are env-only and switch each embedded-FS catalog to an
on-disk override.

## Architecture (one-liner map)

Detailed structure lives in `docs/CODEMAPS/architecture.md`. Quick index:

- **`cmd/server/main.go`** — entrypoint: config → DB+migrations → repos →
  catalogs → `world.LoadAndSync` → command registry on a `server` struct
  → `tick.Scheduler` + `tick.Buckets` + `persist.Manager` → optional
  `backup` + `metrics` → TCP accept loop. New long-lived deps belong on
  the `server` struct. `buildVersion`/`buildCommit`/`buildDate` come
  from goreleaser ldflags. main.go is intentionally a thin orchestrator;
  per-concern helpers live in sibling files (`registry.go`,
  `lua_bindings.go`, `subscribers_combat.go`, `tickers.go`,
  `bootstrap_observability.go`, `shutdown_admin.go`, `mssp.go`,
  `adapters.go`, `audit_metrics.go`, `catalog_validate.go`) — see
  `docs/CODEMAPS/architecture.md` for the index.
- **`telnet/`** — protocol primitives + per-connection driver. See
  `docs/CODEMAPS/telnet.md` and `docs/CONVENTIONS.md` (write paths,
  session field ownership, option negotiation, GMCP).
- **`internal/cmd/`** — concrete commands wired in `main.go::buildRegistry`.
  See `docs/CODEMAPS/commands.md` for the verb list.
- **`internal/mode/`** — login, character_select, character_create, game,
  postauth promotion (`promoteToGame` stamps Character* + AuthLevel +
  channel-mute, then `ReplaceMode`s into game).
- **`internal/repo/`** — typed repos with sqlite + memory implementations
  and a shared test suite. Write-through on mutation; `persist.Manager`
  Save bucket layers periodic + shutdown flushes.
- **`internal/db/migrations/`** — embedded forward-only migrations.
  See `docs/CONVENTIONS.md` for key invariants.
- **`internal/lua/`, `internal/scripts/`** — gopher-lua sandbox + runner
  and embedded script catalog. See `docs/CONVENTIONS.md` for sandbox
  rules.
- **`internal/quest/`** — event-driven quest engine. Subscribes to
  `combat.CombatDeath` (kill_n), `world.PlayerEntered` (reach_room);
  talk_to advances via dialogue `advance_quest`; `script` steps wait
  on Lua `quest.advance(id)`. State on `characters.quest_log_json` via
  `RecordQuestProgress`. Final-step XP via `RecordXP`, coin via
  `RecordCoin` (one optimistic-lock retry on `ErrCoinConflict`). Dialogue
  effects wired via `mode.DialogueHooks` from `main.go` so `internal/cmd`
  and `internal/dialogue` stay free of `internal/quest` imports.
- **`internal/progression/`** — pure-function d20 XP curve + level-up
  math (`MaxLevel=20`). `ComputeLevelUp → LevelGains` → cmd-layer hands
  to `RecordLevelUp` via `LevelUpFields`. No DB, no session.
- **`internal/channeling/`** — per-tick driver for channeler state.
  `SessionTicker` subscribed to `tick.Buckets.Regen` (30s).
- **`internal/group/`** — in-memory party manager (`MaxGroupSize = 6`).
  Wired into combat via `combat.GroupResolver`. Not persisted.
- **`internal/world/`** — YAML zone loader, `Restocker`, `ZoneResetter`,
  `Clock`. See `docs/CONVENTIONS.md` for loader rules and
  `data/world/README.md` for schema.
- **`internal/chargen/`** — YAML catalog (backgrounds, classes, feats,
  skills, weaves) under `internal/chargen/default/*.yaml` (or
  `CHARGEN_DIR`). Cross-refs validated at Load.
- **`internal/emote/`** — YAML social-verb catalog
  (`internal/emote/default/socials.yaml` or `EMOTE_DIR`). Per-social
  commands are emitted by `cmd.NewSocials` and registered alongside
  the freeform `cmd.NewEmote` (alias `:`); both honour the §M.2
  visibility filter. §M.6 added `Catalog.Replace` for hot-reload
  (mu-guarded). See `docs/CONVENTIONS.md` (Socials + Hot-reload
  sections).
- **`internal/cmd/reload.go`** — §M.6 admin verb backing
  `reload socials` / `reload help`. Uses `telnet.Registry.Unregister`
  (added §M.6) + `Register` to swap per-social verbs without a
  restart; help reload re-runs `MergeGenerated`. Audits per success.
- **`internal/flow/`** — Phase O.0 engine core. Generic multi-step
  Flow runner with typed `Step` interface (`TextStep` / `ChoiceStep`
  / `ConfirmStep`), Go-only `ActionRegistry` + `ValidatorRegistry`,
  pluggable `Renderer` (no telnet dep — adapter lives at the edge).
  Validators return `*ValidationError` for re-prompt; other errors
  abort the flow. State is plain-data for the O.2 persistence layer
  to JSON-marshal later. No live consumer yet (O.1 adds mode
  integration + YAML loader + a `wizdemo` test verb).
- **`internal/session/`** — process-level registry enforcing single-
  session-per-account. `Bind` displaces the prior session; `Unbind` is
  compare-and-delete.
- **`internal/eventbus/`, `internal/tick/`, `internal/persist/`,
  `internal/safego/`** — typed pub/sub, scheduler + named buckets,
  periodic+shutdown autosave, `safego.Go("name", fn)` for panic-safe
  long-lived goroutines.
- **`internal/config/`** — YAML+env loader, precedence "struct defaults
  → YAML → env". Empty path = env-only.
- **`internal/metrics/`** — Prometheus + pprof + healthz on a fresh
  registry. `SetReady` atomic drives healthz (200 once ready AND DB
  ping passes within 500ms). Loopback default keeps pprof off the wire.
  `mode.Game` takes a `CommandMetricFn` hook so `internal/mode` doesn't
  import metrics.
- **`internal/backup/`** — wall-clock `VACUUM INTO` + retention pruning
  (refuses symlinks via `os.Lstat`). Decoupled from `tick.Buckets`.
- **`internal/auth/`** — bcrypt password hashing (tunable cost).
- **`internal/creature/`** — `Core` stat block shared by characters and
  mob_templates, plus the `Channeling` weave model.
- **`clients/mudlet/`** — drop-in package consuming V1 GMCP frames. See
  `docs/CONVENTIONS.md` (GMCP section) for the field-name contract.

## Cross-cutting rules

Detailed recipes are in `docs/CONVENTIONS.md`. The bullets below are the
ones easy to break without knowing they exist:

- **Writes**: `WriteRaw` is the only safe write path. Cross-session
  output uses `WriteAsync`; the dispatcher's own reply to `c.Session`
  uses `WriteString`. See `docs/CONVENTIONS.md` (Write paths).
- **Goroutines**: spawn long-lived ones via `safego.Go("name", fn)`.
- **Mode handlers** run synchronously on the dispatcher and must
  observe `ctx` — a slow handler stalls input for that session.
- **Auth + Lag + segments**: `Registry.Dispatch` enforces `Command.Auth`
  (privilege-denied returns the same `Unknown command` text — no
  enumeration), gates per-segment on `Command.Lag` via `StampLag`, and
  splits top-level `;` via `SplitOnSemicolon` (cap 16 segments, alias
  depth 3).
- **Admin audit**: privileged verbs record one `admin_audit` row per
  *successful* invocation. Refusal paths MUST NOT audit.
- **Items**: 3-location invariant — use `SetOwner`/`SetRoom`/`SetParent`
  or `Transfer*`. Concurrent moves surface as `ErrItemMoved`.
- **Schema lock-step**: new `characters` columns need three matching
  list entries; new `rooms`/`items`/`exits` columns need both repo
  and loader (raw SQL) updates. Full recipe in `docs/CONVENTIONS.md`.

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
`github.com/yuin/gopher-lua` (sandboxed Lua), `github.com/prometheus/
client_golang` (metrics). See `docs/CODEMAPS/dependencies.md` for the
full picture.
