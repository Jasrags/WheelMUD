<!-- Generated: 2026-05-02 | Files scanned: internal/db/*, internal/repo/*, internal/auth/*, internal/world/*, internal/creature/*, migrations | Token estimate: ~900 -->

# Data

SQLite-backed persistence via pure-Go `modernc.org/sqlite` (no CGO). 11 migrations (0001-0011) are embedded and applied at boot. `cmd/server/main.go` opens the DB, runs migrations, loads channel catalog from DB, populates world tables via `internal/world.LoadAndSync` (YAML → SQL), and constructs the SQLite-backed repos (accounts, characters, rooms, exits, items, mob_templates, mob_instances, channeling, channels) that modes and commands consume.

## Layers

```
┌────────────────────────────────────────────────────────────────┐
│ Modes + commands (internal/mode, internal/cmd)         depend on│
├────────────────────────────────────────────────────────────────┤  repo
│ internal/repo/  AccountRepo    CharacterRepo  RoomRepo        │ *interfaces*
│                 ExitRepo       ItemRepo       MobTemplate      │ (Creature
│                 MobInstance    Channeling     Channel          │  /Currency
├────────────────────────────────────────────────────────────────┤  models)
│ SQLite{Account,Character,Room,Exit,Item,MobTemplate,MobInstance,
│   Channeling,Channel}Repo  (prod)                             │
│ Memory{Account,Character,Room,Exit,Item,MobTemplate,MobInstance,
│   Channeling,Channel}Repo  (tests)                            │
├────────────────────────────────────────────────────────────────┤
│ internal/creature/     Core, Abilities, Channeling models     │
│ internal/currency/     Amount type                            │
├────────────────────────────────────────────────────────────────┤
│ internal/db.Open / Migrate              *sql.DB + 11 migrations│
├────────────────────────────────────────────────────────────────┤
│ modernc.org/sqlite                      driver                │
└────────────────────────────────────────────────────────────────┘
```

## Migrations (0001-0011)

| Migration | Purpose |
|---|---|
| `0001_create_accounts.sql` | accounts table + lockout index |
| `0002_create_characters.sql` | characters + FK cascade to accounts |
| `0003_create_world.sql` | rooms / exits / items / legacy mobs (flat) |
| `0004_seed_starter_zone.sql` | 3-room demo zone (wipe + external_id in 0006) |
| `0005_add_character_room.sql` | `characters.current_room_id` (no FK) |
| `0006_world_external_id.sql` | Add `external_id` columns to rooms/items/mobs, wipe seed |
| `0007_widen_exit_directions.sql` | Widen direction CHECK (n/s/e/w/u/d → +ne/nw/se/sw) |
| `0008_create_creatures.sql` | mob_templates, mob_instances, polymorphic channeling table |
| `0009_extend_characters.sql` | Add Full Core + player columns to characters (race/background/class/xp/coin/etc.) |
| `0010_drop_legacy_mobs.sql` | Drop legacy flat mobs table (world loader now spawns from templates) |
| `0011_create_channels.sql` | channels table (id, name, description), seeded with ooc/gossip/newbie |

Runner (`internal/db/db.go::Migrate`):
- Ensures `schema_migrations(version, applied_at)` exists.
- Loads applied versions, sorts files lexically, applies unrecorded ones (one tx per migration).
- Idempotent — safe to call repeatedly.

Pragmas set on `Open`: `foreign_keys=ON`, `journal_mode=WAL`, `synchronous=NORMAL`, `busy_timeout=5000`.

## Tables

| Table | Migration | Columns |
|---|---|---|
| `schema_migrations` | (bootstrap) | `version PK, applied_at` |
| `accounts` | 0001 | `id, username, username_lower (unique), password_hash, created_at, last_login_at, failed_login_count, locked_until` + partial index on `locked_until` |
| `characters` | 0002/0005/0009 | `id, account_id FK, name, name_lower (unique), created_at, last_played_at, current_room_id` + Core (str/dex/con/int/wis/cha, hp, defense, etc.) + player (race/background/class_levels JSON/xp/coin/bank/stance/fame/infamy/etc.) |
| `rooms` | 0003/0006 | `id, external_id (unique), zone, name, short_desc, long_desc, created_at` |
| `exits` | 0003/0007 | `id, from_room_id FK, to_room_id FK, direction CHECK (n/s/e/w/u/d/ne/nw/se/sw)`, unique `(from_room_id, direction)` |
| `items` | 0003/0006 | `id, external_id (unique), name, name_lower, short_desc, room_id FK nullable, created_at` |
| `mob_templates` | 0008 | `id, external_id (unique), name, name_lower, short_desc, created_at` + Core stat block + mob-specific fields |
| `mob_instances` | 0008 | `id, template_id FK, room_id FK nullable, created_at` (instance state separate from archetype) |
| `channeling` | 0008 | `owner_kind (enum: 'mob_template'/'mob_instance'/'character'), owner_id, gender_source, channeler_type, affinity JSON, talents JSON, weaves_known JSON, slots_per_level JSON, embraced, madness, stilled, bonded_warder_id, bonded_aes_sedai_id, held_angreal_id, held_saangreal_id, circle_id, aes_sedai_oaths, damane_collar_to` — polymorphic via `(owner_kind, owner_id)` |
| `channels` | 0011 | `id, name (unique), description` — seeded with ooc/gossip/newbie |

**World data flow**: `0004_seed_starter_zone.sql` was the original SQL seed for the 3-room demo zone. `0006_world_external_id.sql` wipes that seed, adds `external_id` columns, and resets the autoincrement sequences. The runtime now populates the world via `internal/world.LoadAndSync` reading YAML — see "World loader" below. Room id `1` is still the starter (`repo.StarterRoomID`); the loader pins the YAML room flagged `starter: true` to that id explicitly.

**FK on `characters.current_room_id`** is enforced at the application layer, not the DB. SQLite forbids `ALTER TABLE ADD COLUMN ... REFERENCES` with a non-NULL default while `foreign_keys=ON`, so the column was added without a `REFERENCES rooms(id)` clause. A future table-rebuild migration can promote it to a true FK.

## Auth (`internal/auth`)

```
Hash(password) → string, error    bcrypt at DefaultCost (10)
                                  enforces 8-rune min / 72-byte max
                                  errors: ErrPasswordTooShort, ErrPasswordTooLong
Verify(hash, password) → bool     bcrypt.CompareHashAndPassword wrapper;
                                  rejects empty / oversized inputs early
SetCost(c) → previous int         test-only knob; tests run at MinCost
```

`accounts.password_hash` stores the bcrypt output verbatim. `auth` is the only package that calls bcrypt; login + create modes consume `Hash` / `Verify`.

## AccountRepo

Interface (`internal/repo/account.go`):

```
Create(ctx, Account)             → Account, error  (ErrDuplicateUsername on conflict)
FindByUsername(ctx, username)    → Account, error  (case-insensitive; ErrAccountNotFound)
RecordLoginSuccess(ctx, id, t)   → error           (clears fail counter + locked_until)
RecordLoginFailure(ctx, id, t)   → error           (bump counter; t=zero leaves lockout alone)

Account.IsLockedAt(t) → bool     true while LockedUntil > t; login mode
                                 calls this BEFORE bcrypt verify so a
                                 known-locked account doesn't burn CPU
```

Two implementations:
- `SQLiteAccountRepo` (`account_sqlite.go`) — wraps `*sql.DB`. Detects unique violations by string match on the driver error since modernc/sqlite doesn't expose typed codes.
- `MemoryAccountRepo` (`account_memory.go`) — concurrent-safe map keyed on `username_lower`. For tests; never used at runtime.

A shared contract test (`account_test.go::runAccountRepoTests`) exercises both impls so the in-memory fake stays a faithful stand-in.

## World loader (`internal/world`)

YAML zone files are the source of truth for rooms, exits, items, and mobs; the SQL tables are a derived runtime cache. The loader runs once on boot before the command registry is built.

```
internal/world/
├── default/                 # //go:embed all:default — bundled into binary
│   └── starter/
│       ├── zone.yaml        # zone metadata (id, name)
│       ├── rooms.yaml       # room list with embedded `exits` map
│       ├── items.yaml       # optional
│       └── mobs.yaml        # optional
├── embed.go                 # SourceFS() — embedded default or WORLD_DIR override
├── yaml.go                  # struct decoders, line-number annotation
├── validate.go              # cross-reference checks (fail-fast)
└── loader.go                # LoadAndSync(ctx, db, fs.FS): parse → validate → tx insert
```

`SourceFS()` returns the embedded `default/` subtree unless `WORLD_DIR` is set, in which case it returns `os.DirFS($WORLD_DIR)` so builders can iterate without rebuilding the binary.

**Pipeline:**
1. Probe — `SELECT EXISTS(SELECT 1 FROM rooms)`. If true, skip (boot-time only).
2. Walk for `*/zone.yaml`. Each match defines a zone. Missing items.yaml / mobs.yaml is OK; missing rooms.yaml or zone.yaml is an error.
3. Validate strictly: unique external IDs per kind, exactly one starter room, all exit targets / item.room / mob.room references resolve, valid direction codes (`n/s/e/w/u/d`).
4. Insert in one transaction: starter room first with `id = repo.StarterRoomID`, remaining rooms autoincrement, exits + items + mobs use a string→int id map built during the room phase.
5. A failed step rolls back the whole load — no partial worlds.

Errors are formatted with `file:line` so `data/world/starter/rooms.yaml:42: …` jumps a builder straight to the offending node.

## CharacterRepo

Interface (`internal/repo/character.go`):

```
Create(ctx, Character)                      → Character, error
FindByName(ctx, name)                       → Character, error  (case-insensitive)
ListByAccount(ctx, accountID)               → []Character, error
RecordPlay(ctx, id, t)                      → error  (updates last_played_at on autosave pulse)
RecordRoom(ctx, id, roomID)                 → error  (write-through on every move)
RecordCore(ctx, id, core)                   → error  (combat HP / condition updates)
RecordChannelSettings(ctx, id, settings)    → error  (channel_settings_json blob)
```

SQLite + Memory impls; shared contract test exercises both. `RecordCore` is write-through from combat; `RecordChannelSettings` syncs mute toggles from `Session.channelMuted` (crossMu-guarded bitmask).

## World repos

```
RoomRepo.FindByID(ctx, id)                   → Room, error
RoomRepo.Create(ctx, Room)                   → Room, error

