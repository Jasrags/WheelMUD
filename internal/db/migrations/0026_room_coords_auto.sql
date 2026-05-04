-- +migrate up
-- §coords-auto: add rooms.coords_auto so the auto-derivation pass
-- (internal/world/coords_derive) can distinguish builder-authored
-- anchors from rooms that should be assigned coords by BFS.
--
-- Default 1 ("auto-derived"): every existing row backfills to 1
-- because none of them carry a builder-authored coords block today
-- (every Two Rivers room ships with the schema-default (0,0,0)).
-- The world loader stamps 0 ("explicit anchor") whenever a room's
-- YAML provides a `coords:` block, so future re-loads of an
-- explicitly-authored room flip back to anchor mode.
--
-- The CHECK constraint pins the field to {0,1} at the DB layer so a
-- corrupt direct UPDATE (admin tooling, hand-edited row, future code
-- path) can't poison the derivation pass.
ALTER TABLE rooms ADD COLUMN coords_auto INTEGER NOT NULL DEFAULT 1
    CHECK (coords_auto IN (0,1));
