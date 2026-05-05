-- 0029_create_admin_audit.sql
--
-- admin_audit table — append-only forensic log of privileged verb
-- invocations (spawn, teleport, goto/transfer/summon, wizinvis,
-- shutdown, reboot, future admin tools).
--
-- Schema choices:
--   * actor_character_id is the stable join key; actor_name is a
--     snapshot at write time so a later rename doesn't rewrite history.
--   * actor_character_id = 0 is reserved for system-actor rows
--     (signal-driven shutdown, scheduled jobs); none are written today
--     but the door is open.
--   * target / args are TEXT NOT NULL DEFAULT '' so callers don't have
--     to think about NULL vs empty string.
--   * No FK on actor_character_id — characters can be deleted; the
--     audit trail must outlive them.
--
-- Forward-only per CLAUDE.md (no down migration).

CREATE TABLE admin_audit (
    id                  INTEGER PRIMARY KEY,
    ts                  INTEGER NOT NULL,
    actor_character_id  INTEGER NOT NULL,
    actor_name          TEXT    NOT NULL DEFAULT '',
    verb                TEXT    NOT NULL,
    target              TEXT    NOT NULL DEFAULT '',
    args                TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX admin_audit_ts_idx    ON admin_audit(ts);
CREATE INDEX admin_audit_actor_idx ON admin_audit(actor_character_id, ts);
