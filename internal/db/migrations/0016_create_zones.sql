-- +migrate up
-- §9 Area / zone. Promotes zones from a parse-time YAML grouping into
-- a persisted, queryable entity that the runtime can attach behavior
-- to. Until this migration, zones existed only inside the loader and
-- were discarded after insert; rooms had no DB-level affiliation.
--
-- - `external_id` mirrors the YAML `zone.yaml` `id:` (e.g.
--   `emonds_field`). UNIQUE so a duplicate zone definition fails fast
--   at load time rather than producing two identically-named zones
--   the admin tools can't tell apart.
-- - `builder` records authorship for §16 builder-tool permission
--   scoping (a `builder_zones` table later keys off zones.id).
-- - `min_level` / `max_level` advise content gating; combat / loot
--   layers will read these to scale encounters and warn players who
--   wander into a too-tough zone.
-- - `reset_interval_s` + `reset_mode` drive the future §9 zone-reset
--   pipeline (areaReset bucket reads them per zone). `mode=empty`
--   resets only when no players are present; `always` ignores
--   occupancy; `never` disables resets entirely. Schema lands now so
--   reset rules can be authored against stable columns; the bucket
--   wiring lands in a follow-up.
-- - `climate` is a free-text bucket for §10 ambient/weather hooks
--   (temperate / arid / coastal / cold / blighted / etc.). Left
--   open-ended on purpose — the value lives in YAML and rendering
--   code can switch on it without a schema bump.
-- - `ambient_json` holds the rotating ambient lines (§10 ambient
--   ticker reads these). JSON list of strings; `[]` is valid.
--
-- The `rooms.zone_id` column joins rooms to their owning zone. Default
-- 0 keeps test fixtures that bypass the world loader buildable; the
-- loader stamps a real id during insert. The FOREIGN KEY is
-- intentionally NOT declared inline because SQLite enforces FK on
-- non-NULL DEFAULT values, which would reject every test fixture that
-- inserts a room without first inserting a zone. Treated as a soft FK
-- enforced by the loader and repo layer in this slice; a follow-up
-- migration can promote it to a hard constraint via table rebuild
-- once §16 admin room-create reliably supplies a zone id.
CREATE TABLE zones (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    external_id       TEXT    NOT NULL UNIQUE,
    name              TEXT    NOT NULL,
    builder           TEXT    NOT NULL DEFAULT '',
    min_level         INTEGER NOT NULL DEFAULT 1,
    max_level         INTEGER NOT NULL DEFAULT 60,
    reset_interval_s  INTEGER NOT NULL DEFAULT 600,
    reset_mode        TEXT    NOT NULL DEFAULT 'empty'
                      CHECK (reset_mode IN ('always', 'empty', 'never')),
    climate           TEXT    NOT NULL DEFAULT '',
    ambient_json      TEXT    NOT NULL DEFAULT '[]',
    CHECK (min_level >= 1),
    CHECK (max_level >= min_level),
    CHECK (reset_interval_s >= 0)
);

ALTER TABLE rooms ADD COLUMN zone_id INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_rooms_zone_id ON rooms(zone_id);
