-- 0050_items_decay_expires_at.sql
--
-- Phase D §19 polish — durable corpse decay.
--
-- Adds a nullable wall-clock deadline to items so corpse rows survive
-- a server restart with their decay schedule intact. Today combat.Decayer
-- is an in-memory FIFO populated by combat.spawnCorpse; restart drops the
-- queue and a corpse lingers indefinitely until an admin purges it.
--
-- Forward path: spawnCorpse stamps decay_expires_at with `now() +
-- corpseDecayDuration`; on boot, combat.Decayer.RearmFromRepo walks
-- every row whose decay_expires_at IS NOT NULL and either deletes
-- (past-deadline) or re-Schedules (future-deadline) it. Existing items
-- have NULL and are untouched.
--
-- No backfill: corpses spawned before the upgrade lose their deadline
-- and linger until admin purge (one-shot pain on the first restart
-- after deploy; accepted in V1).
--
-- Partial index keeps the boot-rearm scan O(corpses) instead of
-- O(items). SQLite supports partial indexes.
--
-- Forward-only per CLAUDE.md.
ALTER TABLE items ADD COLUMN decay_expires_at TIMESTAMP NULL;
CREATE INDEX idx_items_decay_expires_at
  ON items(decay_expires_at) WHERE decay_expires_at IS NOT NULL;