ExitRepo.ListFrom(ctx, fromRoomID)           → []Exit, error  (sorted by direction)
ExitRepo.FindByDirection(ctx, fromRoomID, direction) → Exit, error
ExitRepo.Create(ctx, Exit)                   → Exit, error

ItemRepo.ListInRoom(ctx, roomID)             → []Item, error
ItemRepo.Create(ctx, Item)                   → Item, error

MobTemplateRepo.FindByID(ctx, id)            → MobTemplate, error
MobTemplateRepo.Create(ctx, MobTemplate)     → MobTemplate, error

MobInstanceRepo.ListInRoom(ctx, roomID)      → []MobInstance, error
MobInstanceRepo.FindByID(ctx, id)            → MobInstance, error
MobInstanceRepo.Create(ctx, MobInstance)     → MobInstance, error

ChannelingRepo.FindByOwner(ctx, kind, ownerID) → Channeling, error
ChannelingRepo.Upsert(ctx, Channeling)       → error

ChannelRepo.List(ctx)                        → []Channel, error  (catalog load at boot)
ChannelRepo.FindByName(ctx, name)            → Channel, error
```

Direction codes (single-char: `n/s/e/w/u/d/ne/nw/se/sw`) match DB CHECK. Commands translate long names (`north`) at boundary. `ChannelingRepo` is polymorphic — `(owner_kind, owner_id)` keys can be mob_template / mob_instance / character. YAML loader runs raw SQL inserts in one tx so all world kinds land atomically; `Create` covers test + future OLC edits.

## Login flow (where this layer is consumed)

```
Login.handleUsername(ctx, s, line)
  username == "new"? → ReplaceMode(NewCreate(...))
  accounts.FindByUsername → cache l.account (or nil if not found)
  s.InPasswordMode = true; advance to password step

