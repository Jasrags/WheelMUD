-- 0052_character_audit.sql
--
-- character_audit table — append-only forensic log of in-game
-- commands dispatched by post-login players (Phase J slice J3).
-- Off by default; turned on via audit.commands_enabled in config.
--
-- Schema choices:
--   * character_id is the stable join key; character_name is a
--     snapshot at write time so a later rename doesn't rewrite
--     history (mirrors admin_audit's actor_name treatment).
--   * room_id captures the player's location at dispatch time so
--     forensic queries can reconstruct movement without joining the
--     world snapshot.
--   * verb is the first whitespace-separated token of the input
--     line (after alias expansion); raw is the full command line,
--     soft-capped at insert time. Both default to '' so callers
--     don't have to think about NULL.
--   * No FK on character_id — characters can be deleted; the audit
--     trail must outlive them.
--
-- Two indexes mirror the admin_audit pattern: (character_id, ts) for
-- per-character forensic queries, and ts alone for time-range scans.
--
-- Forward-only per CLAUDE.md (no down migration).

CREATE TABLE character_audit (
    id             INTEGER PRIMARY KEY,
    ts             INTEGER NOT NULL,
    character_id   INTEGER NOT NULL,
    character_name TEXT    NOT NULL DEFAULT '',
    room_id        INTEGER NOT NULL DEFAULT 0,
    verb           TEXT    NOT NULL,
    raw            TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX character_audit_char_idx ON character_audit(character_id, ts);
CREATE INDEX character_audit_ts_idx   ON character_audit(ts);
