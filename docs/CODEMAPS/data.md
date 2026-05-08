<!-- Generated: 2026-05-08 | Updated for Phase D #19: death/respawn/XP-debt | Token estimate: ~1600 -->

# Data

SQLite-backed persistence via pure-Go `modernc.org/sqlite` (no CGO). 41
embedded migrations applied at boot. `cmd/server/main.go` opens the DB,
runs migrations, loads YAML catalogs (chargen / news / help), populates
world tables via `internal/world.LoadAndSync`, and constructs the
SQLite-backed repos that modes and commands consume.

## Layers

```
┌────────────────────────────────────────────────────────────────────┐
│ Modes + commands (internal/mode, internal/cmd, internal/combat,    │
│                   internal/progression, internal/group)            │
├────────────────────────────────────────────────────────────────────┤  repo
│ internal/repo/  Account, AccountLogin, Character (incl. Level/Skill│ *interfaces*
│                 Coin Pvp etc), Room, Exit, Item, MobTemplate,      │
│                 MobInstance, MobTrail, Zone, Channel, Channeling,   │
│                 Shop+ShopStock, Banker, Trainer, AdminAudit,       │
│                 WorldState                                          │
├────────────────────────────────────────────────────────────────────┤
│ SQLite{...}Repo (prod) + Memory{...}Repo (tests, shared contract)  │
├────────────────────────────────────────────────────────────────────┤
│ internal/creature/  Core, Abilities, Equipment, Channeling,         │
│                     SkillRanks, MobTemplate/MobInstance models      │
│ internal/currency/  Amount type                                     │
├────────────────────────────────────────────────────────────────────┤
│ internal/db.Open / Migrate           *sql.DB + 39 embedded migrations│
├────────────────────────────────────────────────────────────────────┤
│ modernc.org/sqlite                   pure-Go driver                 │
└────────────────────────────────────────────────────────────────────┘
```

## Migrations (0001-0041)

