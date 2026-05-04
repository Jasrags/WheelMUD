-- +migrate up
-- §10 wander tuning: per-template override of the global 25 % wander
-- chance the wander tick rolls for non-Sentinel mobs. NULL is not
-- meaningful here — every existing template defaults to the global
-- value, and per-template authoring sets a real number. CHECK pins
-- the column to [0.0, 1.0] so a typo can't disable wander globally
-- (negative) or panic the rng path (out-of-range).
--
-- wander_radius is reserved for a future slice once mob_instances
-- carries a stable spawn-room id; it's a forward declaration here so
-- builders authoring new templates know the column will exist.
ALTER TABLE mob_templates
    ADD COLUMN wander_chance REAL NOT NULL DEFAULT 0.25
        CHECK (wander_chance >= 0.0 AND wander_chance <= 1.0);
