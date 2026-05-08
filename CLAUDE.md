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
  `internal/db.Open` (runs embedded migrations 0001–0028), constructs every
  repo (accounts, characters, rooms, exits, items, mob_instances,
  mob_templates, mob_trails, zones, channels), loads the news catalog
  (`internal/news`) and the chargen catalog (`internal/chargen`),
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
  router with a leading `weave` arg), the admin movement verbs (`goto <player|
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

- **`internal/db/migrations/`** — embedded migrations 0001–0039. Each
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

- **`internal/progression/`** — pure-function helpers for the d20
  XP curve and level-up math (Phase E #23). `XPForLevel(n)`,
  `LevelForXP(xp)`, `XPToNext(xp)` (MaxLevel=20). `ComputeLevelUp(
  ch, cat, classKey) → LevelGains` recomputes ClassLevels + HP /
  BAB / saves and the per-pool deltas (FeatDelta / SkillDelta /
  AbilityDelta / WeaveDelta) the cmd-layer hands to
  `repo.RecordLevelUp` via `repo.LevelUpFields`. No DB / no
  session — content + math.

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
  know. Also hosts
  the `Restocker` (refills sub-max `shop_stock` lines older than
  `restock_interval_s`, wired to `tick.Buckets.AreaReset` —
  5min default cadence) and the `Clock.HourOfDay()` helper backing
  the shop hour gate.

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
  The four `pending_*` int32 columns added in 0039 are the most
  recent example (slotted between `pvp` and `auth_level`). JSON
  columns also need a `characterJSON` field plus marshal/unmarshal
  lines in `character_sqlite.go::marshalCharacterJSON` /
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
