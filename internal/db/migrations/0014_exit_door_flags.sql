-- +migrate up
-- §9 Exit polish: door flags, optional key, lock difficulty, and a
-- per-exit description. Existing exit rows acquire the defaults
-- (no door, no key, no lock, blank description) so the migration
-- doesn't disturb the already-loaded world.
--
-- key_external_id references items.external_id so builders can
-- author keys symbolically in YAML; resolution to the item id
-- happens at unlock time (§16) once item-pickup ships. Empty
-- string means "no key needed" — a locked exit with an empty
-- key still requires lockpicking.
ALTER TABLE exits ADD COLUMN closed           INTEGER NOT NULL DEFAULT 0 CHECK (closed   IN (0,1));
ALTER TABLE exits ADD COLUMN locked           INTEGER NOT NULL DEFAULT 0 CHECK (locked   IN (0,1));
ALTER TABLE exits ADD COLUMN pickable         INTEGER NOT NULL DEFAULT 1 CHECK (pickable IN (0,1));
ALTER TABLE exits ADD COLUMN hidden           INTEGER NOT NULL DEFAULT 0 CHECK (hidden   IN (0,1));
ALTER TABLE exits ADD COLUMN nopass           INTEGER NOT NULL DEFAULT 0 CHECK (nopass   IN (0,1));
ALTER TABLE exits ADD COLUMN key_external_id  TEXT    NOT NULL DEFAULT '';
ALTER TABLE exits ADD COLUMN lock_difficulty  INTEGER NOT NULL DEFAULT 0 CHECK (lock_difficulty >= 0);
ALTER TABLE exits ADD COLUMN description      TEXT    NOT NULL DEFAULT '';
