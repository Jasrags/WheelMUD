-- 0043_create_weave_teachers.sql
--
-- Weave-teacher subsystem (§12 / Phase E #28). Single table, 1:1 to a
-- mob_template (the teacher NPC). Mirrors `trainers` (0038) — same
-- soft-FK + UNIQUE convention as shops (0030) / bankers (0031).
-- Teachers gate the mid-game `learn weave` path: when one is present
-- in the room, `learn weave` drains the existing
-- characters.practice_points column (defined in 0009 but previously
-- unwritten — earning was wired in #28 via RecordLevelUp).
--
-- Schema choices:
--   * mob_template_id is a soft FK; UNIQUE so one template = one
--     weave teacher. Mirrors the trainer / shop / banker convention.
--   * max_level_taught is the highest weave level (0..9) the teacher
--     will offer. The chargen catalog is level-0 only today, so 0
--     is the conventional value; the column is in place for when
--     §12 widens the catalog.
--   * affinity_filter is a creature.PowerSet bitmask (5 bits over
--     Air/Earth/Fire/Water/Spirit). Zero means "any in-affinity
--     weave the channeler can learn"; non-zero restricts the
--     teacher to those Powers. The verb still intersects the
--     teacher's filter with the channeler's own affinity bitmask.
--
-- V1 has no fees, no min-level, no time cost. The cmd-layer `learn
-- weave` verb (§28) commits one weave into channeling_json's
-- WeavesKnownIDs and decrements practice_points by the weave's
-- chargen-catalog practice_cost.
--
-- Forward-only per CLAUDE.md (no down migration).

CREATE TABLE weave_teachers (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    mob_template_id   INTEGER NOT NULL UNIQUE,
    max_level_taught  INTEGER NOT NULL DEFAULT 0,
    affinity_filter   INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_weave_teachers_mob_template ON weave_teachers(mob_template_id);
