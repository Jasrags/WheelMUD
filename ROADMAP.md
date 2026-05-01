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
- [ ] Prompt templating (HP/MP/room placeholders) — `%h/%H` (cur/max
      HP), `%m/%M`, `%v/%V` (move), `%r` (room name), `%g` (gold),
      `%t` (target). `Mode.Prompt(*Session) string` already exists;
      add a `prompt.Render(template, *Character)` that's invoked
      from `Game.Prompt`. Deferred until the character/world model
      lands so there are real values to interpolate.
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
- [x] Per-command argument completer — `Command.Completer` field;
      `help` ships a real one (sample: tab on `help <prefix>`)
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
- [ ] Periodic + shutdown autosave — dirty-bit tracking on character
      and mob-instance aggregates; a `save` bucket fires every
      30 s and walks the dirty set; `srv.shutdown()` performs a
      final pass under the 10 s drain budget. Saves are idempotent
      `UPSERT`s so a crash mid-loop doesn't corrupt rows.
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

- [~] **Room** — `repo.Room` exists with id, zone, name, description,
      `external_id`. Pending: flags (`indoors`, `nopvp`, `noteleport`,
      `dark`, `silent`, `peaceful`), sector/terrain enum (city, forest,
      water, air, underwater) for movement cost + swim/fly gating,
      ambient/extra descriptions keyed by phrase (`look fountain`),
      light level (auto-dark at night unless lit), per-room reset hooks,
      coordinates for `map`/`track` (§10).
- [~] **Exit** — `repo.Exit` has `from_room_id`, `direction`, `to_room_id`.
      Pending: door flags (`closed`, `locked`, `pickable`, `hidden`,
      `nopass`), key item id, lock difficulty, exit-specific
      description ("a heavy oak door"), one-way exit support,
      diagonal directions (`ne`/`nw`/`se`/`sw`) — the
      `exits.direction` CHECK constraint must widen first
      (already noted in `world_aggregates_followups.md`).
- [ ] **Area / zone** — currently rooms carry a free-text `zone`
      string. Promote to a real `zones` table: id, name, builder,
      level range, reset interval, reset mode (`always` / `empty` /
      `never`), climate, ambient messages. Reset rules live in a
      sibling `zone_resets` table (load mob X into room Y up to count
      Z; load item into room/container; set door state; equip mob).
      Resets fire from the §8 `areaReset` bucket.
- [~] **Item / object** — `repo.Item` covers id, room/zone, name,
      description, `external_id`. Pending: type enum (`weapon`,
      `armor`, `container`, `consumable`, `light`, `key`, `scroll`,
      `wand`, `currency`, `trash`), weight, value (in base currency),
      wear-slot bitmask, material, condition/durability, item flags
      (`notake`, `nodrop`, `nosell`, `bind-on-pickup`, `magic`,
      `glow`, `hum`), level requirement, class/align restrictions,
      affects list (stat mods, resistances, regen) with duration for
      consumables, charges for wands/scrolls, type-specific stat blob
      (weapon dice + damage type, armor AC, light fuel ticks, key id).
- [ ] **Container semantics** — capacity (weight + slot count),
      open/closed/locked state sharing the door schema, content
      visibility flag (`see-inside`), nested-container depth cap,
      take/put permission flags, weight-reduction multiplier (bag of
      holding), liquid containers as a separate subtype with capacity
      in sips and a liquid id.
- [~] **Mob / NPC** — `repo.Mob` has id, room/zone, name,
      description, `external_id`. Pending: stat block (level, HP/MP,
      str/dex/con/int/wis/cha, AC, hit/dam dice, attack type,
      damage type), behavior flags (`aggressive`, `wimpy`,
      `sentinel`, `scavenger`, `assist-same-race`, `helper`),
      faction / race / class, default loadout (eq + inventory),
      gold range, XP award, corpse decay timer, spawn rules
      (zone reset references), dialogue/script hooks (§15
      triggers), shopkeeper subtype (inventory list, buy/sell
      multipliers, hours), quest-giver flag.
