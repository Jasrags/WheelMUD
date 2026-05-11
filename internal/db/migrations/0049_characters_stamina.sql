-- 0049_characters_stamina.sql
--
-- Phase L slice 63 — racial speed + stamina pool.
--
-- Adds the per-character stamina pool that drains on every combat
-- action and refills via the Regen tick. Mirrors the HP shape from
-- migration 0009 (current/max are persisted; regen is per-character
-- so racial differences survive a level-up).
--
-- StaminaCurrent / StaminaMax / StaminaRegen are int32 to match HP
-- and avoid future overflow guards when buffs / temporary pools
-- land. Default 0 is safe: pre-0049 characters present as "no
-- stamina" until a relog through chargen finalize stamps the racial
-- profile (or until an admin tool top-ups them; deferred).
--
-- Mob templates intentionally stay stamina-less in V1 — NPC AI
-- doesn't pick attack variants yet, so a stamina pool would only
-- gate behavior nothing produces. Mob stamina is a follow-up once
-- AI grows variant selection.
--
-- Placement: strictly between `skill_cooldowns_json` (0047) and
-- `auth_level`. The auth_level column MUST stay the very last entry
-- in charPlayerColumns / charPlayerValues / charPlayerScanDest for
-- the SQLite first-character bootstrap CASE in CharacterRepo.Create.
--
-- Forward-only per CLAUDE.md (no down migration).
ALTER TABLE characters ADD COLUMN stamina_current INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN stamina_max     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN stamina_regen   INTEGER NOT NULL DEFAULT 0;
