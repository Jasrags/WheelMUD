-- 0048_exits_authored_door_state.sql
--
-- §7 Area/zone reset extension — door state on AreaReset. The reset
-- pipeline now restores authored door state on the same per-zone
-- cadence the mob respawn (0042) already obeys. To do that, the
-- exits table needs to remember the YAML-authored Closed / Locked
-- values separately from the runtime-mutable ones the §16
-- open/close/lock/unlock verbs flip.
--
--   exits.authored_closed   — Closed value at YAML load. Never
--                             mutates after insert.
--   exits.authored_locked   — Locked value at YAML load. Never
--                             mutates after insert.
--
-- ZoneResetter restores the runtime columns from these on every
-- zone reset that fires (subject to ResetMode + ResetIntervalS +
-- occupancy gates). Hidden / NoPass / Pickable have no authored/
-- runtime split because they can't change at runtime today.
--
-- Backfill: pre-0048 zones lose their original authoring data
-- (it was never recorded). Backfill from the current runtime
-- state so reset becomes a no-op on existing rows; new zones
-- loaded post-migration get the real authored values from YAML.
--
-- Forward-only per CLAUDE.md (no down migration).
ALTER TABLE exits ADD COLUMN authored_closed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE exits ADD COLUMN authored_locked INTEGER NOT NULL DEFAULT 0;
UPDATE exits SET authored_closed = closed, authored_locked = locked;
