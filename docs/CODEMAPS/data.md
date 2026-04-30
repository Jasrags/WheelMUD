<!-- Generated: 2026-04-30 | Files scanned: internal/db/*, internal/repo/*, internal/auth/*, migrations | Token estimate: ~450 -->

# Data

SQLite-backed persistence via pure-Go `modernc.org/sqlite` (no CGO). Migrations are embedded into the binary and applied at boot. `cmd/server/main.go` opens the DB on startup, runs `Migrate`, and constructs a `SQLiteAccountRepo` that login + account-create modes consume.

## Layers

```
┌────────────────────────────────────────────┐
│ Login + Create modes (internal/mode)       │  consumers depend on the
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

- `CharacterRepo` once a character model exists (1:N to accounts).
- World aggregates (rooms / items / mobs) when the world model lands.
- CHECK constraints on `accounts` (length / charset) — see `persistence_followups.md` item 4.
- Down-migration / rollback path — see `persistence_followups.md` item 1.
- Backup / vacuum / WAL checkpoint policy.
