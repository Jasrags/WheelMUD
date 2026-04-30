-- +migrate up
-- Migrate from SQL-seeded world to YAML-loaded world. The existing
-- seed rows are wiped; the loader (internal/world/LoadAndSync)
-- repopulates them on the next boot. Existing characters keep their
-- account/name but lose their room pointer; promoteToGame's fallback
-- to repo.StarterRoomID drops them at the new starter on next login.
UPDATE characters SET current_room_id = 0;
DELETE FROM mobs;
DELETE FROM items;
DELETE FROM exits;
DELETE FROM rooms;

-- Reset autoincrement so the loader can pin the starter room to id=1
-- (preserving the repo.StarterRoomID = 1 contract). sqlite_sequence
-- only contains rows for tables that have AUTOINCREMENT; missing rows
-- are tolerated by the WHERE clause.
DELETE FROM sqlite_sequence WHERE name IN ('rooms','items','mobs','exits');

-- external_id is the stable, human-authored identifier referenced from
-- YAML (e.g. "plaza.fountain"). The repo Create methods reject empty
-- strings; the DEFAULT '' here is just a transitional artifact for the
-- ALTER, not a real default for inserts.
ALTER TABLE rooms ADD COLUMN external_id TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX rooms_external_id_idx ON rooms(external_id);

ALTER TABLE items ADD COLUMN external_id TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX items_external_id_idx ON items(external_id);

ALTER TABLE mobs  ADD COLUMN external_id TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX mobs_external_id_idx  ON mobs(external_id);
