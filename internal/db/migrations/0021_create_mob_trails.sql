-- +migrate up
-- §10 trails: per-mob ring-buffer of recent room visits, recorded
-- by MobInstanceRepo.UpdateRoom on every mob movement (admin tools,
-- future wander tick, zone-reset relocations). The forthcoming
-- `track <name>` verb reads the freshest entry per mob; the §12
-- skill check decides the staleness window at read time.
--
-- ON DELETE CASCADE on both parents keeps the table self-pruning
-- when a mob despawns or a room is removed. The (mob_id, ts DESC)
-- index serves both "newest entry" and "list last N for a mob"
-- without an extra sort, and is the column the cap-prune query
-- subselects on.
CREATE TABLE mob_trails (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    mob_id  INTEGER NOT NULL REFERENCES mob_instances(id) ON DELETE CASCADE,
    room_id INTEGER NOT NULL REFERENCES rooms(id)         ON DELETE CASCADE,
    ts      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX mob_trails_mob_ts_idx ON mob_trails(mob_id, ts DESC);
