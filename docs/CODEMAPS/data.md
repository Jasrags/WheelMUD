<!-- Generated: 2026-04-30 | Files scanned: internal/db/*, internal/repo/*, internal/auth/*, migrations | Token estimate: ~600 -->

# Data

SQLite-backed persistence via pure-Go `modernc.org/sqlite` (no CGO). Migrations are embedded into the binary and applied at boot. `cmd/server/main.go` opens the DB on startup, runs `Migrate`, and constructs the SQLite-backed repos (accounts, characters, rooms, exits, items, mobs) that the modes and commands consume.

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
| `rooms` | `0003_create_world.sql` | `id, name, short_desc, long_desc, created_at` |
| `exits` | `0003_create_world.sql` | `id, from_room_id (FK rooms ON DELETE CASCADE), to_room_id (FK rooms ON DELETE CASCADE), direction CHECK in (n/s/e/w/u/d)`, unique `(from_room_id, direction)` |
| `items` | `0003_create_world.sql` | `id, name, name_lower, short_desc, room_id (nullable, FK rooms ON DELETE SET NULL), created_at` |
| `mobs` | `0003_create_world.sql` | `id, name, name_lower, short_desc, room_id (nullable, FK rooms ON DELETE SET NULL), created_at` |

**Seed data**: `0004_seed_starter_zone.sql` inserts a 3-room starter zone (Plaza ↔ North Road ↔ South Road) with one item per room and one mob in the plaza. Room id `1` is the starter — `repo.StarterRoomID` references it from Go code.

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

All four are read-only for now. World data ships via `0004_seed_starter_zone.sql`; authoring (Create/Update) lands with the YAML-loader slice in §7 of the roadmap.

```
RoomRepo.FindByID(ctx, id)                              → Room, error  (ErrRoomNotFound)
ExitRepo.ListFrom(ctx, fromRoomID)                      → []Exit, error (sorted by direction)
ExitRepo.FindByDirection(ctx, fromRoomID, dir)          → Exit, error  (ErrExitNotFound)
ItemRepo.ListInRoom(ctx, roomID)                        → []Item, error (sorted by name_lower)
MobRepo.ListInRoom(ctx, roomID)                         → []Mob, error
```

Direction codes (`repo.DirNorth..DirDown`) are single-byte strings (`n/s/e/w/u/d`) matching the DB CHECK constraint. The `look`/`move` commands translate the long names (`north`, ...) at the boundary.

`SQLite*Repo` and `Memory*Repo` for each of the four; the memory variants expose `Insert(...)` for test fixtures (no public Create on the interface yet). Shared contract tests in `room_test.go` / `exit_test.go` / `item_test.go` / `mob_test.go`.

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

- World authoring (YAML/JSON loader, hot reload, autosave) — §7 of the roadmap.
- True FK on `characters.current_room_id` — needs a table-rebuild migration; see comment in `0005_add_character_room.sql`.
- Item/mob template-vs-instance split — only minimal placeholder schemas today; lifecycle (spawn/despawn, inventory ownership) lands with §11/§14.
- CHECK constraints on `accounts` (length / charset) — see `persistence_followups.md` item 4.
- Down-migration / rollback path — see `persistence_followups.md` item 1.
- Backup / vacuum / WAL checkpoint policy.