| # | File | Purpose |
|---|---|---|
| 0001 | `create_accounts` | accounts table + lockout index |
| 0002 | `create_characters` | characters + FK cascade to accounts |
| 0003 | `create_world` | rooms / exits / items / legacy mobs (flat) |
| 0004 | `seed_starter_zone` | 3-room demo seed (wiped in 0006) |
| 0005 | `add_character_room` | `characters.current_room_id` |
| 0006 | `world_external_id` | `external_id` columns + reset autoincrement |
| 0007 | `widen_exit_directions` | n/s/e/w/u/d → +ne/nw/se/sw |
| 0008 | `create_creatures` | mob_templates, mob_instances, polymorphic channeling |
| 0009 | `extend_characters` | full Core + race/background/class/xp/coin/etc. |
| 0010 | `drop_legacy_mobs` | drop pre-template mobs table |
| 0011 | `create_channels` | channels catalog (ooc/gossip/newbie) |
| 0012 | `room_flags_and_sector` | rooms.flags + rooms.sector |
| 0013 | `room_extra_descs` | rooms.extra_descs JSON |
| 0014 | `exit_door_flags` | exit door flags + key + lock difficulty |
| 0015 | `item_taxonomy` | typed item columns (type/weight/value/quality/flags + typed stats) |
| 0016 | `create_zones` | zones table + rooms.zone_id (soft FK) |
| 0017 | `item_owner` | items.owner_character_id (soft FK) — location invariant |
| 0018 | `account_auth_level` | (transient) auth_level on accounts |
| 0019 | `character_auth_level` | moved auth_level to characters; dropped from accounts |
| 0020 | `room_nomap` | rooms.nomap (hide from minimap) |
| 0021 | `create_mob_trails` | per-mob (room_id, ts) for `track` |
| 0022 | `mob_template_wander` | mob_templates.wander_chance |
| 0023 | `character_prompt_template` | per-character `prompt` override |
| 0024 | `world_state` | key/value table (`world.ticks` for Clock) |
| 0025 | `room_sector_extension` | widened sector enum (blight/waste/stedding/swamp) |
| 0026 | `room_coords_auto` | rooms.coords_auto (1=BFS-derived, 0=anchor) |
| 0027 | `character_last_news_seen` | MOTD/news watermark |
| 0028 | `item_parent` | items.parent_item_id — completes location invariant (room ⊕ owner ⊕ parent) |
| 0029 | `create_admin_audit` | append-only forensic log for privileged verbs |
| 0030 | `create_shops` | shops + shop_stock (sentinel `qty=-1` infinite) |
| 0031 | `create_bankers` | bankers (operating hours; coin via RecordCoin) |
| 0032 | `character_coin_version` | optimistic-lock token; `RecordCoin(expectedVersion)` returns `ErrCoinConflict` |
| 0033 | `characters_channeling_json` | channeler chargen branch persistence (`*creature.Channeling`) |
| 0034 | `admin_audit_account_actor` | extends admin_audit with actor_account_id + actor_type for account-mode rows |
| 0035 | `accounts_settings_json` | `repo.AccountSettings` (color/prompt/width/locale/MOTD-toggle) |
| 0036 | `create_account_logins` | append-only authn log (success/failure/lockout/kick) backing the §6 security menu |
| 0037 | `characters_pvp` | PvP opt-in (Phase D #21) |
| 0038 | `create_trainers` | trainers (1:1 mob_template → class id) for §E #23 |
| 0039 | `characters_pending_pools` | pending_feats / pending_skill_points / pending_ability_bumps / pending_weaves (Phase E #23 slice 4) |
| 0040 | `create_shops` | (legacy: absorbed into 0030) |
| 0041 | `characters_xp_debt` | xp_debt INT NOT NULL DEFAULT 0 — stacks on character death via `DeathDebt(curXP, curLevel) → debt` (Phase D #19) |

Runner (`internal/db/db.go::Migrate`):
- Ensures `schema_migrations(version, applied_at)` exists.
- Loads applied versions, sorts files lexically, applies unrecorded ones one tx per migration.
- Idempotent — safe to re-run.

Pragmas on `Open`: `foreign_keys=ON`, `journal_mode=WAL`,
`synchronous=NORMAL`, `busy_timeout=5000`.

## Tables (current schema)

| Table | Migrations | Notable columns |
|---|---|---|
| `schema_migrations` | bootstrap | `version PK, applied_at` |
| `accounts` | 0001/0035 | `id, username, username_lower (unique), password_hash, created_at, last_login_at, failed_login_count, locked_until, settings_json` |
| `account_logins` | 0036 | `id, account_id, ts, remote_address, outcome, info` + index on `(account_id, ts)` |
| `characters` | 0002–0041 | `id, account_id, name, name_lower (unique), created_at, last_played_at, current_room_id` + Core block (str/dex/con/int/wis/cha + str_cur/dex_cur/con_cur/int_cur/wis_cur/cha_cur + hp/hp_max/etc.) + race/background/class_levels_json/xp/coin/coin_version/bank/feats_json/skills_json/equipment_json/inventory_json/channel_settings_json/channeling_json/prompt_template/last_news_seen/pvp/pending_feats/pending_skill_points/pending_ability_bumps/pending_weaves/xp_debt/auth_level. **Write sites:** chargen (abilities/feats/skills/equipment), `RecordLevelUp` (class_levels + hp/bab/saves + pending deltas), `RecordAbilityBump` (*_cur columns), `RecordFeatPick` (feats_json + pending_feats), `RecordSkillRank` (skills_json + pending_skill_points), `RecordWeavePick` (channeling_json + pending_weaves), `RecordXPDebt` (xp_debt via character death). **Lock-step:** `charPlayerColumns`/`charPlayerValues`/`charPlayerScanDest` in `internal/repo/character_sql.go` must move together; `auth_level` is the trailing column for the SQLite first-character bootstrap CASE. |
| `rooms` | 0003/0006/0012/0013/0016/0020/0025/0026 | `id, external_id, zone_id, name, short_desc, long_desc, flags, sector, light_level, extra_descs_json, nomap, coords_auto, coord_x, coord_y, coord_z` |
| `exits` | 0003/0007/0014 | `id, from_room_id, to_room_id, direction CHECK, door_flags, key_external_id, lock_difficulty, description` |
| `items` | 0003/0006/0015/0017/0028 | `id, external_id, name, name_lower, short_desc, room_id, owner_character_id, parent_item_id, type, weight, value, quality, flags, stats_json` — **location invariant:** exactly one of (`room_id`, `owner_character_id`, `parent_item_id`) is non-null |
| `zones` | 0016 | `id, external_id, name, builder, level_range, reset_interval_s, reset_mode, climate, ambient` |
| `mob_templates` | 0008/0022 | `id, external_id, name, name_lower, short_desc` + Core + mob fields + `wander_chance` |
| `mob_instances` | 0008 | `id, template_id, room_id, created_at` (instance state) |
| `mob_trails` | 0021 | `id, mob_id, room_id, ts` — per-mob movement history for `track` |
| `channeling` | 0008 | polymorphic via `(owner_kind, owner_id)`; gender_source / channeler_type / affinities / weaves_known / slots / madness / stilled / bonded_* — characters now also persist via `characters.channeling_json` (0033) for chargen branch |
| `channels` | 0011 | catalog seeded with ooc / gossip / newbie |
| `shops` | 0030 | `id, mob_template_id (UNIQUE), sell_markup, buy_markdown, restock_interval_s, buy_types` |
| `shop_stock` | 0030 | `(shop_id, item_external_id), qty, qty_max, last_restock_ts` — sentinel `qty=-1, qty_max=-1` is infinite |
| `bankers` | 0031 | `id, mob_template_id (UNIQUE), open_hour, close_hour` — V1 has no fees, no item vault |
| `trainers` | 0038 | `id, mob_template_id (UNIQUE), class_id` — Phase E #23 slice 2 |
| `admin_audit` | 0029/0034 | `id, ts, actor_type, actor_account_id, actor_username, character_id, character_name, verb, target, args, remote_address` — append-only |
| `world_state` | 0024 | `(key TEXT PK, value TEXT)` — currently `world.ticks` for `Clock` |

## Repos

Every repo has memory + sqlite + a shared contract test
(`<name>_test.go::run<Name>RepoTests`). Memory impls are concurrent-safe
maps for tests; never used at runtime.

```
AccountRepo            Create / FindByUsername / RecordLoginSuccess /
                       RecordLoginFailure / SetSettings / GetByID
AccountLoginRepo       Append / ListRecentByAccount
CharacterRepo          Create / FindByName / GetByID / ListByAccount /
                       RecordPlay / RecordRoom / RecordCore /
                       RecordChannelSettings / RecordInventory /
                       RecordEquipment / RecordCoin (expectedVersion) /
                       RecordXP / RecordXPDebt / RecordPromptTemplate /
                       RecordPvP / RecordLevelUp(LevelUpFields) /
                       RecordSkillRank(skillID, ranks, isClassSkill,
                                       newPending) /
                       RecordFeatPick(featID, newPending) /
                       RecordAbilityBump(abilityKey, newPending) /
                       RecordWeavePick(weaveID, newPending) /
                       MarkNewsSeen / Delete
RoomRepo               Find/Create + flag/sector/coords accessors
ExitRepo               ListFrom / FindByDirection / door state writes
ItemRepo               Find / Create / Delete / SetOwner / SetRoom /
                       Transfer* (location-guarded, ErrItemMoved on race) /
                       ListInRoom / ListOwnedBy / ListAllOwnedTransitive
MobTemplateRepo        FindByID / Create / GetByExternalID
MobInstanceRepo        Create / Delete / UpdateRoom / UpdateLive /
                       ListInRoom / FindByID
MobTrailRepo           Append / ListRecent
ZoneRepo               Find / Create / List / CountByZone
ChannelRepo            List / FindByName
ChannelingRepo         FindByOwner / Upsert (polymorphic)
ShopRepo               GetByMobTemplateID / ListStock / DecrementStock /
                       RestockSubMax (areaReset bucket)
BankerRepo             GetByMobTemplateID
TrainerRepo            Create / GetByMobTemplateID / List
AdminAuditRepo         Record / RecordAccount / List(filter)
WorldStateRepo         Get / Set (key/value store)
```

**New repo methods (Phase D #19):**
- `RecordXPDebt(ctx, id, debt int64)` — absolute-write, mirrors `RecordXP`.

**New repo errors (Phase E #25):**
- `ErrNotChanneler` — returned by `RecordWeavePick` on non-channeler characters.

**New repo types (Phase E #25):**
- `AbilityKey` enum (Str/Dex/Con/Int/Wis/Cha) with `String()` method.

### Optimistic-lock contract

`CharacterRepo.RecordCoin` takes `expectedVersion int64` and bumps
`coin_version` on success; mismatch returns `ErrCoinConflict`. Mirrors
`ItemRepo.Transfer*`'s `ErrItemMoved` shape. Verbs surface as "your
balance just changed — try again" (sell, deposit, withdraw, give); buy
logs-and-accepts because the item already shipped.

`RecordSkillRank` (Phase E #24) takes the absolute new pending balance
(no version token — single-session-per-account makes the RMW non-
exploitable today; multi-session followup tracked in
`progression_24_followups.md`).

`RecordLevelUp` takes a `repo.LevelUpFields` struct; pending-pool
deltas accumulate via `pending_x = pending_x + ?` in the same UPDATE.

## Auth (`internal/auth`)

```
Hash(password) → string, error    bcrypt at DefaultCost (10)
                                  enforces 8-rune min / 72-byte max
Verify(hash, password) → bool     CompareHashAndPassword wrapper
SetCost(c) → previous int         test-only knob; tests run at MinCost
```

`accounts.password_hash` stores the bcrypt output verbatim. `auth` is
the only package that calls bcrypt; login + create modes consume
`Hash` / `Verify`. `Account.IsLockedAt(t)` short-circuits Verify on
known-locked rows.

## World loader (`internal/world`)

YAML zone files are the source of truth for rooms / exits / items /
mobs / shops / bankers / trainers / zones; SQL tables are a derived
runtime cache. The loader runs once on boot before the registry is
built.

```
internal/world/
├── default/                    //go:embed all:default
├── embed.go                    SourceFS() — embedded or WORLD_DIR override
├── yaml.go                     decoders + line-number annotation
├── validate.go                 cross-reference checks (fail-fast)
├── loader.go                   LoadAndSync(ctx, db, fs.FS)
├── restocker.go                shop_stock refill (areaReset bucket)
└── clock.go                    in-world time (HourOfDay / day-of-year)
```

`SourceFS()` returns embedded `default/` unless `WORLD_DIR` is set, then
returns `os.DirFS($WORLD_DIR)` so builders can iterate without
rebuilding the binary.

**Pipeline:** Probe `rooms` count → walk for `*/zone.yaml` → validate
strictly (unique external IDs, exactly one starter, exit/item/mob
references resolve, valid directions, shop/banker/trainer mob_template
references valid) → insert in one transaction. Failed step rolls back
the whole load.

**Lock-step rule:** new columns on `rooms`/`items` need to land in BOTH
the `repo.Sqlite*Repo` SELECT/INSERT lists AND the loader-side raw SQL
INSERT in `loader.go::roomInsertValues` / `insertItems`. Loader writes
raw SQL inside one transaction rather than calling `Create`, so column
lists are duplicated and must move together.

## Catalogs (content, not state)

- `internal/chargen/` — backgrounds / classes / feats / skills /
  weaves / starting items YAML. Cross-references validated at
  `chargen.Load` (fails boot on any broken reference). Stamps
  `creature.Background` / `creature.Class` enums onto each entry.
  `chargen.HashID` (FNV-32a) hashes string ids → int32 keys for
  `Character.Feats` / `Character.Skills` round-trip.
- `internal/news/` — MOTD / news entries. `WriteMOTDBlock` renders
  unseen entries gated on `characters.last_news_seen` (0027).
- `internal/help/` — help-topic catalog with prefix matching.

None of the three touch SQL — pure content loaded once at boot.

## Pending

- True FK on `characters.current_room_id` (table-rebuild migration).
- Down-migration / rollback path.
- Backup / VACUUM / WAL checkpoint automation.
- CHECK constraints on accounts (username length / charset).
- Hot-reload of world YAML without restart (depends on §16 OLC).
- Optimistic-lock token on `RecordSkillRank` / `RecordLevelUp` for
  multi-session safety (`progression_24_followups.md`).
