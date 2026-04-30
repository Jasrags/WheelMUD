# WheelMUD Roadmap

A living checklist of the components a modern MUD typically ships with, annotated
with what already exists in this repo. Status legend:

- `[x]` Done — landed on `main`
- `[~]` Partial — scaffolded or wired in a limited form, more work tracked below
- `[ ]` Not started

The "Notes" under each section call out concrete files when something exists, so
the doc stays anchored to code rather than drifting into wishlist territory.

---

## 1. Network & protocol

- [x] TCP listener on `:2323` (`LISTEN_ADDR` overridable)
- [x] Telnet IAC negotiation: `WILL/WONT/DO/DONT`, `SB ... SE`
- [x] Subnegotiation handlers for `TERM_TYPE` (RFC 1091) and `NAWS` (RFC 1073)
- [x] Per-connection goroutine + protocol parser
- [x] `IAC IAC` escape (literal 0xFF in stream) and standalone `IAC GA` / `IAC NOP` / etc.
- [ ] **MCCP2/3** (Mud Client Compression Protocol)
- [ ] **GMCP** (Generic MUD Communication Protocol) — out-of-band JSON messages
- [ ] **MSDP** (MUD Server Data Protocol)
- [ ] **MSSP** (MUD Server Status Protocol) — for crawlers / listings
- [ ] **MXP** (MUD eXtension Protocol) — clickable links, server-pushed UI
- [ ] **MNES / CHARSET** — UTF-8 negotiation
- [ ] TLS listener (telnet-over-TLS on a second port)
- [ ] WebSocket gateway (browser clients) sharing the session layer
- [ ] SSH listener (optional, for ops/admin shell)

Notes: parser lives in `telnet/server.go::bufferInput` and `RunSession`; option
constants and helpers in `telnet/iac.go`.

## 2. Terminal & rendering

- [x] ANSI SGR constants (`telnet/color.go`)
- [x] Color-level detection from `TERM` (`None`/`Basic`/`16`/`256`)
- [x] `cfmt`-based `{{text}}::style` rendering through `Session.WriteString`
- [x] NAWS-driven width/height tracking on `Session`
- [x] Truecolor (24-bit) path — `ColorLevelTrueColor`, `RenderRGBFG`/`RenderRGBBG`
      with 256-color and 16-color downsampling fallbacks (`telnet/color.go`)
- [x] ANSI-aware word-wrap respecting `Session.Width` (`telnet/wrap.go`,
      `Session.WriteWrapped`)
