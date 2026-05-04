-- 0028_item_parent.sql
--
-- Adds parent_item_id to items, enabling "inside another item" as a
-- first-class location alongside room_id and owner_character_id.
-- Same nullable-FK pattern as 0017_item_owner. Soft FK only — we don't
-- want ON DELETE CASCADE because deleting a container shouldn't make
-- its contents silently vanish; the verb layer pours contents to the
-- floor (or refuses) before destruction.
--
-- Invariant enforced in code (not the DB): for any reachable item,
-- exactly one of room_id / owner_character_id / parent_item_id is
-- non-NULL. All three NULL is the transient transfer-in-progress
-- state. A CHECK constraint can't model the brief "all NULL" gap, and
-- sqlite migrations are forward-only, so the invariant lives in the
-- repo Transfer* helpers (each UPDATE clears the other two location
-- columns in one statement).

ALTER TABLE items ADD COLUMN parent_item_id INTEGER;

CREATE INDEX items_parent_idx ON items(parent_item_id);
