<!-- Generated: 2026-04-30 | Files scanned: internal/db/*, internal/repo/*, internal/auth/*, internal/world/*, migrations | Token estimate: ~700 -->

# Data

SQLite-backed persistence via pure-Go `modernc.org/sqlite` (no CGO). Migrations are embedded into the binary and applied at boot. `cmd/server/main.go` opens the DB on startup, runs `Migrate`, populates the world tables via `internal/world.LoadAndSync` (YAML → SQL), and constructs the SQLite-backed repos (accounts, characters, rooms, exits, items, mobs) that the modes and commands consume.

## Layers

```
┌────────────────────────────────────────────────────────────┐
│ Modes + commands (internal/mode, internal/cmd)             │  depend on
├────────────────────────────────────────────────────────────┤   the repo
│ internal/repo/  AccountRepo │ CharacterRepo │ RoomRepo     │  *interfaces*
│                 ExitRepo    │ ItemRepo      │ MobRepo      │
├────────────────────────────────────────────────────────────┤
│ SQLite{Account,Character,Room,Exit,Item,Mob}Repo  (prod)   │
│ Memory{Account,Character,Room,Exit,Item,Mob}Repo  (tests)  │
├────────────────────────────────────────────────────────────┤
│ internal/db.Open / Migrate                  *sql.DB + SQL  │
├────────────────────────────────────────────────────────────┤
│ modernc.org/sqlite                          driver         │
└────────────────────────────────────────────────────────────┘
```

## Migrations

Location: `internal/db/migrations/NNNN_description.sql`, embedded via `//go:embed all:migrations`.

Runner (`internal/db/db.go::Migrate`):
- Ensures `schema_migrations(version, applied_at)` exists.
- Loads applied versions, sorts files lexically, applies any unrecorded ones inside a single transaction each (DDL + the `INSERT INTO schema_migrations`).
- Idempotent — `Migrate` is safe to call repeatedly.

Pragmas set on every `Open`: `foreign_keys=ON`, `journal_mode=WAL`, `synchronous=NORMAL`, `busy_timeout=5000`.

## Tables

| Table | Migration | Columns |
|---|---|---|
| `schema_migrations` | (bootstrap) | `version PK, applied_at` |
| `accounts` | `0001_create_accounts.sql` | `id, username, username_lower (unique), password_hash, created_at, last_login_at, failed_login_count, locked_until` + partial index on `locked_until` |
| `characters` | `0002_create_characters.sql` (+ `0005_add_character_room.sql` for `current_room_id`) | `id, account_id (FK accounts.id ON DELETE CASCADE), name, name_lower (unique), created_at, last_played_at, current_room_id` (defaults to `1` = starter room) |
| `rooms` | `0003_create_world.sql` (+ `0006_world_external_id.sql` for `external_id`) | `id, external_id (unique), name, short_desc, long_desc, created_at` |
| `exits` | `0003_create_world.sql` | `id, from_room_id (FK rooms ON DELETE CASCADE), to_room_id (FK rooms ON DELETE CASCADE), direction CHECK in (n/s/e/w/u/d)`, unique `(from_room_id, direction)` |
| `items` | `0003_create_world.sql` (+ `0006_world_external_id.sql` for `external_id`) | `id, external_id (unique), name, name_lower, short_desc, room_id (nullable, FK rooms ON DELETE SET NULL), created_at` |
| `mobs` | `0003_create_world.sql` (+ `0006_world_external_id.sql` for `external_id`) | `id, external_id (unique), name, name_lower, short_desc, room_id (nullable, FK rooms ON DELETE SET NULL), created_at` |

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
Create(ctx, Character)              → Character, error  (defaults CurrentRoomID to StarterRoomID)
FindByName(ctx, name)               → Character, error  (case-insensitive)
ListByAccount(ctx, accountID)       → []Character, error (recent-first, then name)
RecordPlay(ctx, id, t)              → error             (last_played_at)
RecordRoom(ctx, id, roomID)         → error             (current_room_id; called on every move)
```

`SQLiteCharacterRepo` + `MemoryCharacterRepo`; shared contract test in `character_test.go`.

## World repos (RoomRepo / ExitRepo / ItemRepo / MobRepo)

```
RoomRepo.FindByID(ctx, id)                              → Room, error  (ErrRoomNotFound)
RoomRepo.FindByExternalID(ctx, externalID)              → Room, error  (ErrRoomNotFound)
RoomRepo.Create(ctx, Room)                              → Room, error  (ErrInvalidExternalID, ErrDuplicateExternalID)
ExitRepo.ListFrom(ctx, fromRoomID)                      → []Exit, error (sorted by direction)
ExitRepo.FindByDirection(ctx, fromRoomID, dir)          → Exit, error  (ErrExitNotFound)
ExitRepo.Create(ctx, Exit)                              → Exit, error  (ErrDuplicateExit)
ItemRepo.ListInRoom(ctx, roomID)                        → []Item, error (sorted by name_lower)
ItemRepo.Create(ctx, Item)                              → Item, error  (sentinels above)
MobRepo.ListInRoom(ctx, roomID)                         → []Mob, error
MobRepo.Create(ctx, Mob)                                → Mob, error
```

Direction codes (`repo.DirNorth..DirDown`) are single-byte strings (`n/s/e/w/u/d`) matching the DB CHECK constraint. The `look`/`move` commands translate the long names (`north`, ...) at the boundary.

`SQLite*Repo` and `Memory*Repo` for each of the four. Memory variants expose `Insert(...)` for test fixtures (skips ExternalID validation); production code (the YAML loader's transaction path) uses raw SQL inserts inside its own tx rather than going through `Create` so that all four kinds land atomically. The repo `Create` methods cover ad-hoc insertion (e.g. via tests of `Create` itself) and will service future authoring paths like OLC.

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

## Pending

- Hot-reload of YAML zones without a DB wipe + restart — needs reset semantics (what to do with characters in deleted rooms, items moved between zones, etc.).
- Periodic + shutdown autosave (player state — characters' rooms today, inventory + xp later).
- True FK on `characters.current_room_id` — needs a table-rebuild migration; see comment in `0005_add_character_room.sql`.
- Item/mob template-vs-instance split — only minimal placeholder schemas today; lifecycle (spawn/despawn, inventory ownership) lands with §11/§14.
- CHECK constraints on `accounts` (length / charset) — see `persistence_followups.md` item 4.
- Down-migration / rollback path — see `persistence_followups.md` item 1.
- Backup / vacuum / WAL checkpoint policy.