Login.handlePassword(ctx, s, line)
  re-fetch account (lockout TOCTOU defense)
  account == nil? → uniform "Login failed."
  IsLockedAt(now)? → "Account temporarily locked." + reset
  auth.Verify(hash, line)? no:
    accounts.RecordLoginFailure(id, lockoutMaybe)
    if failure_count >= LockoutThreshold: set locked_until = now + 15m
  yes:
    accounts.RecordLoginSuccess(id, now)  // resets counters
    s.AuthLevel = AuthPlayer
    ReplaceMode(next)  // typically Game
```

Create mode follows the same shape but goes username → Hash(password) → confirm → `accounts.Create`. `auth.Hash` enforces the password length rules; mode handles the duplicate / mismatch retries.

## In-process state

| Owner | Lifetime | Notes |
|---|---|---|
| `*sql.DB` | server lifetime | Opened in `main`; single pool shared by all sessions |
| `repo.AccountRepo` | server lifetime | Wrapped on `*sql.DB`; held on `server.accounts` |
| `telnet.Session` (per conn) | accept → disconnect | InputBuffer, inbox, mode stack, AuthLevel, InPasswordMode |
| `*Login` mode (per conn) | login → success/teardown | Per-connection — built fresh by `srv.newInitial()` factory |
| `telnet.Registry` | server lifetime | Built once, read-only across sessions |

## Creature & Currency models

**`internal/creature/creature.go`** defines the stat block shared by mobs and characters:
- `Core` — base attributes (name, size, type, gender, alignment), ability scores (Str/Dex/Con/Int/Wis/Cha), HP, defenses, saves, speed, movement types.
- `Abilities` — individual ability with current/max/inherent (for drain vs. damage tracking).
- `Affect`, `StatMod` — buff/debuff stubs for §12 (duration ticks, effects).
- `Equipment` — slot enum + equipment tracking (wield, wear).
- `MobTemplate`, `MobInstance`, `Character` — type skeletons landed. Character extends mob with PC-specific fields (account_id FK, class_levels JSON, race/background, XP, coin, fame/infamy, quest log, etc.) — all column families from 0009.

**`internal/currency/amount.go`** — Amount type:
- Four denominations at fixed ratios (1 cp / 10 sp / 100 mk / 1000 gc).
- `New(gc, mk, sp, cp) Amount`, `Parse("1gc 2mk ...")`, `Format()` (greedy largest-first), `In(coin)`.
- `Add` / `Sub` with overflow guards. Stored as signed copper pennies on `characters.coin` column.

## Pending

- Hot-reload of world YAML without restart (soft-delete via `deleted_at` flag, character relocation on room delete).
- Item/mob polymorphic inventory (creature inventory vs. room floor vs. containers).
- Dirty-bit autosave for combat HP / mob state / affect tick counters (rooms/items/character core already write-through).
- True FK on `characters.current_room_id` (table-rebuild migration).
- CHECK constraints on accounts (username length / charset rules).
- Down-migration / rollback path.
- Backup / VACUUM / WAL checkpoint automation.
