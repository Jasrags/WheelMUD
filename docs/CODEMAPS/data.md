<!-- Generated: 2026-04-30 | Files scanned: internal/db/*, internal/repo/*, migrations | Token estimate: ~400 -->

# Data

SQLite-backed persistence via pure-Go `modernc.org/sqlite` (no CGO). Migrations are embedded into the binary and applied at boot.

## Layers

```
┌────────────────────────────────────────────┐
│ login mode / future modes                  │  consumers depend on the
├────────────────────────────────────────────┤   AccountRepo *interface*
│ internal/repo/AccountRepo (interface)      │
├────────────────────────────────────────────┤
│ SQLiteAccountRepo  │  MemoryAccountRepo    │  prod + test impls
├────────────────────┴───────────────────────┤
│ internal/db.Open / Migrate                 │  *sql.DB + migrations
├────────────────────────────────────────────┤
│ modernc.org/sqlite                         │  driver
└────────────────────────────────────────────┘
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

## Auth (`internal/auth`)

```
Hash(password) → string, error    bcrypt at DefaultCost (10)
                                  enforces 8-rune min / 72-byte max
                                  errors: ErrPasswordTooShort, ErrPasswordTooLong
Verify(hash, password) → bool     bcrypt.CompareHashAndPassword wrapper;
                                  rejects empty / oversized inputs early
SetCost(c) → previous int         test-only knob; tests run at MinCost
```

`accounts.password_hash` stores the bcrypt output verbatim. `auth` is the only package that calls bcrypt; login mode and any future password-bearing flow consume `Hash` / `Verify`.

## AccountRepo

Interface (`internal/repo/account.go`):

```
Create(ctx, Account)             → Account, error  (ErrDuplicateUsername on conflict)
FindByUsername(ctx, username)    → Account, error  (case-insensitive; ErrAccountNotFound)
RecordLoginSuccess(ctx, id, t)   → error           (clears fail counter + locked_until)
RecordLoginFailure(ctx, id, t)   → error           (bump counter; t=zero leaves lockout alone)
```

Two implementations:
- `SQLiteAccountRepo` (`account_sqlite.go`) — wraps `*sql.DB`. Detects unique violations by string match on the driver error since modernc/sqlite doesn't expose typed codes.
- `MemoryAccountRepo` (`account_memory.go`) — concurrent-safe map keyed on `username_lower`. For tests; never used at runtime.

A shared contract test (`account_test.go::runAccountRepoTests`) exercises both impls so the in-memory fake stays a faithful stand-in.

## In-process state

| Owner | Lifetime | Notes |
|---|---|---|
| `*sql.DB` | server lifetime | Created in `main` (not yet wired); single connection pool shared by all sessions |
| `telnet.Session` (per conn) | accept → disconnect | InputBuffer, inbox, mode stack, AuthLevel, etc. |
| `telnet.Registry` | server lifetime | Built once, read-only across sessions |

## Pending

- DSN config in `cmd/server/main.go` (env var `DB_DSN`, default `wheelmud.db`).
- Wire the repo into login mode (next slice).
- `CharacterRepo` once a character model exists.
- World aggregates (rooms / items / mobs) when the world model lands.
- Backup / vacuum / WAL checkpoint policy.
