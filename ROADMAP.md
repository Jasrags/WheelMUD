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
- [ ] **MCCP2/3** (Mud Client Compression Protocol) — negotiate option 86
      (MCCP2) on connect; on `IAC SB MCCP2 IAC SE` start a `zlib.Writer`
      around `Session.WriteRaw` and flush per write. MCCP3 (option 87)
      adds upstream compression — lower priority. Track compressed-byte
      counters for telemetry.
- [ ] **GMCP** (Generic MUD Communication Protocol) — out-of-band JSON
      messages over option 201. Define a `gmcp.Message{Package, Body}`
      type, route inbound packages (`Core.Hello`, `Char.Login`,
      `Room.Info`, `Comm.Channel.Text`) to a registry of handlers,
      and a publish helper for server-pushed updates (room desc,
      vitals, channels). Per-session opt-in tracked on `Session`.
- [ ] **MSDP** (MUD Server Data Protocol) — option 69. Simpler key/value
      protocol than GMCP; useful for clients that don't speak JSON.
      Share the publish layer with GMCP behind a common `OOB` interface
      so handlers report variables once.
- [ ] **MSSP** (MUD Server Status Protocol) — option 70. Respond to
      `IAC DO MSSP` with name/uptime/players/codebase/contact crawled
      from a `mssp.Vars()` snapshot built from `session.Registry` +
      build info. No persistent state — pure read.
- [ ] **MXP** (MUD eXtension Protocol) — clickable links, server-pushed
      UI. Negotiate option 91, wrap output in `<send>`/`<a>` tags via
      a `MXPWriter` shim that no-ops when not negotiated. Most value
      comes from making exits and `who` entries clickable.
- [ ] **MNES / CHARSET** — UTF-8 negotiation. Send `IAC DO CHARSET`,
      offer `UTF-8`, fall back to ASCII. Surface as `Session.Charset`
      and have `WrapText` switch from rune-count to display-cell
      counting when UTF-8 is negotiated (ties into §2 wide-glyph wrap).
- [ ] TLS listener (telnet-over-TLS on a second port) — second
      `net.Listener` from `tls.Listen` on `:2992`, wrap conn before
      handing to `RunSession`. Cert path via `TLS_CERT`/`TLS_KEY` env;
      auto-reload on SIGHUP for cert rotation.
- [ ] WebSocket gateway (browser clients) sharing the session layer —
      `gorilla/websocket` upgrader behind an HTTP handler, adapt to a
      `net.Conn` shim so `RunSession` is reused unchanged. Frame each
      newline-terminated chunk as one text message; binary frames
      reserved for GMCP.
- [ ] SSH listener (optional, for ops/admin shell) — `gliderlabs/ssh`
      with a public-key allowlist from `ssh_admins` table. Sessions
      open with `AuthLevel=Admin` and a special `Mode` skipping the
      login prompt; intended for emergency console only.

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
- [ ] Pager mode for output that exceeds `Session.Height` — push a
      `Pager` mode that buffers the full payload, renders one
      screenful, and waits for `space`/`enter`/`q` keystrokes.
      Requires the byte-level keypress dispatch (already partially
      built for line-edit), plus a `WriteParaged` helper that
      checks `len(lines) > Session.Height-1` before pushing.
- [x] Prompt templating (HP/MP/room placeholders) — `internal/prompt`
      renders `%h/%H` (HP), `%r` (room), `%g` (gold), `%%` literal,
      with `%m/%M`/`%v/%V`/`%t` reserved for mana, move, and combat
      target once those systems land. `Game.Prompt` invokes it once
      per dispatch with `FindByName`-fetched character + optional
      `RoomRepo.FindByID` (only when `%r` is in the template). Server
      default: `"<%h/%H hp> "`. cfmt color tags (`{{...}}::red`) are
      rendered before write; interpolated `%r`/`%g` values are
      defanged to prevent style injection. Per-character override via
      the `prompt set/clear/show` command (migration 0023 added the
      `characters.prompt_template` column).
- [ ] Width-aware wrap & cursor accounting (CJK fullwidth, combining
      marks) — `WrapText` and `extendBuffer` count runes, not display
      cells. Adopt `golang.org/x/text/width` (or vendor a small east-
      asian-width table) and a `displayWidth(rune)` helper used by
      both wrap and the line editor's cursor math.
- [ ] Long-token break in `WrapText` — currently overflows tokens past
      `width`. Add a `breakToken(token, width)` that splits on grapheme
      boundaries (or just runes for ASCII) when a single token would
      exceed the line, inserting a soft hyphen-free wrap point.

## 3. Input loop & line editing

- [x] Byte-at-a-time read loop with CR/LF line termination
- [x] Backspace / DEL handling
- [x] Tab autocomplete (verb-only) via `telnet/completion.go` + `Game.Complete`
- [x] Password-mode echo suppression — driven by Login / Create mode lifecycle
      (`Session.InPasswordMode` flipped on entering / leaving password substep)
- [x] Argument-side tab completion via `Command.Completer`; `Game.Complete`
      delegates to the matched command once whitespace appears, with
      AuthLevel filtering so privileged verbs can't be enumerated
- [x] Quoted-argument tokenization (`telnet/tokenize.go`) — `Tokenize`
      replaces `strings.Fields` in `Registry.Dispatch`; supports double,
      single, and bare-backslash escapes; unbalanced quotes surface as
      "Unbalanced quote"
