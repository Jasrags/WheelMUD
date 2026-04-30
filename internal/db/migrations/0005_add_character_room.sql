-- +migrate up
-- SQLite forbids ALTER TABLE ADD COLUMN ... REFERENCES with a non-NULL
-- default while foreign_keys=ON, so the FK is enforced at the application
-- layer for now. A future table-rebuild migration can promote this to a
-- proper REFERENCES rooms(id) once we want strict integrity.
ALTER TABLE characters
    ADD COLUMN current_room_id INTEGER NOT NULL DEFAULT 1;
