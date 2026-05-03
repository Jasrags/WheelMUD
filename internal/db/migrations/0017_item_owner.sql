-- 0017_item_owner.sql
--
-- Adds owner_character_id to items, enabling character inventory as a
-- first-class location alongside room_id. Mirrors the nullable room_id
-- pattern (NULL = not held by anyone). Soft FK — the loader and the
-- inventory commands keep referential integrity in code; we don't add
-- ON DELETE CASCADE because deleting a character should leave their
-- items recoverable for admin review rather than silently vanish.
--
-- Invariant enforced in code (not the DB): for any reachable item,
-- exactly one of room_id / owner_character_id is non-NULL. Both NULL
-- means orphan/limbo and is only legal during a transfer transaction.

-- Soft FK: no REFERENCES clause. Same rationale as rooms.zone_id from
-- migration 0016 — characters are heavyweight (account + full schema)
-- so a hard FK makes repo-level item tests painful, and the inventory
-- commands enforce the invariant at write time.
ALTER TABLE items ADD COLUMN owner_character_id INTEGER;

CREATE INDEX items_owner_idx ON items(owner_character_id);