- [ ] **Player character extending the mob model** — share a
      `creatures` core (stats, position, equipment, inventory,
      affects) and split into `mob_template` (immutable) /
      `mob_instance` (in-world state) / `character` (persisted
      player). Player adds: account fk, class, race, alignment,
      XP/level curve state, skill/spell book, quest log,
      hunger/thirst, condition (standing/sitting/sleeping/fighting),
      idle timer, bank balance, played-time counter, last-login.
- [ ] **Equipment slots and wear/wield logic** — slot enum
      (`light`, `head`, `neck`, `body`, `arms`, `hands`, `wrist-l/r`,
      `finger-l/r`, `waist`, `legs`, `feet`, `wield`, `hold`,
      `shield`, `back`, `face`, `ear-l/r`, `float`), wear-flag
      validation against item, two-handed weapons consume `wield` +
      `hold`, swap semantics, save/restore on login, affects
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
- [ ] Doors, locks, keys — depends on the §9 Exit door-flag fields.
      Commands: `open <dir>`, `close <dir>`, `lock <dir>`, `unlock
      <dir>`, `pick <dir>`. Movement through a closed door auto-fails
      with "The door is closed."; `nopass` exits never auto-open.
      Key match by item id; lockpick uses a skill check (§12).
- [ ] `examine` for detailed inspection — `examine <target>` resolves
      against room mobs, room items, inventory, and equipment in that
      order. Renders the long description, condition (for items),
      visible affects (for mobs), and contents (for open containers).
- [ ] `map` / mini-map ASCII rendering — BFS from the current room
      out to a configurable depth (default 3), lay out by exit
      direction onto a grid (north decreases y, east increases x),
      render with `[*]` for current, `[ ]` for visited, `:` for
      unknown. Up/down indicated with `^v` glyphs in the cell.
      Skips rooms with the `nomap` flag.
- [ ] Trails / `track` command — `mob_trails` ring buffer keyed by
      `(mob_id, room_id, ts)` updated on every move; `track <name>`
      finds the freshest trail entry within the last N minutes and
      reports the first-step direction. Fades with time; skill
      check determines the max staleness the tracker can read.

## 11. Combat

- [ ] Initiative / round structure — combat ticks off the `combat`
      bucket from §8 (default 4 s). On `attack <target>` push both
      participants into a `Fight` aggregate keyed by room; each pulse
      resolves one round per combatant in initiative order
      (`d20 + dex_mod`, ties broken by random). Players queue one
      `combat-mode` command (`flee`, `kick`, cast a spell) per round.
- [ ] Damage types and resistances — enum `slash/pierce/bludgeon/
      fire/cold/lightning/acid/poison/holy/shadow/psychic`. Mobs and
      items carry a `[]Resist{Type, Pct}`; `applyDamage(target, dmg,
      type)` multiplies by `1 - resist + vuln`. Negative resist =
      vulnerability. Surface in `examine` for inspectable creatures.
- [ ] Hit/miss/dodge/parry rolls — attacker rolls `d20 + hit_bonus`
      vs defender AC; on a hit, defender gets a dodge check
      (`d20 + dex_mod` vs DC), then a parry check if wielding a
      weapon. Crit on natural 20 doubles dice, fumble on natural 1
      drops the weapon. All rolls go through a single `combat.Roll`
      seam for deterministic tests.
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
      hit_die, primary_stat, save_progression, skill_list). Multi-
      class deferred. Race separately gates stat ranges and innate
      abilities. Both selected during character-create after name +
      stat-roll step.
- [ ] Skill tree with practice / training — `character_skills`
      (`character_id`, `skill_id`, `percent`). `practice <skill>`
      at a guildmaster spends a practice point to raise the cap;
      use raises percent toward cap on success and (slowly) on
      failure. Caps and gain rates live in `skills` table seed.
- [ ] Spell list with mana costs and reagents — `spells` table
      (id, name, school, mana, cast_time_ticks, target_type,
      reagents []ItemID, effect script ref). `cast <spell> [target]`
      validates mana + reagents + line-of-sight, locks caster for
      `cast_time` ticks (interrupt on damage), then resolves via
      the effect script (§15).
