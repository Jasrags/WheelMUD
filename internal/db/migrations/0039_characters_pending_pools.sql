-- 0039_characters_pending_pools.sql
--
-- Phase E #23 slice 4: pending-pool counters deposited at level-up.
-- `train` (slice 3) commits HP/BAB/saves/ClassLevels but doesn't yet
-- grant the *content* a level confers — feat slots, skill points,
-- ability bumps, and weave slots. This migration adds four typed
-- counters on `characters` so `RecordLevelUp` can deposit per-level
-- gains; future spend verbs (`learn`, `pick`, `bump`) will decrement.
--
-- All four default 0 so existing rows backfill cleanly. Slot order
-- per CLAUDE.md: between `pvp` (0037) and `auth_level` (must remain
-- the trailing column for the SQLite first-character bootstrap CASE
-- in CharacterRepo.Create).
--
-- Forward-only per CLAUDE.md (no down migration).
ALTER TABLE characters ADD COLUMN pending_feats INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN pending_skill_points INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN pending_ability_bumps INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN pending_weaves INTEGER NOT NULL DEFAULT 0;