- [x] Command history with up/down arrows — `History` ring on `Session`
      (`telnet/history.go`), CSI parser (`telnet/csi.go`) recognizes
      ESC[A/B and SS3 ESC O A/B; password-mode lines never enter history
- [x] In-line cursor movement — `LineEdit` model (`telnet/lineedit.go`)
      handles ←/→, Home/End (ESC[H/F, ESC[1~, ESC[4~), forward-delete
      (ESC[3~), Ctrl-A/E/U/W/K, mid-line insert/backspace; password mode
      bells on every motion key
- [x] Aliases at the user level — `AliasTable` on `Session`,
      `alias`/`unalias` commands, single-pass expansion in
      `Registry.Dispatch` so chained aliases don't recurse

## 4. Command system

- [x] Command registry with aliases, prefix lookup, ambiguity detection (`telnet/command.go`)
- [x] Per-session dispatcher and `Mode` stack (`telnet/mode.go`, `telnet/session.go`)
- [x] `Game` mode wrapping the registry (`internal/mode/game.go`)
- [x] Sample commands: `quit`, `who`, `help`, `colors`
- [x] `AuthLevel` enforcement in `Registry.Dispatch` — denials render as
      `"Unknown command"` so privileged verbs can't be enumerated
- [x] Persisted AuthLevel on the **character** row (migrations 0018/0019;
      0018 introduced it on accounts, 0019 moved it to characters so a
      single account can own admin and player characters side-by-side).
      `CharacterRepo.Create` atomically promotes the very first
      character on the server to AuthAdmin; `mode/postauth.promoteToGame`
      stamps `s.AuthLevel` from the chosen character.
- [x] Per-command argument completer — `Command.Completer` field;
      `help` ships a real one (sample: tab on `help <prefix>`)
- [x] Broaden argument completers across the command catalog —
      `internal/cmd/door.go` ships `completeExits` (visible exits
      from the actor's room, hidden filtered) on `open`/`close`/
      `lock`/`unlock`/`pick`; `teleport` unions room external IDs +
      online peer names on slot 0 and room IDs only on slot 1;
      `get` completes floor item keywords, `drop` completes
      inventory keywords, `give` does inventory keyword on slot 0
      then online name on slot 1; `examine` unions room mobs +
      room items + inventory items; `tell` completes online names
      with self/higher-auth peers filtered out. Ordinal `2.sword`
      syntax flows through `splitOrdinalPartial` so the typed
      ordinal is preserved on each candidate's replacement text.
      Combat-verb completers and 3-arg "get sword from chest" forms
      will land alongside their respective features. `reply` takes
      a free-form message and bells. Channel verbs continue to
      self-name (no completer needed).
- [ ] Command cooldowns / lag system (combat balance lever) — add
      `Command.Lag time.Duration` and a `Session.NextReady time.Time`
      gate checked in `Registry.Dispatch`; commands dispatched while
      lagged queue (bounded, drop on overflow with a "you're too busy"
      message). Combat skills set lag on success; movement uses the
      sector cost from §9.
- [ ] Macro / multi-command lines (`;` separator) — split the raw
      input on unquoted `;` in `Tokenize`, dispatch each segment in
      order, abort the chain on the first error or when the session's
      mode changes. Cap chain length (e.g. 10) to defang `kill;kill;…`
      spam.
- [ ] User-defined aliases stored on the character — promote the
      in-memory `AliasTable` from §3 to a `character_aliases` table
      (`character_id`, `name`, `expansion`, unique on `(character_id,
      name)`), load on character promote, persist on `alias`/`unalias`,
      cap per-character count (e.g. 64) and expansion length.

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
- [ ] OLC editor modes (room editor, mob editor, etc.) — one mode
      per aggregate (`RoomEdit`, `MobEdit`, `ItemEdit`, `ZoneEdit`)
      sharing a small DSL: `name <text>`, `desc` (drops into the
      multi-line editor below), `flag <name>`, `set <field> <value>`,
      `show`, `done` to commit, `abort` to discard. Edits buffer in
      a working copy and only hit the repo on `done`.
- [ ] Mail / note editor mode — multi-line input mode with `.` on its
      own line to end, `~q` to abort, `~h` for help. Backed by a
      `[]string` line buffer on the mode; submits to the parent on
      `OnExit` via a `Done(text string)` callback. Used by mail,
      bulletin boards, and OLC `desc`.
- [ ] Pager mode for long output (also tracked in §2) — lives in
      `internal/mode/pager.go`, pushed by helpers in §2.

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
- [ ] Email verification / password reset — optional `email` column
      on accounts, `email_verifications` table holding single-use
      tokens with TTL, `verify <token>` command. Reset flow: `forgot
      <email>` from login mode, mail a token, `reset <token>
      <newpass>` swaps the hash. Mail dispatch behind a small
      `mail.Sender` interface (SMTP impl + log-only fake).

## 7. Persistence

- [x] Backing store chosen: SQLite via pure-Go `modernc.org/sqlite` (no CGO)
- [x] Migration runner: embedded `internal/db/migrations/*.sql`, applied
      lexically, tracked in `schema_migrations` (`internal/db/db.go`)
- [x] First migration: `0001_create_accounts.sql` (accounts table + lockout index)
- [x] Account aggregate: `repo.AccountRepo` interface, `SQLiteAccountRepo`
      impl, `MemoryAccountRepo` fake; shared contract test exercises both
- [x] Character aggregate: `repo.CharacterRepo`, `0002_create_characters.sql`
      with FK + cascade, both impls + shared contract test
- [x] World aggregates (rooms, exits, items, mobs) — `repo.Room`/`Exit`/`Item`/`Mob`
      with SQLite + memory repos (`internal/repo/{room,exit,item,mob}_*.go`),
      `0003_create_world.sql` schema, `0004_seed_starter_zone.sql` 3-room demo
      zone (Plaza ↔ North/South Roads), and `0005_add_character_room.sql`
      adding `current_room_id` to characters. `Session.CurrentRoomID` is
      populated in `promoteToGame` and persisted across reconnects via
      `CharacterRepo.RecordRoom`. `look`, `n/s/e/w/u/d` commands wired into
      Game mode (`internal/cmd/look.go`, `internal/cmd/move.go`).
      The legacy flat `mobs` table and `repo.Mob` impls were
      retired in `0010_drop_legacy_mobs.sql`; the world loader
      now manufactures one `mob_template` per YAML mob entry and
      spawns a single `mob_instance` from it (`internal/world/
      loader.go::insertMobs`), and `look`/`move`/`teleport` read
      from `MobInstanceRepo`. A richer YAML schema for builders
      (separate `mob_templates.yaml` + spawn references with
      counts) is still pending; today every YAML mob is its own
      one-of-a-kind template.
- [x] World data on disk (YAML/JSON area files) with a loader —
      `internal/world` package: `gopkg.in/yaml.v3` parsing, strict
      cross-reference validation (unique ids, exactly-one starter, valid
      directions, all refs resolve), one-shot transactional sync into
      the SQLite world tables. Default world embedded via
      `//go:embed all:default` at `internal/world/default/`;
      `WORLD_DIR` env var overrides with a real filesystem path.
      Loader is no-op when world tables already have rows
      (`0006_world_external_id.sql` wipes the SQL seed and adds
      `external_id` columns to rooms/items/mobs so reloads pick up
      changes via DB wipe). Starter room pinned to id=1 to preserve
      `repo.StarterRoomID`.
- [ ] Hot-reload of area files without restart — `reload world` admin
      command re-runs the loader against `WORLD_DIR`, diffs against
      current rooms/exits/items/mobs by `external_id`, applies adds
      and updates in a transaction, soft-deletes rows whose ids
      vanished (mark `deleted_at`, skip in queries). Players in a
      deleted room get teleported to the starter room with an
      "the world shifts around you" message. `fsnotify` watcher is
      a follow-up so manual reload is the v1 path.
- [~] Periodic + shutdown autosave — `internal/persist.Manager`
      hosts named `SaverFunc` registrations; the new `tick.Buckets
      .Save` bucket (default 30s, `DefaultSaveInterval`) calls
      `FlushAll` on every pulse, and `srv.shutdown()` runs a final
      `FlushAll` under a 5s context budget after the session drain
      finishes. One concrete saver landed: `savePlayTimes`
      iterates `session.Registry.Snapshot()` and stamps
      `last_played_at` on every authenticated character — so a
      crash within the 30s window now loses at most 30s of
      tracked play-time instead of the entire session.
      Pending: dirty-bit aggregate types for combat HP / mob-
      instance state / affect tick counters once §11 / §12 land.
      Most other state (room / item / character core / mob room
      moves) is already write-through, so the dirty-bit pattern
      only matters for state that ticks faster than we want to
      round-trip per mutation.
- [ ] Backup rotation — nightly `VACUUM INTO` of the SQLite DB into
      `backups/wheelmud-YYYYMMDD.db`; keep 7 daily / 4 weekly /
      12 monthly. Triggered from the scheduler so no external cron
      dep. Optional gzip of older snapshots.

## 8. Game loop & scheduling

- [x] Tick / heartbeat goroutine — `internal/tick.Scheduler` runs a
      single 1 Hz goroutine, dispatches due `Subscribe(every, fn)` /
      `After(d, fn)` handlers fire-and-forget, recovers panics, warns
      on >50 ms fan-out. Clock seam (`WithClock`) for deterministic
      tests. Owned by `server` in `cmd/server/main.go`, started after
      DB open and stopped after the session drain.
- [x] Pulse buckets — `internal/tick.Bucket` wraps a single scheduler
      subscription and fans out to its own subscriber list;
      `tick.NewBuckets(s)` registers `combat` / `regen` / `areaReset`
      with default cadences (4 s / 30 s / 5 min). No game logic yet —
      just the plumbing for §11 / §12 to attach to.
- [x] Event bus / pub-sub for room and zone events —
      `internal/eventbus.Bus` with generic `Subscribe[T]` /
      `SubscribeAsync[T]`, sync dispatch by default, single-worker
      async path, panic-recovery per handler. `world.PlayerEntered`
      / `PlayerLeft` defined in `internal/world/events.go` and
      published from `cmd/move.go::moveDir`. Owned by `server` and
      drained alongside scheduler in `srv.shutdown()`.
- [x] Delayed / scheduled actions (`after 5s do X`) —
      `Scheduler.After` for unscoped one-shots; `tick.AfterCtx`
      auto-cancels via `context.Context` so callers can pass a
      session or room ctx and have the timer die with its scope.
      The wrapped handler also re-checks `ctx.Err()` before firing
      so a race between dispatch and cancel doesn't leak through.
- [x] Graceful shutdown — `signal.NotifyContext(SIGINT, SIGTERM)` in
      `main.go` closes the listener, accept loop exits on
      `net.ErrClosed`, `srv.shutdown()` drains in-flight sessions via
      `WaitGroup` with a 10 s bound, then stops buckets + scheduler.
      World save still pending (depends on §7 dirty-tracking).

## 9. World model

The §7 persistence layer already lands rough `Room`/`Exit`/`Item`/`Mob`
aggregates as flat rows so `look` / movement work end-to-end. This section
tracks the richer domain model the gameplay layers (combat, quests, economy)
will need on top of those tables.

- [~] **Room** — `repo.Room` covers id, zone, name, description,
      `external_id`, `RoomFlags` (`indoors` / `nopvp` / `noteleport`
      / `dark` / `silent` / `peaceful`), `Sector` enum (city, forest,
      field, hills, mountain, desert, water, underwater, air,
      underground), `LightLevel`, `coord_x/y/z` (migration 0012),
      and `ExtraDescs` keyword map (migration 0013, JSON column).
      YAML schema accepts `flags:`, `sector:`, `light_level:`,
      `coords:`, and `descriptions:` blocks; defaults are outdoors
      / city / fully lit / origin / no extras. Behavior wired:
      `look` (no args) renders the room and pitch-blacks dark+0
      light rooms; `look <noun>` resolves against `ExtraDescs`
      (case-insensitive) and falls back to "nothing special" on
      miss; `tp` refuses `noteleport` rooms; `say` is swallowed in
      `silent` rooms; `move` consults the mover's
      `creature.Speed` to gate `air`/`underwater` (FlyFt/SwimFt
      requirements; zero-speed defaults to "blocked"); `whereami`
      command surfaces id/sector/flags/light/coords/keywords for
      builders. Day/night cycling shipped via `internal/world.Clock`
      (migration 0024 + `world_state` table, 30 real-min per game
      day, 4 phases of 450 ticks each). `EffectiveLight(room)` ramps
      outdoor rooms dawn → noon → dusk → midnight; indoor /
      underground / underwater rooms ride the static baseline; the
      `Dark` flag is an explicit "always pitch black" override.
      `look` and `whereami` consult the clock; persistence runs on
      the existing Save bucket. The `time` admin command (gated at
      AuthAdmin) reports tick count, phase, phase progress, and the
      countdown to the next phase boundary so builders can sanity-
      check the cycle without scraping `whereami`. Phase-change ambient
      broadcasts ship sector defaults: a 1-second tick poll
      (`world.PhaseAmbientWatcher`) detects dawn/sunrise/dusk/nightfall
      crossings and writes a single sector-appropriate sentence to every
      session in an outdoor, non-Silent, non-Dark, non-Indoors room
      (Underground/Underwater filtered too); seed-on-construct prevents
      a spurious fire on boot, and non-adjacent phase jumps (future
      `time set` rebases) advance lastPhase silently. Pending: per-room
      reset hooks (block on §9 zone schema); `coords` consumed by
      `map`/`track` rendering (block on §10); player-carried light
      sources (block on item taxonomy + §11 combat); deferred ambient
      extensions tracked in `phase_ambients_open_questions.md` memory
      (empty-sector default policy alignment, optional `RoomFlags.
      Windowed` for shuttered indoor rooms, per-room ambient overrides
      `rooms.ambient_phase_json` for hand-built set-piece rooms, and
      zone-`climate` variants for sector-by-climate text tuples once
      overrides exist).
- [~] **Exit** — `repo.Exit` covers `from_room_id`, `direction`,
      `to_room_id`, `ExitFlags` (`Closed` / `Locked` / `Pickable` /
      `Hidden` / `NoPass`), `KeyExternalID`, `LockDifficulty`, and
      `Description` (migration 0014). YAML accepts both shorthand
      (`north: room.id`) and object form (`north: {to: ..., closed,
      locked, key, difficulty, description, ...}`); `Pickable`
      defaults to true. Behavior wired: `look` skips Hidden exits
      and annotates the rest with `(closed)` / `(locked)` in dim
      gray; `move` refuses Hidden as "you can't go that way",
      Closed as "the door is closed", and NoPass with an
      unseen-force message. One-way exits already supported via
      single-direction authoring. Pending: `open` / `close` /
      `lock` / `unlock` / `pick` commands (§16) to mutate Closed
      and Locked at runtime — needs an `ExitRepo.UpdateFlags`
      method and key/skill resolution against §14 inventory + §12
      skill checks. Diagonal directions (ne/nw/se/sw) shipped via
      migration 0007.
- [~] **Area / zone** — `zones` table landed (migration 0016): id,
      external_id, name, builder, min/max_level, reset_interval_s,
      reset_mode (`always`/`empty`/`never` enforced via CHECK),
      climate, ambient_json. `rooms.zone_id` joins rooms to their
      owning zone (soft FK; default 0 keeps test fixtures buildable
      without a backing zones row). `repo.ZoneRepo` (sqlite + memory
      + shared test suite) exposes Create / GetByID /
      GetByExternalID / List; ZoneResetMode is a typed enum mirrored
      across both impls so the SQLite CHECK is a backstop, not the
      front line. The YAML loader inserts a zones row per `zone.yaml`
      and stamps `rooms.zone_id` during insertRooms; `zone.yaml`
      accepts `builder`, `level_range`, `reset_interval_s`,
      `reset_mode`, `climate`, `ambient` (all optional, defaults
      applied at insert). `validateZones` enforces external-id
      uniqueness, valid reset_mode, level-range bounds, and
      non-negative reset interval with builder-friendly file:line
      errors. `zones` admin command (AuthAdmin) lists zones and
      shows per-zone metadata + room count via
      `RoomRepo.CountByZone`. Pending: `zone_resets` sibling table
      (load mob X into room Y up to count Z; load item into
      room/container; set door state; equip mob) wired into the §8
      `areaReset` bucket; admin edit (`zedit`) lands with §16;
      promote `rooms.zone_id` to a hard FK via table rebuild once
      §16 admin room-create reliably supplies a zone id.
- [~] **Item / object** — taxonomy schema landed (migration 0015).
      `repo.Item` carries `Type` enum (`weapon` / `armor` / `shield` /
      `container` / `consumable` / `light` / `key` / `tool` /
      `clothing` / `food` / `trade_good` / `trash`), `Weight` (lb),
      `Value` (`currency.Amount` in cp), `Quality` (`normal` /
      `masterwork` / `masterpiece` / `power_wrought`), `Flags` bitset
      (`notake` / `nodrop` / `nosell` / `bind_on_pickup` / `magic` /
      `glow` / `hum` / `trade_good`), and a polymorphic
      `stats_json` blob decoded into typed structs:
      `WeaponStats` (proficiency / size / range / damage dice /
      threat range / crit mult / range increment / B-P-S types /
      special tags), `ArmorStats` (weight class / bonus / max-dex /
      check penalty / speed), `ShieldStats` (kind / bonus / check
      penalty), `ContainerStats` (lb + cuft + liquid pints + depth
      cap + weight multiplier), `ConsumableStats` (charges +
      effect-id forward ref), `LightStats` (radius + fuel ticks),
      `KeyStats` (key id), `ToolStats` (skill tag + charges). YAML
      schema accepts `type:`, `weight:`, `value: "15mk"` (parsed
      via `currency.Parse`), `quality:`, `flags: [...]`, and a
      type-discriminated `stats:` sub-block; unknown types and
      flags fail at validate time. `examine` surfaces type / quality
      / weight / formatted value when set. `lock` / `unlock` prefer
      typed `KeyStats.KeyID` matches over the legacy ExternalID
      fallback. Pending: combat damage resolution against
      `WeaponStats` (§11), wear/wield slot management (§9 equipment
      slots bullet), encumbrance / carry-weight (§14), don timers,
      masterwork/masterpiece attack/AC effects, Reputation deltas,
      container nesting + `take` / `put` / `open container` (§14),
      light radius affecting room visibility (§10), Power-wrought
      unbreakable handling.
- [ ] **Container semantics** — capacity (weight + slot count),
      open/closed/locked state sharing the door schema, content
      visibility flag (`see-inside`), nested-container depth cap,
      take/put permission flags, weight-reduction multiplier (bag of
      holding), liquid containers as a separate subtype with capacity
      in sips and a liquid id.
- [~] **Mob / NPC** — `repo.Mob` has id, room/zone, name,
      description, `external_id`; `mob_templates` / `mob_instances`
      tables and `creature.MobTemplate` / `MobInstance` types now
      exist (see "Player character extending the mob model" below)
      but no repo or loader integration yet. Pending: stat block (level, HP/MP,
      str/dex/con/int/wis/cha, AC, hit/dam dice, attack type,
      damage type), behavior flags (`aggressive`, `wimpy`,
      `sentinel`, `scavenger`, `assist-same-race`, `helper`),
      faction / race / class, default loadout (eq + inventory),
      gold range, XP award, corpse decay timer, spawn rules
      (zone reset references), dialogue/script hooks (§15
      triggers), shopkeeper subtype (inventory list, buy/sell
      multipliers, hours), quest-giver flag.
- [~] **Player character extending the mob model** — type skeleton
      and schema landed (`internal/creature/creature.go` defines
      `Core`, `Abilities`/`AbilityScore`, `Saves`, `Speed`,
      condition/position/quality bitsets, `Affect`/`StatMod`,
      damage types + DR/resists, `Equipment` slots, `Channeling`
      sub-record, `MobTemplate`, `MobInstance`, `Character`).
      Migrations `0008_create_creatures.sql` (mob_templates,
      mob_instances, polymorphic channeling) and
      `0009_extend_characters.sql` (Core + player columns) applied.
      `MobTemplateRepo`, `MobInstanceRepo`, and `ChannelingRepo`
      (interfaces + SQLite + memory + contract tests) landed in
      `internal/repo/{mob_template,mob_instance,channeling}_*.go`;
      `ChannelingRepo` keys off a polymorphic `(OwnerKind, owner_id)`
      so the same row schema serves PCs, mob templates, and mob
      instances.
      World loader migrated to spawn from templates
      (`0010_drop_legacy_mobs.sql` removes the legacy flat table);
      `look`/`move`/`teleport` read from `MobInstanceRepo`.
      `repo.CharacterRepo` extended to load/persist the full Core
      stat block + player columns from migration 0009 (race,
      background, class levels, XP, coin/bank, stance, fame/
      infamy, JSON catalogs for feats / skills / equipment /
      inventory / quests / dialogue) plus a `RecordCore` method
      for combat HP/condition updates; round-trip test covers
      every column class.
      Pending: char-create rolling abilities
      / picking race / class / background / starting HP & defense,
      richer YAML schema for builders (template/spawn split with
      counts), `examine` rendering full mob detail, and seed catalogs
      for feats / skills / talents / weaves / classes /
      backgrounds (stubbed as `int32` ids today). Original spec:
      share a `creatures` core: `name`, `size` (Fine→Colossal), `type`
      (Humanoid / Animal / Exotic / Shadowspawn), `gender`,
      `alignment_posture` (Good/Bad/Evil narrative tag); abilities
      `Str/Dex/Con/Int/Wis/Cha` as `current/max/inherent` triples
      (drain vs damage vs ter'angreal); `hp_current/max` plus a
      separate `subdual_damage` pool; Hit Dice; **`defense`** (replaces
      AC — class+Dex+size+armor+shield+dodge); `fort/ref/will` saves;
      `init_modifier`; `base_speed_ft` (+ optional climb / fly with
      maneuverability / swim); `bab` (drives multi-attack at +6/+11/
      +16); reach/face/threat range; conditions set (§12); position
      flags (prone, flat-footed, flanked, grappling targets); damage
      reduction; resistances; special qualities (Blindsight,
      Low-Light Vision, Scent). Split into `mob_template` (immutable
      archetype — `challenge_code` A–I, `organization`, climate/
      terrain, advancement rules, behavior flags, natural attacks,
      `special_attacks`, traits, `loot_table_id`, `gold_dice`,
      `dialogue_tree_id`, `trigger_scripts[]`, `shopkeeper_config`,
      `corpse_decay_ticks`, `respawn_zone_reset_id`, Shadowspawn-
      specific `shadow_link_to_myrddraal_id` + `taint_immune` +
      fade-on-link-master timer) / `mob_instance` (in-world state) /
      `character` (persisted player). Player adds: account fk,
      `class_levels: map[Class]int` over the seven WoT classes
      (multiclass = sum), `race` (Human/Ogier), `background` (Aiel /
      Atha'an Miere / Borderlander / Cairhienin / Domani / Ebou Dari
      / Illianer / Midlander / Taraboner / Tairen / Tar Valoner —
      supplies starting gear, languages, height-mod), `feats[]`
      (general/special/channeling/lost-ability), `skills:
      map[SkillID]ranks` (class-skill cap = level+3, cross-class
      = ½), `practice_points`, `class_features[]` (Uncanny Dodge,
      Dance the Spears, Sneak Attack, etc.), appearance (height/
      weight/age/handedness), **reputation** with `infamy_share`
      (≥½ vicious gains → Infamous; gates fame/infamy feats),
      `followers[]` (unlocked lvl 10, capped by Reputation),
      `coin: Amount` (existing currency), encumbrance load +
      fatigue/exhaustion timers, condition (standing/sitting/
      sleeping/fighting), idle timer, `bound_room_id` (respawn),
      `bank_balance`, `played_seconds`, `last_login`, `quest_log`,
      per-NPC dialogue state. Channelers (PC or NPC) attach a
      sub-record — see new bullet below.
- [ ] **Channeling (One Power) sub-record** — attached to any
      channeler (PC, Aes Sedai, Wise Ones, Forsaken, damane,
      Asha'man). Fields: `gender_source` (`Saidin` male / `Saidar`
      female — same mechanics, asymmetric perception);
      `channeler_type` (`Initiate` Int+Wis-keyed / `Wilder`
      Cha+Wis-keyed); `affinities: Set[Power]` ⊆ {Air, Earth, Fire,
      Water, Spirit}; `talents: Set[TalentID]` (Healing, Traveling,
      Warding, Cloud Dancing, Earth Singing, Elementalism, Illusion,
      Conjunction, …); `weaves_known: []WeaveID` (each tagged
      Common / Rare / Lost); `slots_per_level: map[int]{cur,max}`
      (slot-based casting, **not mana** — levels 0–9). Casting
      thresholds: Initiate `Int ≥ 10+level` & `Wis ≥ 10`; Wilder
      `Cha ≥ 10+level` & `Wis ≥ 10+level`; Wilders cap at level-2
      outside their Talents, Initiates cap at level-0. State:
      `embraced` (full-round to enter; blocks rest/heal/sleep;
      addictive), `madness: int` (men only — accrues while embraced;
      Mental Stability feat slows; `Heal the Mind` weave reduces),
      `stilled_state` (recoverable via the Lost `Restore the Power`
      weave), `bonded_warder_id` / `bonded_to_aes_sedai_id` (via the
      Conjunction Talent), `held_angreal_id` / `held_saangreal_id`
      (adds power 1–10 to slot levels; cross-gender devices appear
      inert), `circle_id` (linking — leader / members / required-men
      ratio per Table 9-1), `aes_sedai_oaths` 3-oath bitmask + an
      `ageless` cosmetic flag, `damane_collar_to: NPCID` (a'dam
      binding). Data layout landed (`creature.Channeling` + the
      polymorphic `channeling` table from `0008_create_creatures
      .sql`); behavior pending: embrace lifecycle (full-round
      enter, blocks rest/heal/sleep, voluntary release), per-tick
      madness accrual for men with Mental Stability slow + `Heal
      the Mind` reduction, slot consumption / 8h refresh from the
      §8 `regen` bucket, circle linking math (Table 9-1 leader/
      member/required-men ratios, pooled slot draw), a'dam bind/
      unbind enforcement (collar-side commands, suppression while
      collared), Warder bond effects, angreal/sa'angreal slot
      boost with cross-gender inert behavior.
- [ ] **Equipment slots and wear/wield logic** — WoT does not use a
      D&D wear-slot bitmask. Discrete slots: `armor` (one body
      armor), `shield` (separate from armor; bonuses stack),
      `primary_wield` + `off_hand` (two-handed weapons consume both;
      double weapons like quarterstaff/ashandarei = one item with
      two attack profiles), `outfit` (Artisan's / Cadin'sor /
      Courtier's / Explorer's / Gleeman's / Noble's / Peasant's /
      Royal / Scholar's / Traveler's / Cold-weather — first free at
      creation), `cloak` (Warder fancloth canonical), `backpack`
      (primary container), `belt_pouches[]` (containers),
      `held_in_hand` (transient — torch/lantern; competes with
      weapon use), `mount` (separate aggregate with barding/saddle/
      saddlebags), `worn_misc[]` (signet ring, Aiel buckler-strap —
      no hard cap, narrative). Wear-flag validation against item
      type, swap semantics, save/restore on login, affects
      reapplied on equip and stripped on remove, container-on-belt
      vs in-inventory distinction.
- [~] **Currency model** — `internal/currency` package: four
      denominations (`cp`/`sp`/`mk`/`gc`) at fixed ratios
      `1/10/100/1000`, all wealth stored as a signed `Amount` of
      base copper pennies. `New(gc, mk, sp, cp)`, `Parse("1gc 2mk
      3sp 4cp")`, `Format` (greedy largest-first), `Short` (largest
      non-zero), `In(coin)`, `Add`/`Sub` with overflow + insufficient-
      funds guards. Pending: a `coin` column on characters (carried
      wealth), a separate `bank_balance` for §14 vaults, gold ranges
      on mob loadouts, and `give <amt>` / shop wiring once §14
      lands.

## 10. Movement, look & navigation

- [x] `look` (current room, items, mobs, exits) — `internal/cmd/look.go`
      via `RenderRoom`; reused by every successful move
- [x] Cardinal + vertical + diagonal directions — `n/s/e/w/u/d` plus
      `ne/nw/se/sw` with short-code aliases via `NewMoveFamily`
      (`internal/cmd/move.go`). Migration `0007_widen_exit_directions.sql`
      rebuilds the `exits` table with the widened CHECK constraint
      (SQLite can't ALTER CHECK in place); `validDirections` in the
      world loader (`internal/world/validate.go`) and the look
      renderer's `directionLongName` map (`internal/cmd/look.go`)
      both accept the diagonals.
- [~] Doors, locks, keys — `open` / `close` / `lock` / `unlock` /
      `pick` shipped (`internal/cmd/door.go`) on top of
      `ExitRepo.UpdateFlags`. Direction parsing accepts long names
      (`open north`) and short codes (`open n`); Hidden exits stay
      invisible to door verbs. `open` refuses Locked, `close` refuses
      NoPass. Both broadcast to the actor's room and (when a reverse
      exit exists) the far room with the inverted direction. `lock` /
      `unlock` enforce a key match against `Exit.KeyExternalID` —
      keys must be in the actor's inventory (§14 retired the
      room-floor placeholder); `AuthAdmin` always bypasses the key
      check. `unlock` leaves the door closed-but-unlocked so the lock
      state stays observable. `pick` requires
      `Pickable=true` and is gated to `AuthAdmin` until §12 skill
      checks land — players see the "you lack the skill" flavor that
      will become a failed roll. Pending: §14 inventory swap (keyed
      item lookup against the player's bag instead of the room),
      §12 lockpicking skill check + difficulty roll against
      `Exit.LockDifficulty`, and door reset on zone reset (§9 zone
      schema).
- [~] `examine` for detailed inspection — `examine <target>` shipped
      (`internal/cmd/examine.go`). Resolves against room mobs first
      (by Core.Name token-prefix / substring), then room items (by
      Item.Name). Mob view renders name, a coarse HP descriptor
      ("perfect health" / "wounded" / "barely standing"), condition
      bitset decoded to labels, and named Affects. Item view renders
      name + ShortDesc. Pending: inventory + equipment lookup
      (waits on §14 inventory tables / repos), container contents
      (waits on §9 container semantics), keyword-disambiguation
      (`2.guard` syntax — also a §14 follow-up), and richer item
      condition/durability once items grow a stat block.
- [x] `map` / mini-map ASCII rendering — `internal/cmd/map.go`. BFS
      from the current room (default depth 3, `map <n>` clamped to
      1..5). Lays cells onto a 2D grid via per-direction unit vectors
      in `dirVec` (north decreases y, east increases x; diagonals are
      integer ±1 steps; vertical exits decorate the source cell).
      Renders `[*]` current, `[ ]` visited, `[?]` for `nomap`
      destinations + depth boundary, `[^]`/`[v]`/`[%]` for cells with
      up/down/both. Connectors (`-`, `|`, `\`, `/`) are drawn from
      actual exits, never grid adjacency, so two visited rooms that
      lack an exit between them stay disconnected. Hidden exits are
      skipped (mirrors look). NoMap rooms render `[?]` and the BFS
      does not recurse through them. First-visit-wins for cycles and
      grid conflicts. Migration `0020_room_nomap.sql` adds the flag
      column; `repo.RoomFlags.NoMap` and `world.RoomFlags.NoMap` mirror
      it. Not yet shipped: dynamic depth from `Session.Width`, door
      state in connectors, depth-boundary `[?]` hint when a visited
      cell has unrecursed exits.
- [~] Trails / `track` command — `mob_trails` ring buffer keyed by
      `(mob_id, room_id, ts)` updated on every move; `track <name>`
      finds the freshest trail entry within the last N minutes and
      reports the first-step direction. Fades with time; skill
      check determines the max staleness the tracker can read.
      Storage shipped (migration 0021, `MobInstanceRepo.UpdateRoom`
      records the row + caps at `MobTrailCap`); the wander tick
      (`internal/mob/wander.go`, `tick.Buckets.Wander` at 20 s,
      per-template `WanderChance` × global multiplier, non-Sentinel
      mobs only, in-zone walkable exits) drives organic movement
      so trails accumulate in normal play. Per-template tuning via
      migration 0022 (`mob_templates.wander_chance`, default 0.25)
      and `wander_chance` on `mobs.yaml`. Admin `track <name>` verb
      shipped (`internal/cmd/track.go`, AuthAdmin) — keyword-resolves
      across all spawned mobs, reports current room + last-step
      direction + elapsed time. Pending: §12 skill-check gate on
      staleness window so players (not just admins) can `track`,
      and per-template `wander_radius` once `mob_instances` carries
      a stable spawn-room id.

## 11. Combat

- [ ] Initiative / round structure — combat ticks off the `combat`
      bucket from §8 (default 4 s). On `attack <target>` push both
      participants into a `Fight` aggregate keyed by room; each pulse
      resolves one round per combatant in initiative order
      (`d20 + init_modifier`, ties broken by Dex then random).
      Players queue one combat-mode action (`flee`, `kick`,
      `weave <name>`) per round. Multi-attack at BAB +6/+11/+16
      grants extra iterative attacks at −5 each.
- [ ] Damage types and resistances — WoT damage kinds: physical
      `slash/pierce/bludgeon` (each weapon entry tags one), plus
      One-Power / energy types from weave effects (`fire/cold/
      lightning/air/earth/spirit`), `subdual` (separate pool, see
      §9), and `taint` (Shadow corruption, bypasses most resists).
      Mobs and items carry `[]Resist{Type, Pct}` and `damage_reduction`
      (flat `DR x/—` or `DR x/<bypass>` keyword); `applyDamage(target,
      dmg, type)` applies DR first, then `1 - resist + vuln`.
      Negative resist = vulnerability. Surface in `examine`.
- [ ] Hit/miss/dodge/parry rolls — attacker rolls `d20 + bab +
      ability_mod + size_mod` vs defender **`defense`** (§9 — class
      bonus + Dex + size + armor + shield + dodge). On hit, optional
      parry check if wielding a weapon and not flat-footed. Crit on
      natural 20 confirmed by a second roll vs defense (doubles
      dice, weapon-specific threat range/multiplier); fumble on
      natural 1 drops the weapon or grants AoO. All rolls go through
      a single `combat.Roll` seam for deterministic tests.
- [ ] Aggro / threat tables — `Fight.Threat map[CreatureID]int`,
      damage adds threat 1:1, healing adds threat to the healer
      from every hostile in the room scaled by 0.5. NPCs retarget
      to highest-threat each round. Taunts add a flat bonus;
      `feign death` zeroes threat from one source.
- [ ] Death, corpses, looting, XP award — at HP ≤ 0: drop a
      `corpse_of_<name>` container item containing all worn/held
      items + carried gold, decay timer (5 min for player, 10 min
      for mob), award XP to all attackers weighted by damage dealt,
      mob respawns via the §9 zone reset, player respawns at
      bound/temple room with a death penalty (XP debt or stat
      drain — pick one and document).
- [ ] PvE vs PvP rules and safe zones — `pvp` flag on character
      (opt-in) plus `nopvp` room flag (always safe). Attack between
      two non-PvP players blocked at the verb level; one-side opt-in
      still blocked. Newbie level cap (e.g. <10) immune.
- [ ] Group / party mechanics — `group <name>` invites, `follow
      <name>` / `unfollow`, leader's moves auto-pull followers
      (subject to AC/exit checks). XP split among in-room group
      members within ±5 levels with a small bonus for grouping.
      `assist <name>` joins their current target; `consider <name>`
      hints at relative power.

## 12. Skills, spells & progression

- [ ] Class / archetype model — table-driven `classes` (id, name,
      hit_die, bab_progression, save_progression `{fort,ref,will}`,
      class_skills, skill_points_per_level, weapon_armor_proficiency,
      class_features by level). The seven WoT classes (Algai'd'siswai,
      Armsman, Initiate, Noble, Wanderer, Wilder, Woodsman). Multi-
      class supported via `class_levels: map[Class]int` summed for
      `character_level`. Race (Human / Ogier) separately gates stat
      ranges, height/weight, and innate abilities. Background
      (eleven from §9) supplies starting gear, languages, height-mod.
      All selected during character-create after the ability-score
      roll/buy step.
- [ ] Skill tree with ranks / training — `character_skills`
      (`character_id`, `skill_id`, `ranks`, `is_class_skill`). Class-
      skill cap = `character_level + 3`; cross-class cap = ½ that.
      Skill points per level from class table (Int mod adds, ×4 at
      1st level). Skill checks roll `d20 + ranks + ability_mod +
      misc`. Caps and skill list live in `skills` table seed.
- [ ] Weave (One Power) list with slot levels — replaces the
      generic spell/mana model. `weaves` table (id, name, level
      0–9, school/affinity (Air/Earth/Fire/Water/Spirit — multiple),
      talent_required, rarity (`Common`/`Rare`/`Lost`), cast_time_
      ticks, target_type, effect script ref). `weave <name>
      [target]` validates: caster `embraced` (or full-round to
      embrace), gender vs source, ability threshold (Initiate
      Int/Wis or Wilder Cha/Wis ≥ 10+level), affinity covered (or
      angreal compensates), free slot at that level, talent gating
      for Talent-only weaves, line-of-sight. Locks caster for
      `cast_time` ticks (interrupt on damage forces a Concentration
      check), consumes a slot of that level (or higher), accrues
      `madness` for men, then resolves via the effect script (§15).
      No mana pool — slots refresh after 8h rest.
- [ ] Levels & XP curve — d20 geometric XP table
      (`xp(n) = 1000 × n × (n-1) / 2` style; cap level 20 v1, raise
      later). Level-up grants: roll new HD for HP, +1 BAB by class
      progression, save bumps, skill points, possibly a feat (every
      3 levels) or ability increase (every 4), new class features
      and (for channelers) new weave slots. Stored on character;
      `train` command at a trainer NPC commits the level-up.
- [ ] Affects / buffs / debuffs with durations — `creature_affects`
      list `(source_id, name, modifiers []StatMod, duration_ticks,
      tick_effect)`. Must support the WoT condition enum from §9:
      `AbilityDamaged, AbilityDrained, Blinded, Checked, Cowering,
      Dazed, Deafened, Disabled, Dying, Entangled, Exhausted,
      Fatigued, FlatFooted, Frightened, Grappled, Held, Helpless,
      Panicked, Paralyzed, Pinned, Prone, Shaken, Stable, Staggered,
      Stunned, Unconscious`. Plus environmental flags driven by
      surroundings rather than affects (`Flanked, Charging,
      TotalDefense, FightingDefensively, Concealed%, Cover`). The
      `combat`/`regen` buckets decrement durations; `tick_effect`
      fires per-tick (DoT, regen, fear check, madness gain while
      embraced). Stacking rules: same `(source, name)` refreshes;
      different sources stack up to a per-affect cap.
- [ ] Cooldowns and global lag — per-skill `cooldown_until` on the
      character + the §4 `Command.Lag` global lag. Display in
      `cooldowns` command grouped by skill family.

## 13. Communication

- [x] `say` (room-scoped) — `internal/cmd/comm.go::NewSay`. Emits
      `You say, "<text>"` to the caller and `<Name> says, "<text>"`
      to every other session whose CurrentRoomID matches the
      speaker's room. `sanitizeChat` strips control bytes, caps at
      1024 runes, and defangs cfmt template syntax (`{{ }} ::`)
      so a player can't inject styling. NPC `on_say` trigger
      hookup lands with §15.
- [~] `tell` / `whisper` (private) — `tell` shipped
      (`internal/cmd/comm.go::NewTell`); resolves the recipient via
      `session.Registry.FindByCharacterName`, sets the recipient's
      `Session.LastTellFrom`, and renders both sides. `reply <text>`
      writes back to LastTellFrom (`NewReply`). Pending: `whisper`
      (room-local with bystanders), ignore-list filtering, and the
      `nochannels` flag — all blocked on §6 / §13 ignore plumbing.
- [ ] `shout` / `yell` (zone-wide) — broadcast to every session whose
      character is in the same `zone_id`. Higher cost (small move
      drain) to discourage spam. Suppressed in `silent` rooms.
- [~] Channels (`ooc`, `gossip`, `newbie`) with on/off toggles —
      catalog table + repo
      (`internal/repo/channel{,_sqlite,_memory,_test}.go`,
      migration `0011_create_channels.sql`) seeded with `ooc` /
      `gossip` / `newbie`. `internal/cmd/channel.go::NewChannel`
      registers one verb per row at startup; no-arg toggles the
      caller's mute bit and write-throughs via
      `CharacterRepo.RecordChannelSettings`, args broadcasts
      `[<NAME>] <speaker>: <text>` to every other authenticated
      session whose mute bit is off. `channels` overview command
      lists each channel with the caller's on/off state. Per-
      character mute lives in `characters.channel_settings_json`
      (sparse — keys present only when muted) and is mirrored onto
      `Session.channelMuted` (crossMu-guarded) at game promotion.
      Pending: `min_level` enforcement / `newbie` auto-leave at
      level 10 (blocked on §12 level computation), §8 event-bus
      dispatch for admin snoop, and `auction` / `clan` channels
      (clan needs clan_id from §17).
- [~] `who` — `internal/cmd/who.go::NewWho` iterates
      `session.Registry.Snapshot()`, renders each bound session
      with its character name, "(you)" marker, and idle time
      (≥30s) computed from `Session.LastInputAt` (stamped by the
      dispatcher on every command). Sessions still in login /
      character-select render as `(connecting)` so `who` can't be
      used to enumerate IPs. Pending: class / level / title columns
      (blocked on char-create populating those fields), AFK flag,
      and sort filters (`who level` / `who class <name>`).
- [ ] Ignore list / mute — per-character `ignored_names []string`
      capped at 50; tells/whispers/channel messages from ignored
      names are silently dropped on the receiver side. Admin
      override exists so staff can always reach a player.
- [ ] Mail between characters — `mail` table (`id`, `to_id`,
      `from_id`, `subject`, `body`, `sent_at`, `read_at`). Compose
      via the §5 multi-line editor. Commands: `mail`, `mail read
      <n>`, `mail send <name>`, `mail delete <n>`. Storage cap per
      character (e.g. 50 messages); login MOTD shows unread count.
- [ ] Bulletin boards / notes — `boards` table (id, name,
      min_read_level, min_post_level), `notes` table (board_id,
      author_id, subject, body, posted_at, expires_at). Boards
      live in specific rooms ("the town crier"); `note list/read/
      post/remove` while standing there. Auto-prune expired notes
      from a daily scheduler tick.

## 14. Inventory & economy

- [~] `inventory`, `get`, `drop`, `give`, `put`, `take` — `inventory`
      / `get` / `drop` / `give` shipped (`internal/cmd/inventory.go`)
      on top of migration `0017_item_owner.sql` (adds nullable
      `items.owner_character_id`, soft FK matching the `rooms.zone_id`
      pattern from §16). Items now carry an `OwnerCharacterID` field
      alongside `RoomID`; `ItemRepo.SetOwner` / `SetRoom` flip both
      atomically so the location invariant (exactly one non-null) holds.
      `inventory` lists held items in `Character.Inventory` JSON order
      (display ordering; SQL `owner_character_id` is the truth),
      renders carry weight + load band (Str-based d20 carrying-capacity
      table in `internal/cmd/encumbrance.go`), and the coin purse via
      `currency.Format`. `get` blocks `FlagNoTake`, blocks at the
      heavy-cap (Overloaded) load band, and broadcasts to room peers.
      `drop` blocks `FlagNoDrop`. `give <item> <name>` requires the
      target online + same room and gates on the recipient's encumbrance.
      `give <amount> <name>` parses via `currency.Parse` and transfers
      between purses with `ErrInsufficientFunds` surfaced cleanly.
      Keyword resolution accepts the ordinal `<n>.<keyword>` form
      (`get 2.sword`) via `internal/cmd/keyword.go::MatchItem` /
      `MatchMob`, used by both `examine` and the inventory verbs.
      `examine` now walks room mobs → room items → inventory items.
      `door` `lock` / `unlock` (§10) read keys from inventory rather
      than the room-floor placeholder. New CharacterRepo write-throughs
      `RecordInventory` / `RecordCoin` mirror the `RecordCore` pattern
      (single targeted UPDATE per call). Pending: `put` / `take from`
      container verbs (block on §9 container nesting), keyword
      disambiguation in non-inventory commands (move/teleport),
      coin weight in the carry calculation.
- [ ] `wear`, `wield`, `remove`, slot validation — `equipment`
      table keyed by `(character_id, slot)`. `wear <item>` picks
      first slot the item permits; `wear <item> <slot>` forces.
      Two-handed weapons consume `wield` + `hold`. Apply affects
      on equip, strip on remove, persist on logout.
- [ ] Shops (buy, sell, list, value) — shopkeeper mob subtype with
      `shop` config (`buy_types []ItemType`, `sell_markup`,
      `buy_markdown`, `open_hour`/`close_hour`, `inventory []ItemID`
      with restock interval). `list` shows wares + price; `value
      <item>` previews sell price; refuses items the shop can't
      buy. Stock restocks via the `areaReset` bucket.
- [ ] Banks / vaults — bank-NPC subtype: `balance`, `deposit
      <n>`, `withdraw <n>`, `transfer <name> <n>`. Stored on
      character row. Optional vault for items with per-tier slot
      limits — separate `bank_items` table.
- [ ] Crafting recipes (later) — `recipes` table (id, name,
      skill_id, min_skill, ingredients []{ItemID, qty},
      result_item_id, result_qty, station_required). `craft
      <recipe>` consumes ingredients on success, partial on
      failure based on skill check.

## 15. Quests, scripts & NPC behavior

- [ ] Quest engine (state machine per character per quest) —
      `quests` table (id, name, steps JSON), `character_quests`
      (`character_id`, `quest_id`, `step_index`, `state JSON`,
      `completed_at`). Step types: `talk_to`, `kill_n`, `fetch`,
      `deliver`, `reach_room`, `script`. Quest log command shows
      active + completed; rewards (xp/gold/item) granted on the
      final step's transition.
- [ ] NPC dialogue trees — JSON dialogue per mob: nodes with
      `prompt`, `responses []{Match []string, Reply, Next, Effect}`.
      `talk <mob>` enters a `Dialogue` mode that takes free-text
      keyword input or numbered choices. Effects can advance a
      quest step or push another mode (e.g. shop).
- [ ] Trigger / event system (`on_enter`, `on_say`, `on_attack`) —
      a `triggers` table attached to mobs/rooms/items keyed by
      event name. The §8 event bus dispatches into a trigger
      runner that resolves the registered script. Event payloads
      defined alongside (e.g. `world.PlayerSaid{Speaker, Text}`).
- [ ] Embedded scripting language — pick `gopher-lua` (mature,
      familiar to MUD builders) over starlark/risor v1.
      Surface a small API: `room`, `mob`, `player`, `say(text)`,
      `give(item)`, `wait(ticks, fn)`, `quest.advance(id)`. Each
      script runs inside a fresh `lua.LState` cached per mob/room.
- [ ] Sandboxing & resource caps for scripts — disable `os`/`io`
      libs, set `lua.LState.SetMx` instructions cap (e.g. 100k),
      wrap each invocation with a `context.WithTimeout` (50 ms),
      and a per-tick budget so one runaway trigger can't starve
      the bucket. Failures log + auto-disable the trigger after
      N consecutive faults.

## 16. Online creation (OLC)

- [ ] `redit` / `oedit` / `medit` / `zedit` mode-based editors —
      one editor mode per aggregate (see §5). `redit` works on the
      current room by default or `redit <id>`; sub-commands
      `name/desc/exit/flag/sector/show/done`. Edits buffer until
      `done`, which writes to the SQLite world tables and re-syncs
      the in-memory cache.
- [ ] Permission gating (admin / builder roles) — `AuthLevel` enum
      `Player < Builder < Admin < Implementor`. OLC commands gated
      at `Builder`; admin-only verbs at `Admin`. Builders may be
      restricted to specific zones via a `builder_zones` table.
- [ ] Versioned area saves with diff/preview before commit —
      `area_revisions` table keeps prior YAML serialization with
      `author_id`, `committed_at`, `message`. `diff` in editor shows
      changes vs HEAD; `commit "<msg>"` snapshots; `revert <rev>`
      restores. Optional git push of the YAML export to a builder
      repo as a follow-up.

## 17. Admin & moderation

- [ ] `goto`, `transfer`, `summon`, `wizinvis`, `snoop` — `goto
      <room|mob|player>` teleports the admin; `transfer <player>
      [room]` pulls a player to admin's room or named room; `summon
      <player>` is the polite variant that prompts the target.
      `wizinvis [level]` hides from `who`/room listings below
      level. `snoop <player>` mirrors that session's I/O to the
      admin via a fan-out on `Session.WriteRaw`; logged + audited.
- [ ] `shutdown` / `reboot` — `shutdown <seconds> [reason]`
      broadcasts countdown messages on a scheduler timer, flushes
      autosave, then exits. `reboot` is a copyover: serialize FD
      table + minimal session state, `exec` the new binary, restore
      sessions in-process so connections aren't dropped. v1 ships
      `shutdown`; copyover is a stretch goal.
- [ ] `ban` / `siteban` / `kick` / `mute` — `bans` table (`pattern`,
      `kind` in `account|ip|cidr`, `reason`, `expires_at`,
      `created_by`). Login mode + accept loop both consult; CIDR
      via `net.ParseCIDR`. `kick <player> [reason]` closes the
      socket with a notice. `mute <player> <duration>` flips a
      flag blocking channel + say emits.
- [ ] Audit log of admin actions — `admin_audit` table (`actor_id`,
      `verb`, `target`, `args`, `at`). Wrap admin commands at
      dispatch with a logger middleware; queryable via `audit
      <name|verb> [since]`.
- [ ] Wizlist / staff hierarchy — `wizlist` command renders staff
      grouped by `AuthLevel` from accounts.auth_level. Title field
      ("Builder of the Plains") shown next to name.

## 18. Help & docs

- [x] In-game `help` command listing registered commands
- [ ] Topic-based help files (lore, mechanics, command detail) —
      `help/*.md` embedded via `//go:embed`; front-matter sets
      `keywords`, `min_level`, `category`. Indexed at startup.
      Commands auto-register a help stub from `Command.Help` so
      `help <verb>` always resolves.
- [ ] `help <topic>` resolution with prefix matching — exact match
      → keyword match → unique prefix match → ambiguity list, same
      ladder used by the command registry.
- [ ] `news` / MOTD on login — `motd.md` embedded; `news` shows a
      paged list of dated entries from `news/*.md`. Tracks per-
      character `last_news_seen`; login banner notes unread count.
- [ ] In-repo developer docs beyond `CLAUDE.md` — `docs/`
      directory: `architecture.md` (subsystem map), `building.md`
      (running OLC + YAML loader), `protocol.md` (telnet/IAC
      gotchas + GMCP/MSDP packages), `contributing.md`. Linked
      from README.

## 19. Logging, telemetry, ops

- [x] `slog` structured logging at `LevelDebug`
- [~] Log levels controllable via env / config — `LOG_LEVEL` env
      (`debug|info|warn|error`) feeding `slog.HandlerOptions.Level`
      via `cmd/server/main.go::parseLogLevel`; unknown values
      default to `info` with a warning. Pending: a `loglevel
      <name>` admin command that swaps the level at runtime via
      an `slog.LevelVar`.
- [ ] Request/command audit log per character — append-only
      `command_log` table (`character_id`, `verb`, `args`, `at`,
      `latency_us`, `outcome`) gated by a config flag (off in dev).
      Keep N days, then prune. Useful for griefer post-mortems.
- [ ] Metrics endpoint (Prometheus) — separate `:9090` HTTP listener
      exposing `/metrics`. Counters: connections_total, commands_
      total{verb,outcome}, telnet_iac_total{kind}. Histograms:
      dispatch_latency_seconds, tick_lag_seconds. Gauges: sessions,
      bucket_subscribers{bucket}.
- [~] Crash recovery — `internal/safego.Go(name, fn)` shipped:
      defers a `recover()` that logs panic + name + stack at
      `LevelError`. Wraps the shutdown-watcher, accept-loop's
      per-session goroutine, and the shutdown-drain in
      `cmd/server/main.go`. The telnet dispatcher
      (`telnet/server.go::runDispatcher`) inlines the same
      recover pattern to keep the telnet package boundary free
      of `internal/*` imports. The tick scheduler and eventbus
      already recover per-handler (existing inline recovers).
      Pending: metric counter for panics-by-name (waits on §19
      Prometheus endpoint) and any restart-on-panic policy
      (deliberate: a panic that survives recover means a logic
      bug to fix, not silently restart).
- [ ] Profiling endpoints (`net/http/pprof`) — same private :9090
      mux, gated behind `PPROF_ENABLED=1`. `/debug/pprof/*` routes
      registered via `import _ "net/http/pprof"`.

## 20. Configuration

- [x] `LISTEN_ADDR` env var
- [x] `DB_DSN` env var (default `wheelmud.db`; `:memory:` supported)
- [ ] Config file (TOML/YAML) for ports, paths, feature flags —
      `config.yaml` parsed at startup into a typed `Config` struct;
      env vars override individual fields (`WMUD_LISTEN_ADDR`,
      `WMUD_DB_DSN`). Feature flags grouped under `features:` block
      so wiring in MCCP/GMCP/etc. flips one bool.
- [ ] Per-environment overrides (dev/stage/prod) — `config.yaml` +
      `config.<env>.yaml` deep-merged based on `WMUD_ENV` env var.
      Production refuses to start if dev-only flags (e.g. wide-open
      pprof) are set.
- [ ] Secrets via env, never committed — `.env.example` checked in,
      `.env` gitignored. Required secrets validated at startup
      (`bcrypt_pepper`, `tls_key_path` if TLS enabled) — fail fast
      with a clear message rather than crashing later.

## 21. Testing & CI

- [x] Unit + contract tests across `telnet`, `internal/auth`,
      `internal/db`, `internal/mode`, `internal/repo`, `internal/session`
- [x] Dependabot config (`.github/dependabot.yml`)
- [ ] GitHub Actions — `.github/workflows/ci.yml` matrix on Go
      `1.24.x` running `go vet ./...`, `go test -race -coverprofile=
      coverage.out ./...`, `staticcheck ./...`, `gosec ./...`. Cache
      `~/go/pkg/mod` keyed on `go.sum`. Upload coverage artifact;
      block merge on red.
- [ ] Integration test that drives the telnet protocol against a
      real listener — `internal/integration` package starts a
      server on a random port with a `:memory:` DB and embedded
      world, dials in with a plain `net.Dial`, and scripts
      `expect`/`send` exchanges (login, look, move, quit).
- [ ] Fuzz tests on the IAC parser and command tokenizer —
      `FuzzReadIAC` feeds arbitrary byte streams into a pipe and
      asserts no panics + bounded memory; `FuzzTokenize` checks
      round-trip stability and quote handling. Run in CI under
      `go test -fuzz=Fuzz -fuzztime=30s` on PR.
- [ ] 80% coverage target tracked in CI — `go tool cover` parsed
      in a small script that fails the job below threshold; per-
      package floors documented so legacy gaps don't get worse.

## 22. Packaging & deploy

- [x] `Dockerfile` + `docker-compose.yml`
- [x] `Makefile` targets (`build/server`, `run/server`, `run/live/server`)
- [x] Hot reload via `air` for dev
- [ ] Versioned releases / `goreleaser` — `.goreleaser.yaml`
      builds linux/darwin amd64+arm64, embeds version via
      `-ldflags "-X main.Version=..."`, uploads tarballs +
      checksums + a Docker image to ghcr.io on tag push. Changelog
      from conventional-commit messages.
- [ ] Systemd unit / deploy doc — `deploy/wheelmud.service`
      template (`User=wheelmud`, `WorkingDirectory`, `EnvironmentFile=
      /etc/wheelmud/env`, `Restart=always`). `docs/deploy.md` walks
      a clean Debian install: user creation, dir layout, log
      rotation via `journald`, backup cron pointing at §7 rotation.
- [ ] Healthcheck endpoint or telnet-level liveness probe —
      `/healthz` on the metrics listener returns 200 if the accept
      loop and scheduler are alive (heartbeat counter advanced
      within last 5 s); 503 otherwise. Telnet variant: short-lived
      probe that opens a connection, expects the banner, closes.

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
- **§9 Zones slice follow-ups** (silent ambient marshal swallow,
  `Name`/`Builder` UTF-8 vs ASCII enforcement, `CountByZone` shared-
  suite gap, loader/schema default duplication) —
  `zones_followups.md`.
- **Off-world realms — design pending** (`docs/wot_geography_mud.md`):
  - **The Ways** (Waygate-linked off-world travel realm; reserved zone
    block 30000–30999). Mechanics: Avendesora-leaf seals, Machin Shin
    encounter, Island/Bridge/Guiding structure, time-slip on exit.
  - **Portal Stones / Mirror Worlds** (reserved zone block 31000–31999).
    Activation, instanced parallel-reality zones, alt-history one-shot
    spaces. **No schema or commands designed yet** — revisit once §10
    travel and §15 quest scripting are further along.
- **Sector enum extension — migration 0025 landed**: `blight`, `waste`,
  `stedding`, `swamp` added to `repo.Sector`, `validate.go::validSectors`,
  the rooms.sector CHECK (via writable_schema REPLACE on sqlite_master),
  `phase_ambient.go::sectorAmbients`, and `data/world/README.md`. Mechanical
  hooks still pending: channeling suppression in stedding (§12), ambient
  horror / DoT in blight (§11), movement-cost / encounter flavor in swamp
  and waste (§10–11).
- **Auto-derived room coordinates — design pending.** Today every
  room ships with `coord_x/y/z = (0,0,0)` unless a builder hand-
  authors them, so coord-aware features (admin `zonemap` direct-grid
  layout, distance/track heuristics, future path scripting) have
  nothing to read. Build a system that derives coords by BFS from
  the starter room (`repo.StarterRoomID`) over cardinal + diagonal
  + vertical exits, with these properties:
  - **Anchor preservation.** New column `rooms.coords_auto` (default
    `1`); the YAML loader stamps `0` whenever a `coords:` block is
    explicitly authored. The derivation pass only mutates rooms with
    `coords_auto = 1`. Multiple anchors are allowed; BFS propagates
    from each.
  - **Direction → delta map.** `n: y+1, s: y-1, e: x+1, w: x-1,
    ne/nw/se/sw: combined, u: z+1, d: z-1`.
  - **First-arrival wins.** Cycles that don't grid-align (n+n+s+s
    forming a non-trivial loop) are normal; first BFS visit assigns
    the coord and subsequent visits don't overwrite. Rooms reachable
    from multiple anchors with conflicting coords are surfaced in an
    admin `coords issues` report but not auto-resolved — builders pin
    them by adding an explicit `coords:` anchor in YAML.
  - **Run points.** (a) On boot, after world load, before listener
    accept; (b) after any room or exit mutation via OLC (§16) —
    incremental re-walk of the affected connected component, not a
    full rebuild; (c) admin `coords rebuild` command for forced
    re-derivation.
  - **Self-healing on deletion.** When a room or exit is removed,
    affected reachable rooms are re-walked. Newly-orphaned rooms
    (no path to any anchor) are flagged in `coords issues` rather
    than left with stale coords.
  - **Migration 0026 (proposed):** add `rooms.coords_auto
    INTEGER NOT NULL DEFAULT 1`; backfill existing rows to `1` (none
    today carry explicit coords); the loader updates `coords_auto`
    in lock-step with `roomInsertValues`.
  - **Admin commands (proposed):** `coords rebuild`, `coords show
    <room>`, `coords issues`.
  - **Unblocks:** zonemap coord-direct layout v2, distance-based
    spell range / track / line-of-sight heuristics, possible
    weather/lighting gradients keyed off coords.
