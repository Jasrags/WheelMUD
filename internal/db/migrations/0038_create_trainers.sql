-- 0038_create_trainers.sql
--
-- Trainer subsystem (§12 / Phase E #23). Single table, 1:1 to a
-- mob_template (the trainer NPC). Carries the chargen class_id the
-- trainer can teach. V1 has no fees, no min-level, no per-class skill
-- requirements; the cmd-layer `train` verb (slice 3) commits one
-- level into ClassLevels[class] when the player has earned it.
--
-- Schema choices:
--   * mob_template_id is a soft FK; UNIQUE so one template = one
--     trainer. Same convention as shops (0030) / bankers (0031).
--   * class_id is a chargen catalog ID (e.g. "armsman", "wilder") —
--     not the creature.Class enum int — so a content swap of the
--     catalog doesn't require a DB migration. The world loader
--     validates the format (non-empty external-id charset) at boot;
--     the `train` verb resolves it against the live chargen catalog
--     before advancing the character (typo → polite refusal at runtime).
--
-- Forward-only per CLAUDE.md (no down migration).

CREATE TABLE trainers (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    mob_template_id INTEGER NOT NULL UNIQUE,
    class_id        TEXT    NOT NULL
);

CREATE INDEX idx_trainers_mob_template ON trainers(mob_template_id);
