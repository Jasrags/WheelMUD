-- 0019_character_auth_level.sql
--
-- Moves AuthLevel from accounts to characters. One account can own
-- multiple characters at different privilege tiers (the standard MUD
-- alts pattern: an admin builder character alongside a player-tier
-- alt on the same login). Authority is a property of the persona,
-- not the login.
--
-- Three forward-only steps:
--   1. Add characters.auth_level (default AuthPlayer = 1).
--   2. Backfill: each character inherits the auth_level of its
--      owning account, so admins promoted under 0018 keep their
--      privilege without manual SQL.
--   3. Drop accounts.auth_level — the column is now redundant. SQLite
--      3.35+ supports ALTER TABLE DROP COLUMN; modernc.org/sqlite
--      v1.50.0 wraps a recent enough upstream.
--
-- After this migration runs, postauth.promoteToGame copies the chosen
-- character's auth_level onto the session; login mode no longer
-- earns privilege of its own.

-- The CHECK constraint pins auth_level to the known enum range
-- (0 = guest, 1 = player, 2 = admin) at write time. The repo also
-- validates on scan, but a CHECK at the DB layer prevents a corrupt
-- direct UPDATE (admin tooling, future code path, hand-edited row)
-- from poisoning a row in a way that would later lock a player out
-- when the repo refused to scan it.
ALTER TABLE characters ADD COLUMN auth_level INTEGER NOT NULL DEFAULT 1
    CHECK (auth_level BETWEEN 0 AND 2);

-- Backfill: each character inherits the auth_level of its owning
-- account. COALESCE defends against orphaned characters where the
-- account row was deleted out from under the character (no FK
-- enforcement on account_id) — the subquery would otherwise return
-- NULL and the migration would fail mid-flight against a NOT NULL
-- column, leaving the schema half-migrated.
UPDATE characters
SET    auth_level = COALESCE(
    (SELECT auth_level FROM accounts WHERE accounts.id = characters.account_id),
    1
);

ALTER TABLE accounts DROP COLUMN auth_level;
