-- +migrate up
-- §10 map: hide a room from the BFS minimap. NoMap rooms render as `[?]`
-- at the boundary of a neighbor's view and the BFS does NOT recurse
-- through them, so secret hideouts and admin-only zones stay
-- topologically opaque. Per-room flag mirrors the existing 0012 pattern;
-- defaults to 0 so every existing row keeps showing up on the map.
ALTER TABLE rooms ADD COLUMN nomap INTEGER NOT NULL DEFAULT 0 CHECK (nomap IN (0,1));