- [x] Output write lock — `Session.writeMu` serializes `WriteRaw`
- [ ] Pager mode for output that exceeds `Session.Height` (needs byte-level
      keypress dispatch in addition to today's line-mode reader)
- [ ] Prompt templating (HP/MP/room placeholders) — deferred until the
      character/world model lands so there are real values to interpolate
- [ ] Width-aware wrap & cursor accounting (CJK fullwidth, combining marks) —
      `WrapText` and `extendBuffer` count runes, not display cells
- [ ] Long-token break in `WrapText` — currently overflows tokens past `width`
      rather than splitting them

## 3. Input loop & line editing

- [x] Byte-at-a-time read loop with CR/LF line termination
- [x] Backspace / DEL handling
- [x] Tab autocomplete (verb-only) via `telnet/completion.go` + `Game.Complete`
- [x] Password-mode echo suppression — driven by Login / Create mode lifecycle
      (`Session.InPasswordMode` flipped on entering / leaving password substep)
- [ ] Argument-side tab completion (per-command `Completer`)
- [ ] Quoted-argument tokenization (`say "hello world"`)
- [ ] Command history with up/down arrows (`ESC [ A` / `ESC [ B`)
- [ ] In-line cursor movement (left/right arrows, Home/End, Ctrl-A/E/U/W)
- [ ] Aliases at the user level (distinct from registry aliases)

## 4. Command system

- [x] Command registry with aliases, prefix lookup, ambiguity detection (`telnet/command.go`)
- [x] Per-session dispatcher and `Mode` stack (`telnet/mode.go`, `telnet/session.go`)
- [x] `Game` mode wrapping the registry (`internal/mode/game.go`)
- [x] Sample commands: `quit`, `who`, `help`, `colors`
- [x] `AuthLevel` enforcement in `Registry.Dispatch` — denials render as
      `"Unknown command"` so privileged verbs can't be enumerated
- [ ] Per-command argument completer
- [ ] Command cooldowns / lag system (combat balance lever)
- [ ] Macro / multi-command lines (`;` separator)
- [ ] User-defined aliases stored on the character

## 5. Mode / state stack

- [x] `Mode` interface with `OnEnter`/`OnExit`/`Handle`/`Prompt`/`Complete`
- [x] `PushMode`, `ReplaceMode`, `PopMode` on `Session`
- [x] `PushMode` / `ReplaceMode` roll back on `OnEnter` error
- [x] `context.Context` plumbed through `Mode.Handle` — canceled when read
      loop exits so blocking I/O in handlers can observe teardown
- [x] Game mode (`internal/mode/game.go`)
- [x] Login mode (`internal/mode/login.go`) — see §6
- [x] Character-select + character-create modes (`internal/mode/character_*.go`)
- [x] Account-create mode (`internal/mode/create.go`)
- [ ] OLC editor modes (room editor, mob editor, etc.)
- [ ] Mail / note editor mode (multi-line input, `.` to end)
- [ ] Pager mode for long output (also tracked in §2)

## 6. Accounts, auth & characters

- [x] Account model — `repo.Account` + accounts table (`internal/repo/account.go`,
      `internal/db/migrations/0001_create_accounts.sql`)
- [x] Password hashing — `internal/auth.Hash` / `Verify` via `bcrypt`,
      enforcing 8-rune min / 72-byte max
- [x] Login mode — `internal/mode.Login` with username/password steps,
      lockout after 5 failures, mode-driven echo masking; replaces
      legacy `togglepassword`
- [x] Account-create mode — `internal/mode.Create` reachable via "new"
      from login; ASCII-only username policy, password confirmation
- [x] Character model — `repo.Character`, `0002_create_characters.sql`
      with FK + cascade, account isolation enforced in `CharacterSelect`
- [x] Character-select + character-create modes — auto-skip when account
      has 0 chars (forced create) or 1 char (auto-promote); 2+ shows menu
- [x] Mode-driven echo masking — Login / Create / CharacterCreate flip
      `Session.InPasswordMode` via lifecycle; `togglepassword` debug
      command retired
- [x] Multi-session detection — `internal/session.Registry` keyed by
      account ID; Login + Create bind on success and disconnect any
      prior occupant with a "logged in elsewhere" notice. Compare-and-
      delete unbind in `handleConnection` defer prevents stale teardown
      from clobbering a takeover.
- [~] Lockout on failed logins — per-account 5-failure / 15-min lock
      enforced; per-connection rate limit / exponential backoff still
      pending (`login_followups.md` items 4 & 6)
- [ ] Email verification / password reset (later)

## 7. Persistence

- [x] Backing store chosen: SQLite via pure-Go `modernc.org/sqlite` (no CGO)
- [x] Migration runner: embedded `internal/db/migrations/*.sql`, applied
      lexically, tracked in `schema_migrations` (`internal/db/db.go`)
- [x] First migration: `0001_create_accounts.sql` (accounts table + lockout index)
- [x] Account aggregate: `repo.AccountRepo` interface, `SQLiteAccountRepo`
      impl, `MemoryAccountRepo` fake; shared contract test exercises both
- [x] Character aggregate: `repo.CharacterRepo`, `0002_create_characters.sql`
      with FK + cascade, both impls + shared contract test
- [ ] World aggregates (rooms, exits, items, mobs)
- [ ] World data on disk (YAML/JSON area files) with a loader
- [ ] Hot-reload of area files without restart
- [ ] Periodic + shutdown autosave
- [ ] Backup rotation

## 8. Game loop & scheduling

- [ ] Tick / heartbeat goroutine (e.g. 1 Hz) with pluggable subscribers
- [ ] Pulse buckets (combat pulse, regen pulse, area reset pulse)
- [ ] Event bus / pub-sub for room and zone events
- [ ] Delayed / scheduled actions (`after 5s do X`)
- [ ] Graceful shutdown: drain dispatcher, save world, close listener

## 9. World model

- [ ] Room (id, name, description, exits, contents)
- [ ] Exit (direction, target, door state, key, flags)
- [ ] Area / zone (room set, reset rules, level range)
- [ ] Item / object (weight, value, wear flags, affects)
- [ ] Container semantics (capacity, nested containers)
- [ ] Mob / NPC (stats, spawn rules, dialogue hooks)
- [ ] Player character extending the mob model
- [ ] Equipment slots and wear/wield logic
- [ ] Currency model

## 10. Movement, look & navigation

- [ ] `look` (current room, item, mob, direction)
- [ ] Cardinal + vertical directions (`n/s/e/w/ne/nw/se/sw/u/d`) with abbreviations
- [ ] Doors, locks, keys
- [ ] `examine` for detailed inspection
- [ ] `map` / mini-map ASCII rendering
- [ ] Trails / `track` command

## 11. Combat

- [ ] Initiative / round structure
- [ ] Damage types and resistances
- [ ] Hit/miss/dodge/parry rolls
- [ ] Aggro / threat tables
- [ ] Death, corpses, looting, XP award
- [ ] PvE vs PvP rules and safe zones
- [ ] Group / party mechanics

## 12. Skills, spells & progression

- [ ] Class / archetype model
- [ ] Skill tree with practice / training
- [ ] Spell list with mana costs and reagents
- [ ] Levels & XP curve
- [ ] Affects / buffs / debuffs with durations
- [ ] Cooldowns and global lag

## 13. Communication

- [ ] `say` (room-scoped)
- [ ] `tell` / `whisper` (private)
- [ ] `shout` / `yell` (zone-wide)
- [ ] Channels (`ooc`, `gossip`, `newbie`) with on/off toggles
- [~] `who` — currently shows only the caller's character name; full
      multi-session listing waits on iterating `session.Registry` (§6)
- [ ] Ignore list / mute
- [ ] Mail between characters
- [ ] Bulletin boards / notes

## 14. Inventory & economy

- [ ] `inventory`, `get`, `drop`, `give`, `put`, `take`
- [ ] `wear`, `wield`, `remove`, slot validation
- [ ] Shops (buy, sell, list, value)
- [ ] Banks / vaults
- [ ] Crafting recipes (later)

## 15. Quests, scripts & NPC behavior

- [ ] Quest engine (state machine per character per quest)
- [ ] NPC dialogue trees
- [ ] Trigger / event system (`on_enter`, `on_say`, `on_attack`)
- [ ] Embedded scripting language (Lua via `gopher-lua`, or starlark/risor) for
      builders to write behavior without recompiling
- [ ] Sandboxing & resource caps for scripts

## 16. Online creation (OLC)

- [ ] `redit` / `oedit` / `medit` / `zedit` mode-based editors
- [ ] Permission gating (admin / builder roles)
- [ ] Versioned area saves with diff/preview before commit

## 17. Admin & moderation

- [ ] `goto`, `transfer`, `summon`, `wizinvis`, `snoop`
- [ ] `shutdown` / `reboot` (copyover-style hot-restart eventually)
- [ ] `ban` / `siteban` / `kick` / `mute`
- [ ] Audit log of admin actions
- [ ] Wizlist / staff hierarchy

## 18. Help & docs

- [x] In-game `help` command listing registered commands
- [ ] Topic-based help files (lore, mechanics, command detail)
- [ ] `help <topic>` resolution with prefix matching
- [ ] `news` / MOTD on login
- [ ] In-repo developer docs beyond `CLAUDE.md`

## 19. Logging, telemetry, ops

- [x] `slog` structured logging at `LevelDebug`
- [ ] Log levels controllable via env / config
- [ ] Request/command audit log per character
- [ ] Metrics endpoint (Prometheus): connections, dispatch latency, tick lag
- [ ] Crash recovery: panic handler per goroutine that doesn't take down the listener
- [ ] Profiling endpoints (`net/http/pprof` on a private port)

## 20. Configuration

- [x] `LISTEN_ADDR` env var
- [x] `DB_DSN` env var (default `wheelmud.db`; `:memory:` supported)
- [ ] Config file (TOML/YAML) for ports, paths, feature flags
- [ ] Per-environment overrides (dev/stage/prod)
- [ ] Secrets via env, never committed

## 21. Testing & CI

- [x] Unit + contract tests across `telnet`, `internal/auth`,
      `internal/db`, `internal/mode`, `internal/repo`, `internal/session`
- [x] Dependabot config (`.github/dependabot.yml`)
- [ ] GitHub Actions: `go vet`, `go test -race -cover`, `staticcheck`, `gosec`
- [ ] Integration test that drives the telnet protocol against a real listener
- [ ] Fuzz tests on the IAC parser and command tokenizer
- [ ] 80% coverage target tracked in CI

## 22. Packaging & deploy

- [x] `Dockerfile` + `docker-compose.yml`
- [x] `Makefile` targets (`build/server`, `run/server`, `run/live/server`)
- [x] Hot reload via `air` for dev
- [ ] Versioned releases / `goreleaser`
- [ ] Systemd unit / deploy doc
- [ ] Healthcheck endpoint or telnet-level liveness probe

---

## Active follow-ups

Items already tracked elsewhere — included here so the roadmap points at them:

- **Command-input deferred work** (quoted args, argument completion) —
  `command_input_followups.md`.
- **Open code-review items** (ambiguous-error wording, empty-candidate
  guard, server ctx parent, timing side channel, AuthLevel encapsulation,
  cctx naming, cancellation test coverage) — `code_review_open_items.md`.
- **Terminal & rendering deferred work** (pager mode, prompt templating,
  wide-glyph wrap, long-token break, `SGR` perf, color-enum naming) —
  `terminal_rendering_followups.md`.
- **Persistence layer follow-ups** (down-migrations, brittle unique-violation
  match, DSN leak, CHECK constraints, memory repo O(n), clock seam, sqlite
  `:memory:` pool trap, NFKC normalization, `UsernameLower` /
  `NameLower` encapsulation, FK pragma reliance) —
  `persistence_followups.md`.
- **Auth (password hashing) follow-ups** (bcrypt cost tuning, lockout-
  before-Verify ordering — `SetCost` mutability now resolved) —
  `auth_followups.md`.
- **Login / account-create follow-ups** (control-char scrub on welcome
  echo, lockout enumeration leak, hash-in-memory hygiene, per-connection
  rate limits, account-create soft DoS, `CharacterSelect.chars` snapshot
  staleness, double Conn.Close) — `login_followups.md`.
