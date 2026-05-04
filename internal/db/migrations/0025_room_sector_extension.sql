-- +migrate up
-- §sector-extension: widen the rooms.sector CHECK constraint to admit
-- four Wheel-of-Time-flavored terrains documented in
-- docs/wot_geography_mud.md:
--
--   blight    — corrupted lands north of the Mountains of Dhoom
--   waste     — Aiel Waste; arid rocky steppe (distinct from desert)
--   stedding  — Ogier sanctuary; channeling suppressed (mechanic TBD)
--   swamp     — Haddon Mirk, Drowned Lands, Paetrinh
--
-- A standard 12-step rebuild would require dropping rooms, but exits,
-- items, characters, mob_instances, and mob_trails all hold FKs into
-- rooms(id) with ON DELETE CASCADE / SET NULL. SQLite ignores
-- `PRAGMA foreign_keys = OFF` inside an open transaction (the migration
-- runner wraps every file in BEGIN/COMMIT), so the rebuild would
-- cascade-delete every child row. Edit sqlite_master directly instead
-- — only the CHECK clause widens, the column layout and data are
-- untouched, and the change is transactional.
--
-- The REPLACE target is the literal `'air','underground'` substring as
-- written in 0012_room_flags_and_sector.sql; this fragment occurs
-- exactly once in the rooms CREATE TABLE (inside the sector CHECK
-- list). If a future migration re-orders or rebuilds the rooms table,
-- it must re-apply this widening.
--
-- `writable_schema = RESET` (rather than OFF) forces the connection
-- to reread the schema on next access, so the new CHECK takes effect
-- immediately on the same *sql.DB pool that just ran the migration.
-- Using OFF leaves the parsed-schema cache stale until the connection
-- is recycled.
PRAGMA writable_schema = ON;

UPDATE sqlite_master
   SET sql = REPLACE(
       sql,
       '''air'',''underground''',
       '''air'',''underground'',''blight'',''waste'',''stedding'',''swamp'''
   )
 WHERE type = 'table' AND name = 'rooms';

PRAGMA writable_schema = RESET;
