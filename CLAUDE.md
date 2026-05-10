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
MUD subsystems and is the source of truth for "what's done."
`docs/PLAN.md` is the source of truth for "what's next" — it sequences the
roadmap into ordered phases (A: quick wins, B: equipment/economy, C: combat,
D: progression, E: NPCs/quests, F: OLC, G/H/I: comms/network/ops). When
ROADMAP and PLAN disagree about *status*, ROADMAP wins; about *order*, PLAN
wins. Re-derive PLAN from ROADMAP whenever a phase finishes or scope shifts.
Token-lean architecture maps for AI context live in `docs/CODEMAPS/`.

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
  `internal/db.Open` (runs embedded migrations 0001–0048), constructs every
  repo (accounts, characters, rooms, exits, items, mob_instances,
  mob_templates, mob_trails, zones, channels), loads the news catalog
  (`internal/news`), the chargen catalog (`internal/chargen`),
  and the quest catalog (`internal/quest`),
  runs `world.LoadAndSync` to seed the DB from
  `WORLD_DIR`, builds the command registry plus a `server` struct
  holding long-lived deps, starts `tick.Scheduler` + `tick.Buckets`
  and the `persist.Manager` autosaver, then accepts TCP connections.
  Each connection gets a `telnet.Session`, the connect splash is
  written, the initial login mode is pushed, and `telnet.RunSession`
  drives it. After login, `news.WriteMOTDBlock` renders any unseen
  MOTD/news entries (gated by `characters.last_news_seen_at` from
  migration 0027). New long-lived dependencies belong on the
  `server` struct.

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
  `quit`, `colors`, `who`, `say`, `tell`, `reply`, `shout`/`yell`
  (zone-wide broadcast), `alias`/`unalias`, `prompt` (per-character
  template), one verb per channel catalog row plus a `channels`
  overview, `help`, `look` (incl. `look in <container>`), `examine`,
  the move family (`n`/`s`/`e`/`w`/`u`/`d`/etc.), `teleport`, the
  door verbs (`open`/`close`/`lock`/`unlock`/`pick`), the inventory
  verbs (`inventory`/`get`/`drop`/`give`/`put`) with `get <item>
  from <container>` semantics and capacity-aware `put`, the
  equipment verbs (`wear`/`wield [off]`/`remove`/`equipment`/`eq`)
  driving the `creature.Equipment` slot map persisted in
  `equipment_json` (an overlay on inventory — equipped items keep
  `owner_character_id`, `inventory` annotates them as
  `(worn)`/`(wielded)`/`(offhand)`, and `drop`/`give`/`put` call
  `autoUnequipIfHeld` so leaving inventory never strands a slot
  pointer), the §14 shop verbs (`list`/`buy`/`sell`/`value` —
  resolve a shopkeeper from `mobs.ListInRoom` + `shops.GetByMobTemplateID`,
  apply `sell_markup` / `buy_markdown` against `Item.Value`,
  honour `FlagTradeGood` for full-price sells and `FlagNoSell` /
  `BuyTypes` for refusals; `buy` clones the YAML-seeded item
  template via `ItemRepo.Create` with a fresh unique
  `external_id`, `sell` `Delete`s the item — V1 doesn't restock
  the shop from sales), the §14 banker verbs
  (`balance`/`deposit`/`withdraw` — resolve a banker from
  `mobs.ListInRoom` + `bankers.GetByMobTemplateID`, gate on
  `Banker.IsOpenAt(clock.HourOfDay())`, move coin via
  `CharacterRepo.RecordCoin(coin, bank, expectedVersion)`; deposit/withdraw audit
  on success — refusals do not), the BFS
  minimap (`map`, default depth 3, max 5), the bigger `zonemap`,
  the auto-coords admin verbs (`coords rebuild`/`show`/`issues`),
  `track`, `time`, `news`, the §D #18 combat verbs
  (`attack`/`kill <target>` and `flee`/`run`/`parry`),
  `pvp [on|off]` (§D #21 PvP opt-in toggle),
  the §D #22 party verbs (`group`
  with `invite`/`accept`/`decline`/`leave`/`kick`/`disband`
  subcommands plus bare `group` roster, `follow <player>`,
  `unfollow`), the §D #19 read-only `score` sheet, the §E #23
  progression verbs (`xp` showing pending levels, `train` at a
  trainer NPC committing one class level), the §E #24 spend verb
  (`learn <skill> [n]` / `learn info <skill>` — anywhere, no
  trainer required), the §E #25 remaining spend verbs
  (`feat [id]` / `feat info <id>` draining `pending_feats`, no
  prereq enforcement V1; `bump <ability>` draining
  `pending_ability_bumps` with a hard cap of 20 — verb name `feat`
  not `pick feat` because `pick` is the lockpicking verb;
  `learn weave [id]` / `learn weave info <id>` draining
  `pending_weaves`, channeler-only and affinity-gated via
  `Channeling.Affinities`, dispatched off the `learn` verb's
  router with a leading `weave` arg), the §F #30 dialogue verb
  (`talk <mob>` — resolves the NPC via `mobs.ListInRoom` +
  `MatchMob`, decodes `MobTemplate.DialogueJSON`, hands off to a
  `cmd.PushDialogueFn` closure provided by `cmd/server/main.go`
  that builds and pushes `*mode.Dialogue`; refusal lines for
  no-mob / no-tree / corrupt-tree are name-aware), the §F #31
  quest verb (`quest` / `quests` — bare lists active +
  completed; `quest info <id>` renders steps with ✓ / ▸ /
  · markers; `quest abandon <id>` drops the entry and audits),
  the admin movement verbs (`goto <player|
  room>`, `transfer <player> [<room>]`, `summon <player>`,
  `wizinvis` toggle), the `shutdown` / `reboot` countdown verbs,
  and the admin tools (`whereami`, `zones`,
  `spawn mob <ext> [count]` / `spawn item <ext> [count]`). New
  commands take their dependencies (repos, registry, sessions, bus)
  by parameter and return a `*telnet.Command`; commands with
  required arguments declare `MinArgs` + `Long` so the dispatcher
  emits a Long-aware usage block on too-few-args. Item/mob keyword
  resolution (including ordinal `2.sword`) goes through
  `keyword.go::MatchItem` / `MatchMob`; encumbrance bands come from
  `encumbrance.go::LoadFor` (Str-keyed d20 carrying-capacity table)
  and now sum recursive container contents (transitive ownership)
  via `ItemRepo.ListAllOwnedTransitive`.

