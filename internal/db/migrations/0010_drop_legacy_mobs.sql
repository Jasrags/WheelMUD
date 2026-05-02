-- +migrate up
--
-- The `mobs` table from 0003_create_world.sql is replaced by the
-- mob_templates / mob_instances pair from 0008. The world loader
-- now spawns instances from templates; nothing reads `mobs` anymore.
DROP TABLE IF EXISTS mobs;
