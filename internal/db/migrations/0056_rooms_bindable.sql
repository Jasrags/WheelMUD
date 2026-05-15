-- +migrate up
-- §D #19 bind: rooms flagged `bindable: true` accept the player `bind` verb,
-- which retargets Character.BoundRoomID so death respawns drop the player
-- here instead of the world starter. Mirrors the per-room flag pattern from
-- 0012/0020 — bool-per-column with a CHECK constraint, defaults to 0 so
-- every existing room stays unbindable until a builder opts in.
ALTER TABLE rooms ADD COLUMN bindable INTEGER NOT NULL DEFAULT 0 CHECK (bindable IN (0,1));