- [ ] Levels & XP curve — geometric curve (e.g. `xp(n) =
      base * 1.6^(n-1)`) capped at level 100 v1. Level-up grants
      hit die roll, mana/move pool growth, practice points, and
      title slot. Stored on character; `train` command at trainer
      mob spends accumulated stat trains.
- [ ] Affects / buffs / debuffs with durations — `creature_affects`
      list `(source_id, name, modifiers []StatMod, duration_ticks,
      tick_effect)`. The `combat`/`regen` buckets decrement
      durations; `tick_effect` fires per-tick (DoT, regen, fear
      check). Stacking rules: same `(source, name)` refreshes;
      different sources stack up to a per-affect cap.
- [ ] Cooldowns and global lag — per-skill `cooldown_until` on the
      character + the §4 `Command.Lag` global lag. Display in
      `cooldowns` command grouped by skill family.

## 13. Communication

- [ ] `say` (room-scoped) — emits `You say, "<text>"` to caller and
      `<Name> says, "<text>"` to other room occupants. Strips control
      chars, caps length (e.g. 1024). Hooks the §15 NPC `on_say`
      trigger so dialogue can react.
- [ ] `tell` / `whisper` (private) — `tell` is global by character
      name (resolved through `session.Registry`), `whisper` is room-
      local and visible to bystanders ("X whispers something to Y").
      Maintain a `LastTellFrom` on `Session` so `reply <text>` works.
      Blocked by ignore list and `nochannels` flag.
- [ ] `shout` / `yell` (zone-wide) — broadcast to every session whose
      character is in the same `zone_id`. Higher cost (small move
      drain) to discourage spam. Suppressed in `silent` rooms.
- [ ] Channels (`ooc`, `gossip`, `newbie`) with on/off toggles —
      `channels` table seeds (`name`, `min_level`, `color`). Per-
      character `channel_settings` JSON tracks on/off + per-channel
      mute. Dispatch via the §8 event bus so admin tools can snoop.
      Built-in: `ooc`, `gossip`, `newbie` (auto-leave at level 10),
      `auction`, `clan` (filtered by clan id).
- [~] `who` — currently shows only the caller's character name; full
      multi-session listing waits on iterating `session.Registry`
      (§6). v1 list: name, level, class, idle time, AFK flag,
      title; sortable by `who level`/`who class <name>`.
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

- [ ] `inventory`, `get`, `drop`, `give`, `put`, `take` — operate on
      `character_inventory` (item instance ids) + room-floor item
      table. Keyword resolution `<n>.<keyword>` (e.g. `2.sword`)
      to disambiguate. Weight cap from str-based formula; over-
      encumbered blocks pickup with "It's too heavy."
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
- [ ] Log levels controllable via env / config — `LOG_LEVEL` env
      (`debug|info|warn|error`) feeding `slog.HandlerOptions.Level`,
      plus a `loglevel <name>` admin command that swaps the level
      at runtime via an `slog.LevelVar`.
- [ ] Request/command audit log per character — append-only
      `command_log` table (`character_id`, `verb`, `args`, `at`,
      `latency_us`, `outcome`) gated by a config flag (off in dev).
      Keep N days, then prune. Useful for griefer post-mortems.
- [ ] Metrics endpoint (Prometheus) — separate `:9090` HTTP listener
      exposing `/metrics`. Counters: connections_total, commands_
      total{verb,outcome}, telnet_iac_total{kind}. Histograms:
      dispatch_latency_seconds, tick_lag_seconds. Gauges: sessions,
      bucket_subscribers{bucket}.
- [ ] Crash recovery — wrap every goroutine spawned by the server
      (`go` calls in `main`, `tick.Scheduler`, `eventbus`, dispatch)
      in a `safego(name, fn)` helper that defers a `recover()` +
      stack-dump log + metric increment. Listener and scheduler
      restart on panic; per-session goroutine just terminates the
      session.
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
