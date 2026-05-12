-- 0051_triggers_login_events.sql
--
-- Phase F #32 slice 5b — widen triggers.event CHECK to include
-- the two new PC lifecycle events `on_login` and `on_logout`.
-- The dispatcher subscribes to new world.PlayerLoggedIn /
-- PlayerLoggedOut events (published from internal/mode/postauth.go
-- and cmd/server/main.go's handleConnection defer) and fans out to
-- room-owned triggers for the affected character's room.
--
-- SQLite doesn't support `ALTER TABLE ... DROP CHECK`, so the
-- table-rebuild dance below is the only forward-only way to widen
-- the constraint. The new table inherits every column from the
-- 0044 + 0046 layout (event/match/action/payload/priority/created_at
-- + consecutive_faults/disabled) so live rows round-trip unchanged.
-- Indexes are recreated from the same idx_triggers_owner_event /
-- idx_triggers_event definitions as 0044.
--
-- Forward-only per CLAUDE.md (no down migration).

PRAGMA foreign_keys = OFF;

CREATE TABLE triggers_new (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_kind         TEXT    NOT NULL CHECK (owner_kind IN ('mob_template','room')),
    owner_id           INTEGER NOT NULL,
    event              TEXT    NOT NULL CHECK (event IN (
        'on_enter','on_say','on_attack','on_death','on_tick',
        'on_login','on_logout'
    )),
    match              TEXT    NOT NULL DEFAULT '',
    action             TEXT    NOT NULL,
    payload            TEXT    NOT NULL DEFAULT '{}',
    priority           INTEGER NOT NULL DEFAULT 0,
    created_at         INTEGER NOT NULL DEFAULT (unixepoch()),
    consecutive_faults INTEGER NOT NULL DEFAULT 0,
    disabled           INTEGER NOT NULL DEFAULT 0
);

INSERT INTO triggers_new (
    id, owner_kind, owner_id, event, match, action, payload, priority,
    created_at, consecutive_faults, disabled
)
SELECT
    id, owner_kind, owner_id, event, match, action, payload, priority,
    created_at, consecutive_faults, disabled
FROM triggers;

DROP TABLE triggers;
ALTER TABLE triggers_new RENAME TO triggers;

CREATE INDEX idx_triggers_owner_event ON triggers(owner_kind, owner_id, event);
CREATE INDEX idx_triggers_event       ON triggers(event);

PRAGMA foreign_keys = ON;