- **`internal/mode/`** — login, character_select, character_create, game,
  postauth promotion. `promoteToGame` stamps `CharacterID`,
  `CharacterName`, `CurrentRoomID`, calls `Session.SetChannelMuted` from
  the loaded character, then ReplaceMode's into game.

- **`internal/repo/`** — typed repos with sqlite + memory implementations
  and a shared test suite. Every repo writes through on mutation; the
  `persist.Manager` Save bucket layers periodic + shutdown flushes for
  fields that aren't covered (e.g. `last_played_at`).

- **`internal/db/migrations/`** — embedded migrations 0001–0047. Each
  migration is forward-only (no down). 0008 introduced the polymorphic
  creature/mob_template/mob_instance/channeling tables; 0010 dropped
  the legacy `mobs` table; 0011 added the chat-channel catalog +
  `channels.channel_settings_json`; 0012 added room flags + sector;
  0013 added room extra-descs JSON; 0014 added exit door flags + key
  + lock difficulty + description; 0015 added the item taxonomy
  columns; 0016 added the `zones` table + `rooms.zone_id` (soft FK,
  default 0; loader stamps real ids); 0017 added
  `items.owner_character_id` (nullable, soft FK) so items can sit on
  a room floor or in a character's inventory but never both; 0018
  briefly placed `auth_level` on accounts; 0019 moved it to
  characters (so one account can own admin and player characters
  side-by-side) and dropped `accounts.auth_level`. Existing rows
  inherited their account's level via the 0019 backfill. 0020 added
  `rooms.nomap` so the §10 minimap can hide secret hideouts and admin
  zones from the player-facing BFS. 0021 added the `mob_trails` table
  (per-mob `(room_id, ts)` history feeding `track`); 0022 added
  `mob_templates.wander_chance`; 0023 added
  `characters.prompt_template` for the `prompt` verb; 0024 added the
  `world_state` key/value table (currently storing `world.ticks` for
  the Clock); 0025 widened `rooms.sector` to the full sector enum;
  0026 added `rooms.coords_auto` (1 = derived by BFS, 0 = anchor
  authored in YAML) backing the auto-coords pass; 0027 added
  `characters.last_news_seen_at` for the MOTD/news gate; 0028 added
  `items.parent_item_id` (nullable, soft self-FK) so an item can
  live inside another item, completing the location invariant
  (room ⊕ owner ⊕ parent). 0029 added the `admin_audit` table —
  append-only forensic log for privileged-verb invocations,
  populated by `internal/audit.Record` from every admin verb's
  success path. 0030 added `shops` + `shop_stock` for the §14
  shopkeeper subsystem — `shops` is keyed 1:1 to a mob_template
  (UNIQUE on `mob_template_id`), `shop_stock` is per-line
  `(shop_id, item_external_id)` with `qty` / `qty_max` /
  `last_restock_ts`. Sentinel `qty == -1 && qty_max == -1` is
  infinite stock. 0031 added `bankers` for the §14 banker subsystem
  — keyed 1:1 to a mob_template (UNIQUE on `mob_template_id`),
  carrying operating hours only. V1 has no fees, no min-deposit,
  and no item vault; coin moves between `characters.coin` and
  `characters.bank_balance` via `CharacterRepo.RecordCoin`. 0032
  added `characters.coin_version` — an optimistic-lock token bumped
  on every `RecordCoin`. Coin-mutating verbs pass the
  `Character.CoinVersion` they computed against; the repo refuses
  the UPDATE on mismatch with `ErrCoinConflict`, mirroring the
  `ItemRepo.Transfer*` / `ErrItemMoved` pattern. Verbs surface this
  as "your balance/purse just changed — try again" (sell, deposit,
  withdraw, give); `buy` logs-and-accepts because the item already
  shipped. 0033 added `characters.channeling_json` (TEXT NOT NULL
  DEFAULT 'null') backing the §C #15 slice 2 channeler-branch
  chargen substep — non-channeler classes round-trip 'null', the
  two channeler classes (Initiate, Wilder) write a JSON-encoded
  `*creature.Channeling` carrying GenderSource (auto-derived from
  Gender), Affinities (PowerSet bitmask, exactly 2 picks at
  chargen), and `WeavesKnownIDs []string` (3 picks from the level-
  0 catalog filtered by affinity). The transitional string-id list
  is a sibling to `WeavesKnown []WeaveRef`; §12 will reconcile the
  two when the numeric weave table lands. 0034 extended `admin_audit`
  with `actor_account_id INTEGER NOT NULL DEFAULT 0` and `actor_type
  TEXT NOT NULL DEFAULT 'character'` so the post-login account menu
  (slice 1b: delete-character; slices 2+ password / settings /
  security) can record account-mode rows. Existing rows backfill
  to ('character', 0); `audit.RecordAccount(ctx, repo, accountID,
  accountUsername, verb, target, args)` writes the new row shape.
  `audit.Record` (character-mode) keeps its existing call sites.
  0035 added `accounts.settings_json` (TEXT NOT NULL DEFAULT '{}')
  backing the §6 post-login account-menu "settings" sub-menu (slice
  3 — color/prompt/width/locale/MOTD-toggle). The blob persists
  `repo.AccountSettings` (`ColorOverride`, `PromptDefault`,
  `WidthOverride`, `Locale`, `MOTDAlways`); the zero value
  round-trips through `{}` and means "use server defaults". Apply
  points: `mode/postauth.go::applyAccountSettings` stamps Color/
  Width onto the session immediately before `promoteToGame`;
  `CharacterCreate.SetSettings` forwards the bag so chargen stamps
  `PromptDefault` onto `Character.PromptTemplate` at finalize time;
  `MOTDAlways` flattens the `last_news_seen` watermark to zero in
  both `postAuth` and `AccountMenu.handleNews`. Locale feeds the
  account-menu's character-list date formatter only — wider
  locale-aware rendering (the `time` verb, etc.) is deferred.
  0036 added the `account_logins` table — append-only per-account
  authentication-event log backing the §6 post-login account-menu
  "security" sub-menu (slice 4). One row per outcome on every login
  attempt (`success` / `failure` / `lockout`) plus one row per kicked
  peer when the menu's `kick` verb runs (`kick`). Schema mirrors
  `admin_audit` (0029): no FK on `account_id`, ts as unix seconds,
  indexed by `(account_id, ts)`. `info` is a short fixed-vocabulary
  note (`"wrong password"`, `"locked"`, `"kicked by other-session"`)
  and NEVER carries the typed password. `Login.SetLogins` /
  `Create.SetLogins` thread the repo through `postAuthDeps` to
  `AccountMenu.SetLogins`; the security view calls
  `ListRecentByAccount(s.AccountID, 10)` and pairs it with
  `session.Registry.Snapshot()` for the active-session list. Single-
  session-per-account makes `kick` a no-op today; the path is
  forward-wired for multi-session work.
  0037 added `characters.pvp` (INTEGER NOT NULL DEFAULT 0) backing
  §D #21 slice 1 (PvP opt-in). The `pvp` verb toggles via
  `CharacterRepo.RecordPvP`; `attack <player>` resolves a peer in
  the same room from `session.Registry` and runs the guard order
  (NoPVP room → attacker newbie → target newbie → attacker opt-in
  → target opt-in) before queueing an `ActionAttack` against an
  `ActorRefForCharacter(id)` defender. Newbie cap is
  `cmd.NewbiePvPLevelCap = 10`; `cmd.characterLevel(ch)` sums
  `ClassLevels` values. The pvp column slots between
  `last_news_seen` and `auth_level` in `charPlayerColumns` —
  auth_level stays last for the SQLite first-character bootstrap
  CASE expression in `Create`.
  0038 added the `trainers` table (1:1 keyed to a mob_template via
  UNIQUE on `mob_template_id`) backing Phase E #23 slice 2 — each
  row maps a trainer NPC to the chargen `class_id` they teach.
  Optional `trainer:` YAML block on a mob seeds the row; the
  `train` verb resolves the trainer in the room via
  `mobs.ListInRoom` + `trainers.GetByMobTemplateID`. No fees / no
  level cap on the trainer (V1).
  0039 added four `pending_*` int32 columns on `characters`
  (`pending_feats`, `pending_skill_points`, `pending_ability_bumps`,
  `pending_weaves`, all NOT NULL DEFAULT 0) backing Phase E #23
  slice 4 — pools deposited on level-up by `train` and decremented
  by spend verbs (`learn` shipped as #24; `pick feat` / `bump
  <abil>` / `learn weave` deferred). `RecordLevelUp` accumulates
  the four counters in the same UPDATE that writes ClassLevels +
  HP/BAB/saves. The pending columns slot strictly between `pvp`
  (0037) and `auth_level`.
  0040 added `mob_templates.xp_value INTEGER NOT NULL DEFAULT 0`
  backing Phase D §19 polish — per-template XP override consumed
  by `combat.xpValueForTemplate` (zero falls back to the
  `xpValueForChallenge` A→I curve). Optional `xp_value:` YAML key
  on mob entries; the loader threads it from `world.Mob.XPValue`
  into `MobTemplate.XPValue`. The same slice wires
  `MobTemplate.GoldDice` (already in 0008) end-to-end: at death
  the dice roll spawns an `ItemTypeTradeGood` "a small pile of
  coins" inside the corpse with `Value = rolledCp` and
  `FlagTradeGood`. Mob inventory transfer uses the new
  `ItemRepo.SetParent(itemID, parentID)` — unconditional sibling
  to `SetOwner` / `SetRoom`; clears the other two location
  columns atomically. Death-pipeline rule: per-item / per-roll
  failures log via `slog.Warn` and continue so the despawn still
  resolves.
  0041 added `characters.xp_debt INTEGER NOT NULL DEFAULT 0`
  backing Phase D §19 player-death — passive XP-debt drained off
  the top of future XP awards via
  `combat.ApplyXPAward(award, debt) → (gain, newDebt)`. Slots
  strictly between `pending_weaves` (0039) and `auth_level`
  (must remain trailing for the bootstrap CASE). Debt is set on
  death (`combat.handleCharacterDeath`, delta =
  `(xp - XPForLevel(level)) / 10`) and decremented by the
  kill-XP loop. New repo method
  `RecordXPDebt(ctx, id, debt int64)` — absolute write, mirrors
  `RecordXP`. New combat events: `CharacterDied` and
  `CharacterRespawned`; `CombatXPAwarded` extended with
  `DebtTaken int64`. Cmd-layer subscribers in
  `cmd/server/main.go` broadcast death/respawn lines and stamp
  the victim's session via `Session.SetCurrentRoom` +
  `cmd.RenderRoom` (the repo-side `RecordRoom` already
  persisted the move). Player keeps inventory / equipment /
  coin — no player corpse spawned.
  0043 added the `weave_teachers` table backing Phase E #28
  mid-game weave learning — keyed 1:1 to a mob_template (UNIQUE
  on `mob_template_id`), carrying `max_level_taught int8` and
  `affinity_filter int8` (creature.PowerSet bitmask; 0 = "teach
  any in-affinity weave"). Optional `weave_teacher:` YAML block
  on a mob seeds the row. The `learn weave` verb now branches
  on room context: with a teacher present it drains
  `characters.practice_points` (column added in 0009 but
  previously unwritten) and consults `chargen.Weave.PracticeCost`
  per pick; without a teacher it keeps the existing
  `pending_weaves` chargen-pool drain. New repo method
  `RecordWeaveStudy(ctx, id, weaveID, newPracticePoints int16)`
  mirrors `RecordWeavePick`'s tx shape but writes
  `practice_points` instead of `pending_weaves`. PP earning:
  `progression.ComputeLevelUp.PracticeDelta = 1` per level (every
  class — non-channelers accrue but have no spend path yet),
  threaded into `LevelUpFields.PracticePointsDelta` by the
  `train` verb and applied via `RecordLevelUp`'s incrementing
  UPDATE. Audit row on the mid-game path:
  `verb=learn target=<weaveID> args=kind=weave_study power=<p> cost=<n>`.
  V1 has no fees, no time cost, no outside-affinity learning;
  the `learn weave` affinity refusal still applies on both paths.
  0044 added the `triggers` table backing §15 / Phase F #29 — one
  row per declarative event handler attached to a mob_template or
  a room. Schema: `(owner_kind, owner_id, event, match, action,
  payload, priority)`. `owner_kind` is CHECK-constrained to
  `('mob_template','room')` (item-owned triggers deferred until
  #32 lands the Lua surface); `event` is CHECK-constrained to
  `('on_enter','on_say','on_attack','on_death','on_tick')`.
  `match` is event-specific text — case-insensitive substring
  keyword for `on_say`, bucket name for `on_tick`, ignored on the
  other events. `payload` is action-defined JSON (e.g.
  `{"text": "..."}` for the say/emote builtins). YAML loader
  inserts rows in the same transaction as the owning mob/room
  via the optional `triggers:` sub-block on `Mob` / `Room`;
  validation in `validate.go::validateTriggers` rejects unknown
  event names + empty actions + malformed payloads at boot. The
  `internal/trigger/` package owns the in-memory `Registry`
  (built from the table once at boot via
  `Registry.Reload(ctx, repo.TriggerRepo)`), the
  `ActionRegistry` (V1 builtins: `noop`, `say`, `emote` —
  consumers extend this for #30 dialogue / #31 quests / #32 Lua),
  the `Runner` (priority-DESC fan-out, swallows handler errors),
  and the `Dispatcher` wiring eventbus subscriptions
  (`world.PlayerEntered` → `on_enter`, `world.PlayerSaid` →
  `on_say`, `combat.CombatHit` → `on_attack`,
  `combat.CombatDeath` / `combat.CharacterDied` → `on_death`)
  plus `tick.Buckets.Phase` for `on_tick`. `world.PlayerSaid`
  is a NEW event published by `internal/cmd/comm.go::NewSay`
  after the room broadcast (silent rooms / empty payload
  short-circuit before the publish). Action handlers MUST use
  `Session.WriteAsync` for peer writes — they run on the
  eventbus goroutine, not a dispatcher (the cross-session output
  rule applies). Dispatcher attaches once at boot in
  `cmd/server/main.go` after the channeling ticker; shutdown
  drain calls `srv.triggers.Stop()` before `bus.Stop()` so
  in-flight subscriptions cancel cleanly.
  0046 added `triggers.consecutive_faults` and `triggers.disabled`
  (both INTEGER NOT NULL DEFAULT 0) backing §15 / Phase F #32
  slice 1 — per-trigger fault budget. Lua action handlers wrap
  their classified errors with `trigger.ErrActionFaulted`; the
  `trigger.Runner` increments the counter on each fault and
  auto-disables at `FaultThreshold = 5`. Successful invocations
  reset the counter to 0. Disabled triggers are skipped silently
  in `Runner.Fire`; recovery is operator-managed (direct SQL or
  re-deploy). World loader resets both columns to 0 at boot via
  `TriggerRepo.ResetAllFaults` so a re-deploy never preserves
  stale fault state. The new `lua` action kind is registered by
  `cmd/server/main.go` against `triggerActions` BEFORE the
  Dispatcher starts; payload is `{"script": "<name>"}` and the
  handler resolves the catalog entry, runs it through
  `internal/lua.Runner`, and wraps any classified Lua error in
  `ErrActionFaulted`. Action handlers gain optional access to
  `repo.TriggerRepo` via `ActionDeps.Triggers` — only the
  fault-budget plumbing uses it today.
  0048 added `exits.authored_closed` and `exits.authored_locked`
  (both INTEGER NOT NULL DEFAULT 0) backing §7 area/zone reset
  extension — door state on AreaReset. The loader stamps both
  columns from the YAML closed/locked at boot; `ZoneResetter`
  (formerly `Respawner`, see `internal/world/respawn.go`) snaps
  the runtime columns back to authored on every per-zone reset
  via `ExitRepo.RestoreAuthored(ctx, fromRoomIDs)` (one zone-
  scoped UPDATE in the SQLite impl). Backfill UPDATE on existing
  rows copies the current runtime state into the new columns —
  pre-0048 zones lose their original authoring data (it was never
  recorded), so the first reset on an upgraded DB is a no-op until
  a builder edits a door. Hidden / NoPass / Pickable have no
  authored / runtime split today (they can't change at runtime).
  Lock-step lists for the new columns: `exit_sqlite.go` keeps
  `exitSelectCols` + the `Create` INSERT + `scanExitInto` in
  sync; the loader's raw-SQL INSERT in `internal/world/loader.go::
  insertExits` mirrors them.
  0047 added `characters.skill_cooldowns_json` (TEXT NOT NULL
  DEFAULT '{}') backing Phase E #26 slice B — per-skill cooldowns.
  Stores a JSON map of `chargen.HashID(skillID)` (int32 keys
  encoded as JSON strings) → absolute deadline. Missing or
  past-`time.Now()` entries are treated as cleared by readers;
  `CharacterRepo.RecordSkillCooldown` (read-modify-write) prunes
  past-deadline entries on every write so the map stays bounded.
  Slot strictly between `xp_debt` (0041) and `auth_level` —
  `auth_level` MUST stay the trailing column for the SQLite
  first-character bootstrap CASE in `CharacterRepo.Create`. V1
  producer is the admin `cooldown <player> <skill> <seconds>`
  verb (mirrors affects #25 slice 1); the player `cooldowns` verb
  is the only reader. Real player skill-check verbs (track / hide
  / lockpick) will stamp at success when those gain skill-check
  gates.
  0045 added `mob_templates.dialogue_json` (nullable TEXT)
  backing §15 / Phase F #30 — NPC dialogue trees authored inline
  on the mob YAML entry (sibling to `shop:` / `trainer:` /
  `weave_teacher:` / `triggers:`). The reserved `dialogue_tree_id`
  INT column from 0008 stays unused (V1 trees are one-off per
  template). `internal/dialogue/` ships the pure-data model
  (`Tree`, `Node`, `Response`, `Effect`, `Show`) plus a
  `Validate` that rejects dangling `next` references, unknown
  effect kinds, missing effect args, and same-flag
  require/forbid combos. The YAML loader translates
  `DialogueDecl` → `*dialogue.Tree`, validates at boot, marshals
  compact JSON onto `MobTemplate.DialogueJSON` before
  `templates.Create`. `internal/mode/dialogue.go` is the runtime
  conversation mode — pushed by the `talk <mob>` verb
  (`internal/cmd/talk.go`, resolves NPC via `mobs.ListInRoom` +
  `MatchMob` + `templates.GetByID`, decodes + revalidates the
  tree). The mode renders the current node prompt + numbered
  flag-gated responses each turn via `Prompt`; `Handle` accepts
  bare number, free-text keyword (case-insensitive substring
  match against `Response.Match[]`), or `bye`/`quit`/`leave`/
  empty to pop. Effects fire in order before `Next` is followed:
  `set_flag` / `clear_flag` (mutates per-session flag bag),
  `goto` (overrides Next), `push_mode` (closure-injected by the
  cmd-layer in `cmd/server/main.go`'s buildRegistry; nil in V1
  → logs a warning), `end` (pops mode). Per-character branch
  state (current node id + flag bag) lives on the mode
  instance — drops on PopMode. Cross-package wiring rule:
  `internal/cmd` does NOT import `internal/mode` (chargen-cycle
  risk); the `talk` verb takes a `cmd.PushDialogueFn` closure
  that `cmd/server/main.go` constructs after both packages
  resolve.

- **`internal/scripts/`** — Phase F #32 slice 1 script catalog:
  one `*.lua` file per script under `internal/scripts/default/`,
  embedded via `//go:embed all:default` with `SCRIPT_DIR` env
  override (mirrors chargen / news / quest catalog pattern). The
  loader compiles every script at boot via gopher-lua's
  `LoadString` so a syntax error fails the boot loudly with the
  file path. Empty catalog is valid: deploys may ship without
  scripts authored, and the runtime falls through to the
  fault-budget path (unknown script names auto-disable the
  referencing trigger after 5 misses).

- **`internal/lua/`** — Phase F #32 slice 1 gopher-lua sandbox +
  runner. `NewSandboxedState()` strips dangerous globals (`os`,
  `io`, `debug`, `package`, `dofile`, `loadfile`, `loadstring`,
  `load`); `Runner` keeps a pre-allocated pool of 8 LStates served
  via a buffered channel (no sync.Pool — that path can synthesize
  states at Stop and we can't deterministically close them).
  `Runner.Run(ctx, scriptName, bind)` wraps the parent ctx with
  `CallTimeout = 50ms` and propagates via gopher-lua's
  `SetContext` so a runaway loop aborts within the timeout. We do
  NOT use `SetMx` — it's a millisecond deadline (not an
  instruction-count cap) that leaks a watchdog goroutine per call.
  `APIBindings.Bind(L)` registers the Slice 1 globals: `say`,
  `emote`, `log`, plus a read-only `ctx` table populated from the
  consumer's `CtxView` (event/room/actor/target/text/bucket).
  Slice 2 (Phase F #32) extends Bind with the V2 mutation surface:
  a `quest` table (`quest.accept(id)` / `quest.advance(id)`) and
  a top-level `push_mode(name)` global. Slice 3 added composing
  closures `apply_affect(target_id, effect_id [, duration])` /
  `give_item(target_id, external_id)` plus a read-only `target`
  table (`target.hp(id)` / `target.level(id)` / `target.classes(id)`).
  Slice 4 added `room.players()` / `room.mobs()` (resolved at bind
  time from `b.Ctx.RoomID` so scripts can't snoop on other rooms),
  `clock.hour()` / `clock.day()`, and the `apply_affect` 3rd
  duration-override arg (0 = catalog default). nil-bound hooks
  register classified-error stubs that raise `<api> not bound in
  this context` so misuse trips the trigger fault budget instead of
  surfacing as a generic "attempt to call nil". The trigger Lua
  action wires the hooks via `trigger.LuaHooks` (legacy alias
  `LuaQuestHooks` kept); the dialogue `script` effect wires them
  via `mode.DialogueHooks.RunScript` (closure-injected from
  `cmd/server/main.go`).
  `Runner.Stop()` closes every LState — must run BEFORE
  `bus.Stop()` in shutdown drain so any in-flight script
  observes ctx cancellation cleanly. The release path wipes the
  full surface (`quest`, `push_mode`, `apply_affect`, `give_item`,
  `target`, `room`, `clock`) alongside the V1 set so a pooled
  LState never observes a leaked closure from the previous borrow.

- **`internal/quest/`** — Phase F #31 quest engine: catalog
  (`Tree`, `Step`, `Reward`), validator (cross-refs against
  world mob_template + room ExternalIDs at boot), engine
  (subscribes to `combat.CombatDeath` for kill_n,
  `world.PlayerEntered` for reach_room; talk_to advances via
  the dialogue `advance_quest` effect). Authoring is one YAML
  file per quest under `internal/quest/default/<id>.yaml` with
  `QUEST_DIR` env-override (mirrors chargen / news). Step kinds:
  `talk_to`, `kill_n`, `reach_room`, and `script` (Phase F #32
  slice 2 — Step.Script names a `internal/scripts/default/<n>.lua`
  catalog entry; the engine has no event subscription for script
  steps and waits for an external `quest.advance(id)` Lua call,
  validated against the script catalog at boot via
  `quest.RefSets.Scripts`). `fetch` / `deliver` deferred. Per-character state lives on the
  existing `characters.quest_log_json` column (migration 0009)
  via `RecordQuestProgress`; engine reloads the log before each
  transition (correctness > throughput on the eventbus
  goroutine). `combat.CombatDeath` was extended with
  `MobTemplateID` + `MobTemplateExternalID` so the engine can
  match kill_n.Mob without re-fetching the dead instance row.
  Final-step transition grants XP via `RecordXP` and coin via
  `RecordCoin` with one optimistic-lock retry on
  `ErrCoinConflict` (mirrors shop verbs). All player-facing
  notifications go through `Session.WriteAsync` — engine runs
  on the eventbus goroutine, so cross-session output rule
  applies. Two new dialogue effects (`accept_quest`,
  `advance_quest`) use the closure-injection pattern from #30:
  `internal/mode/dialogue.go::DialogueHooks` is wired by
  `cmd/server/main.go::buildRegistry` to the engine's
  `AcceptQuest` / `AdvanceTalkTo` methods so `internal/cmd`
  and `internal/dialogue` stay free of `internal/quest`
  imports. Verb: `quest` / `quests` (`internal/cmd/quest.go`)
  with `info <id>` and `abandon <id>` subcommands. The kind-
  agnostic `Engine.Advance(charID, questID)` (Phase F #32 slice
  2) is the entry point for the V2 Lua `quest.advance` API: it
  advances `talk_to` and `script` steps and logs + no-ops on
  counter-driven kinds (`kill_n` / `reach_room`) so a buggy
  script can't skip a kill quota. The dialogue `script` effect
  (Phase F #32 slice 2) routes through
  `mode.DialogueHooks.RunScript` — `cmd/server/main.go` builds a
  closure that runs a catalog script through `lua.Runner.Run`
  with `bindings.QuestAccept` / `bindings.QuestAdvance` wired to
  `Engine.AcceptQuest` / `Engine.Advance`. PushMode stays nil on
  the dialogue path (no concrete cross-mode push targets in V2).
  Boot-time `validateDialogueScriptRefs` walks every
  mob_template's `dialogue_json` and rejects unknown script
  references so a typo fails the boot loudly.

- **`internal/progression/`** — pure-function helpers for the d20
  XP curve and level-up math (Phase E #23). `XPForLevel(n)`,
  `LevelForXP(xp)`, `XPToNext(xp)` (MaxLevel=20). `ComputeLevelUp(
  ch, cat, classKey) → LevelGains` recomputes ClassLevels + HP /
  BAB / saves and the per-pool deltas (FeatDelta / SkillDelta /
  AbilityDelta / WeaveDelta) the cmd-layer hands to
  `repo.RecordLevelUp` via `repo.LevelUpFields`. No DB / no
  session — content + math.

- **`internal/channeling/`** — pure helpers + per-tick driver for
  channeler state (Phase E #27). `RefreshIfDue(c, now)` refills
  `Slots[*].Cur` to `Max` once `RefreshInterval` (8h wall-clock)
  has elapsed since `c.LastSlotRefreshAt`; `AccrueMadness(c, now)`
  adds `MadnessPerPulse` (clamped at int16 max) iff the channeler
  is `Embraced` and drawing on `SourceSaidin`. Both are no-ops on
  `Stilled`. `SessionTicker` mirrors the affects ticker shape
  (Candidate snapshot from session.Registry) but skips the
  `FightLookup` gate — slots/madness are independent of combat
  pacing. Subscribed to `tick.Buckets.Regen` (30s). Verbs that
  flip the toggles: `embrace`/`release` (player) and
  `still`/`unstill` (admin, audited). `LastSlotRefreshAt` lives
  on `creature.Channeling` and round-trips through
  `characters.channeling_json` (no migration — added in #27).

- **`internal/group/`** — in-memory party manager (Phase D #22).
  `Group` aggregate (Leader CharacterID + Members map);
  `Manager` keyed by leader with reverse `byCharacter` index.
  `MaxGroupSize = 6`; leader-leaves-disbands. Methods:
  `Invite`/`Accept`/`Decline`/`Leave`/`Kick`/`Disband`/`Of`/
  `SameGroup`/`MembersInRoom`/`PendingInvite`/
  `ClearForCharacter`. Wired into combat as a
  `combat.GroupResolver` callback so `expandTallyByGroup` can
  split per-character damage across in-room party members at
  XP-award time. No persistence — server restart drops party
  state.

- **`internal/world/`** — YAML zone loader that syncs `WORLD_DIR` into the
  DB on startup (zones/rooms/exits/items/mob_templates/mob_instances/
  shops/bankers). The on-disk tree is hierarchical (continent → nation →
  region → settlement → building); see `data/world/README.md` for
  the full zone.yaml schema (id, name, builder, level_range,
  reset_interval_s, reset_mode, climate, ambient), the optional
  `shop:` and `banker:` mob sub-blocks (§14), and the room-id /
  currency-string / typed-item-stats conventions builders need to
  know. `LoadAndSync` always parses + validates YAML on every boot
  (even when the DB is already populated and the insert path
  short-circuits) and returns a `LoadedWorld` whose
  `ItemSpecsByZone` (keyed by zone external_id) feeds the
  `ZoneResetter` — see `item_spec.go::buildItemSpec` for the
  YAML→`repo.Item` translation shared with `insertItems`. Also hosts
  the `Restocker` (refills sub-max `shop_stock` lines older than
  `restock_interval_s`, wired to `tick.Buckets.AreaReset` —
  5min default cadence), the `ZoneResetter` (formerly `Respawner`,
  same bucket; per-zone gate runs three steps in order — mob
  respawn from anchored templates, door restoration via
  `ExitRepo.RestoreAuthored`, item respawn via
  `ItemRepo.FindByExternalID` global presence check + `Create`),
  and the `Clock.HourOfDay()` helper backing the shop hour gate.

- **`internal/chargen/`** — YAML chargen content catalog
  (backgrounds, classes, feats, skills, weaves) loaded once at
  boot from `internal/chargen/default/*.yaml` (or `CHARGEN_DIR`
  override). Mirrors the `internal/news` / `internal/world`
  embed-with-override pattern. The Catalog is content, not state
  — it never touches the DB; chargen mode (#11+) reads typed
  structs from it. Cross-references (background → feats/skills,
  class → skills, weave → power) are validated at Load time so a
  catalog typo fails boot loudly. ID → `creature.Background` /
  `creature.Class` enum mapping is stamped on each entry so the
  chargen mode can persist selections through the existing
  Character schema.

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
- Cross-session output (broadcasts to peers, channel fanout, mob
  arrival/departure, phase ambients, anything writing to a session
  that isn't the current dispatcher's `c.Session`) MUST use
  `Session.WriteAsync`, not `WriteString`. WriteAsync wraps the message
  with a CR+EL erase prefix and replays the cached prompt + line-edit
  buffer afterwards so a mid-line broadcast doesn't clobber the
  player's prompt or in-progress input. The dispatcher caches each
  prompt via `WritePrompt`; mode transitions clear the cache via
  `ClearLastPrompt` (handled by `PushMode`/`PopMode`). Synchronous
  dispatcher output (the command's own response to `c.Session`) keeps
  using `WriteString` because the dispatcher repaints the prompt
  immediately after `Mode.Handle` returns.
- Read-goroutine paths that mutate `Session.Input` (every keystroke
  handler in `telnet/server.go`) wrap "decide echo + mutate Input +
  emit echo" in `Session.EditAndWrite(fn)`. `fn` runs under `writeMu`
  and returns the bytes the terminal needs; the wrapper writes them
  in the same critical section. This serializes against `WriteAsync`,
  `WritePrompt`, and `listAndRedraw` so a concurrent broadcast cannot
  observe a half-mutated Input or replay a stale prompt cache.
  `InPasswordMode` writes from mode handlers go through
  `Session.SetPasswordMode(bool)` (also under `writeMu`). Top-level
  password-mode reads in `handleEscape`/`handleTab` are pre-existing
  fast-path checks that race with mode-handler writes; the inner
  EditAndWrite paths re-read under the lock so the race only widens
  the bell vs. dispatch decision by one keystroke and never corrupts
  state.
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
- `Command.Lag` (Phase E #26) is the per-verb global cooldown stamped
  on `Session.nextReady` via `s.StampLag(cmd.Lag)` after a successful
  `cmd.Run`. The gate lives in `Registry.dispatchOne` (per-segment,
  NOT in `Dispatch`) so chained `;` inputs gate independently —
  `look; attack bob` runs `look` even when a prior segment lagged.
  Refuse-with-message V1 (`{{You're too busy. (~Ns)}}::yellow`);
  promotion to a bounded queue is a single dispatcher swap on the
  same wire shape. Stamp on success only — failing `cmd.Run`
  leaves the session unlagged. Wired V1 verbs: combat
  (`attack`/`kill`=3s, `flee`=2s, `parry`=1s), zone broadcast
  (`shout`/`yell`=2s). Movement and say/tell lag deferred.
- `Registry.Dispatch` is segment-aware: a top-level `;` outside quotes
  splits the input into multiple commands run in order via
  `dispatchOne`. `telnet.SplitOnSemicolon` mirrors `Tokenize`'s
  quote/escape rules, so commands consuming `c.Raw` (e.g. `say`,
  `tell`, `shout`) get the same `Raw` they would have without
  chaining — `say "hello; world"` stays one command. Lookup errors
  and Run errors don't abort the chain; the first Run error is
  returned. Hard cap `maxSegmentsPerLine = 16`; alias expansions that
  themselves introduce `;` are bounded at `maxAliasDepth = 3`.
- Logging uses `slog`; level is set in `main.go` from `LOG_LEVEL`.
- Spawn long-lived goroutines via `safego.Go("name", fn)` so panics
  surface as warnings instead of taking down the process.
- The `shutdown` / `reboot` admin verbs (cmd/server/main.go::Request*)
  drive teardown by calling the same `stop` cancel that
  `signal.NotifyContext` returns; the existing shutdown-watcher
  goroutine then closes the listener and `srv.shutdown()` runs the
  drain + `persist.FlushAll`. `reboot` flips `srv.rebootOnExit`
  before triggering, and `main()` ends with `syscall.Exec(os.Args[0],
  os.Args, os.Environ())` — POSIX-only. The countdown goroutine
  broadcasts via `Session.WriteAsync` (cross-session output rule)
  and is interruptible via `RequestAbort`.
- Privileged verbs (`spawn`, `teleport`, `goto`, `transfer`,
  `summon`, `wizinvis`, `shutdown`, `reboot`) record one
  `admin_audit` row per successful invocation via
  `internal/audit.Record(c.Ctx, audits, c.Session, verb, target,
  args)`. Refusal paths (auth denied, bad target, NoTeleport,
  controller error, unknown template) MUST NOT audit — the row
  represents "this side effect actually happened." Synchronous
  by design so a `shutdown` row commits before drain begins. New
  admin verbs follow the same pattern: thread `audits
  repo.AdminAuditRepo` into the factory, call `audit.Record`
  immediately after the side effect lands, and pass `nil` from
  tests that don't care about the audit assertion.
- New columns on `characters` need to land in BOTH `charPlayerColumns`
  AND `charPlayerValues` AND `charPlayerScanDest` in lock-step
  (`internal/repo/character_sql.go`); ordering is load-bearing.
  The most recent example is `skill_cooldowns_json` (0047, slotted
  between `xp_debt` and `auth_level`); the four `pending_*` int32
  columns (0039, between `pvp` and `auth_level`) were the previous
  one. JSON columns also need a `characterJSON` field plus
  marshal/unmarshal lines in
  `character_sqlite.go::marshalCharacterJSON` /
  `(characterJSON).unmarshalInto`. The `auth_level` column MUST stay
  the very last entry in all three lists — the SQLite first-character
  bootstrap CASE expression in `Create` consumes it as the trailing
  placeholder; new columns belong before it.
- Progression spend verbs (`learn`, `feat`, `bump`, `learn weave`)
  follow the §E #24 pattern: a per-verb repo method named `RecordX`
  that takes the absolute new pending value and the per-pool entry
  to upsert (mirroring `RecordCoin` / `RecordXP` rather than
  widening `RecordLevelUp`). Cmd-layer computes the cap + budget
  guards before the call; refusals do NOT mutate or audit; success
  writes one `audit.Record(verb=X, target=<id>, args=<n>)` row.
  Catalog string ids that need int32 keys go through
  `chargen.HashID(id)` so chargen-persisted entries round-trip.
  All four pending pools deposited by `RecordLevelUp` now have a
  drain (#24 + #25). `RecordWeavePick` is the one repo method that
  returns a non-`ErrCharacterNotFound` typed error (`ErrNotChanneler`)
  as defense in depth — the verb layer already refuses non-channelers
  with `Character.Channeling == nil`. New ability-bump verbs use the
  `repo.AbilityKey` enum (Str/Dex/Con/Int/Wis/Cha) to select which
  `*_cur` column SQLite updates; the column lookup is from a fixed
  allow-list (`abilityCurColumn`), no SQL injection surface despite
  the format-string assembly.
- AuthLevel lives on the character row, not the account. The session
  stays at AuthGuest through login + account-create; it's stamped by
  `mode/postauth.promoteToGame` from `Character.AuthLevel` once a
  character is selected. `CharacterRepo.Create` atomically promotes
  the very first character on the server to AuthAdmin so a fresh
  deploy has a working operator without manual SQL.
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
- New columns on `exits` need to land in `exitSelectCols`,
  `scanExitInto`, the `Create` INSERT in
  `internal/repo/exit_sqlite.go`, AND the loader-side INSERT in
  `internal/world/loader.go::insertExits` (raw SQL, same
  single-transaction pattern as rooms/items). The most recent
  example is the `authored_closed` / `authored_locked` pair
  (0048).
- Items live in exactly one of three locations: `room_id` (on the
  floor), `owner_character_id` (in someone's inventory), or
  `parent_item_id` (inside another item — i.e. a container).
  `ItemRepo.SetOwner` / `SetRoom` and the `Transfer*` family flip
  the relevant columns atomically and clear the other two; do not
  write the columns directly. The `Transfer*` variants (preferred
  from the command layer) also guard on prior location so a
  concurrent `get`/`give`/`put` race surfaces as `ErrItemMoved`
  instead of a silent overwrite. `Character.Inventory` (JSON id
  list on `inventory_json`) is just the display ordering — SQL
  `owner_character_id` is the source of truth, and `inventory.go::
  orderInventory` self-heals stale or missing JSON entries. Items
  inside containers are NOT in `inventory_json`; encumbrance reads
  them via `ListAllOwnedTransitive` (BFS through `parent_item_id`).
- Cross-session output from a command (broadcasts to other
  occupants of the room or zone — `say`, `shout`/`yell`, `give`,
  `put`, `get`, door verbs, `spawn`) MUST go through
  `Session.WriteAsync`; only the dispatcher's reply to its own
  session uses `WriteString`. See the WriteAsync rule above.

## Tests

`go test -race ./...` covers the registry, mode dispatcher, completion
handler, IAC parser, color helpers, word wrap, tokenizer, line editor,
alias table, every repo (memory + sqlite, including ZoneRepo,
MobTemplateRepo, mob_trails, news, ShopRepo, BankerRepo, TrainerRepo,
CharacterRepo's coin_version optimistic-lock contract +
RecordLevelUp pending-pool accumulation + RecordSkillRank atomic
upsert), the world loader (zone metadata, room.zone_id linkage,
item taxonomy, container fixtures, dark-room fixtures, shop
round-trip + invalid-stock-item rejection, banker round-trip +
bad-hour rejection), the session registry, the eventbus, the tick
scheduler, the persist manager, the world Restocker, the
`internal/combat` package (initiative, attack/damage resolution,
threat tables, group XP split), the `internal/group` party manager,
the `internal/progression` curve + level-up math, and the concrete
commands (look / move / say / tell / reply / shout / yell / channel
/ teleport / alias / prompt / examine / door verbs / inventory
verbs / put / equipment verbs /
shop verbs (list/buy/sell/value) /
banker verbs (balance/deposit/withdraw) /
attack / pvp / group / follow / unfollow / score / xp / train /
learn / spawn / map / zonemap / coords / track / time / news /
whereami / zones).
Telnet-package tests reuse `newPipeSession(t)` / `bufSession(t)` /
`bufConn` from `telnet/command_test.go`. Cmd-package tests reuse
`commPair` / `runCmd` from `internal/cmd/comm_test.go`.

## Module

`github.com/Jasrags/WheelMUD`. Direct deps: `github.com/i582/cfmt`
(styling), `golang.org/x/crypto` (bcrypt), `gopkg.in/yaml.v3` (world
loader), `modernc.org/sqlite` (pure-Go SQLite). See
`docs/CODEMAPS/dependencies.md` for the full picture.
