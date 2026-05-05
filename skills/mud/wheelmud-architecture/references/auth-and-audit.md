# Auth and audit

## AuthLevel placement

`AuthLevel` lives on the `characters` row (migration 0019), **not** the
account. One account can own admin and player characters
side-by-side. Migration 0019 backfilled existing rows from the prior
account-level field and dropped `accounts.auth_level`.

## Promotion path

The session is `AuthGuest` through login + account-create. It's
stamped to the character's level by `mode/postauth.promoteToGame`,
which also writes `CharacterID`, `CharacterName`, `CurrentRoomID`,
and calls `Session.SetChannelMuted` from the loaded character.

## First-admin bootstrap

`CharacterRepo.Create` atomically promotes the **very first character
on the server** to `AuthAdmin`. A fresh deploy has a working operator
without manual SQL.

## Auth checks

`Registry.Dispatch` enforces `Command.Auth` against
`Session.AuthLevel`. Privilege-denied lookups return the same
"Unknown command" text as a missing verb — privileged verb names do
not leak.

## admin_audit (migration 0029)

Append-only forensic log. One row per **successful** privileged-verb
invocation, recorded synchronously by
`internal/audit.Record(c.Ctx, audits, c.Session, verb, target, args)`.

Audit-on-success rule (CRITICAL):
- ✅ Audit after the side effect lands.
- ❌ Never audit refusals (auth denied, bad target, NoTeleport,
  controller error, unknown template).

Synchronous by design so a `shutdown` row commits before drain begins.

## Adding a new admin verb

1. Set `Auth: telnet.AuthAdmin` (or appropriate level) on the command.
2. Thread `audits repo.AdminAuditRepo` into the factory.
3. Call `audit.Record(...)` immediately after the side effect lands —
   not before.
4. Refusal paths return early without auditing.
5. Tests that don't assert on audit pass `nil` for the repo.
