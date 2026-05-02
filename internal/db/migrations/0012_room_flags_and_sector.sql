-- +migrate up
-- §9 Room: add flag columns, sector enum, light level, and coordinates.
-- All columns are NOT NULL with sensible defaults so existing rows
-- (loaded from YAML in earlier boots) come along for the ride. The
-- sector CHECK constraint mirrors the documented enum; widening it
-- later requires a table rebuild (see 0007 for the pattern).
ALTER TABLE rooms ADD COLUMN indoors     INTEGER NOT NULL DEFAULT 0 CHECK (indoors     IN (0,1));
ALTER TABLE rooms ADD COLUMN nopvp       INTEGER NOT NULL DEFAULT 0 CHECK (nopvp       IN (0,1));
ALTER TABLE rooms ADD COLUMN noteleport  INTEGER NOT NULL DEFAULT 0 CHECK (noteleport  IN (0,1));
ALTER TABLE rooms ADD COLUMN dark        INTEGER NOT NULL DEFAULT 0 CHECK (dark        IN (0,1));
ALTER TABLE rooms ADD COLUMN silent      INTEGER NOT NULL DEFAULT 0 CHECK (silent      IN (0,1));
ALTER TABLE rooms ADD COLUMN peaceful    INTEGER NOT NULL DEFAULT 0 CHECK (peaceful    IN (0,1));
ALTER TABLE rooms ADD COLUMN sector      TEXT    NOT NULL DEFAULT 'city'
    CHECK (sector IN ('city','forest','field','hills','mountain','desert','water','underwater','air','underground'));
-- light_level: 0 = pitch black, higher = brighter. dark=1 with
-- light_level=0 hides description/items/mobs in look. A torch in the
-- room (or on a player, once §14 lands) bumps the effective level.
ALTER TABLE rooms ADD COLUMN light_level INTEGER NOT NULL DEFAULT 100;
ALTER TABLE rooms ADD COLUMN coord_x     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE rooms ADD COLUMN coord_y     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE rooms ADD COLUMN coord_z     INTEGER NOT NULL DEFAULT 0;
