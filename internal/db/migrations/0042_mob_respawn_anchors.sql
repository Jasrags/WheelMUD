-- 0042_mob_respawn_anchors.sql
--
-- Phase D §19 — mob respawn via §9 zone reset. Each YAML-seeded mob
-- entry is one (template, room) pair, so the template row doubles as
-- the spawn anchor. Two columns wire that anchor to the AreaReset
-- bucket:
--
--   mob_templates.home_room_id   — the room the loader spawned this
--                                  template into. 0 = not respawnable
--                                  (manual spawns via the `spawn`
--                                  admin verb leave it 0).
--   zones.last_reset_ts          — unix seconds, 0 = never. The
--                                  Respawner gates each zone's pass
--                                  on (now - last_reset_ts) >=
--                                  reset_interval_s and stamps this
--                                  on a successful pass.
--
-- The companion column mob_templates.respawn_zone_reset_id (added in
-- 0008 but never written) is finally populated by the loader: it
-- holds the zone.id this template belongs to, indexed below for the
-- per-zone anchor scan.
--
-- Forward-only per CLAUDE.md (no down migration).
ALTER TABLE mob_templates ADD COLUMN home_room_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE zones ADD COLUMN last_reset_ts INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_mob_templates_respawn_zone
    ON mob_templates(respawn_zone_reset_id);
