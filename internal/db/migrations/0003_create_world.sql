-- +migrate up
CREATE TABLE rooms (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    short_desc  TEXT NOT NULL DEFAULT '',
    long_desc   TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE exits (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    from_room_id INTEGER NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    to_room_id   INTEGER NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    direction    TEXT    NOT NULL CHECK (direction IN ('n','s','e','w','u','d')),
    UNIQUE(from_room_id, direction)
);
CREATE INDEX exits_from_room_idx ON exits(from_room_id);

CREATE TABLE items (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    name_lower  TEXT NOT NULL,
    short_desc  TEXT NOT NULL DEFAULT '',
    room_id     INTEGER REFERENCES rooms(id) ON DELETE SET NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX items_room_idx       ON items(room_id);
CREATE INDEX items_name_lower_idx ON items(name_lower);

CREATE TABLE mobs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    name_lower  TEXT NOT NULL,
    short_desc  TEXT NOT NULL DEFAULT '',
    room_id     INTEGER REFERENCES rooms(id) ON DELETE SET NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX mobs_room_idx       ON mobs(room_id);
CREATE INDEX mobs_name_lower_idx ON mobs(name_lower);
