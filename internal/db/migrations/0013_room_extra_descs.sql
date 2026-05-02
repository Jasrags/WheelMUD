-- +migrate up
-- §9 Room ambient/extra descriptions: a per-room map from keyword
-- (e.g. "fountain", "statue") to long-form text rendered by
-- `look <noun>`. JSON column so builders can grow the map without
-- a schema change per room. DEFAULT '{}' lets existing rows from
-- migration 0012 round-trip without a backfill pass.
ALTER TABLE rooms ADD COLUMN extra_descs_json TEXT NOT NULL DEFAULT '{}';
