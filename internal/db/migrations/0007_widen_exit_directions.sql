-- +migrate up
-- Widen the exits.direction CHECK constraint to accept the four
-- diagonals (ne/nw/se/sw) in addition to the cardinals + vertical.
-- SQLite cannot ALTER a CHECK constraint in place, so the table is
-- rebuilt with the same shape and indexes restored. Existing rows
-- carry over via the implicit column-order copy below.
--
-- Safe to DROP TABLE exits with foreign keys enforced because no
-- other schema object references exits(id). If a future table ever
-- gains an FK to exits, this rebuild must be split out of the
-- surrounding migration transaction so PRAGMA foreign_keys=OFF can
-- actually take effect (it is a no-op inside a transaction).
CREATE TABLE exits_new (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    from_room_id INTEGER NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    to_room_id   INTEGER NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    direction    TEXT    NOT NULL CHECK (direction IN ('n','s','e','w','u','d','ne','nw','se','sw')),
    UNIQUE(from_room_id, direction)
);

INSERT INTO exits_new (id, from_room_id, to_room_id, direction)
    SELECT id, from_room_id, to_room_id, direction FROM exits;

DROP TABLE exits;
ALTER TABLE exits_new RENAME TO exits;
CREATE INDEX exits_from_room_idx ON exits(from_room_id);
