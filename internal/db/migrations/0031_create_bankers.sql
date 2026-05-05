-- 0031_create_bankers.sql
--
-- Banker subsystem (§14). Single table, 1:1 to a mob_template (the
-- banker NPC). Carries operating hours only — V1 has no fees, no
-- min-deposit, no per-tier slot limits. Coin moves through the
-- existing characters.coin / characters.bank_balance columns via
-- CharacterRepo.RecordCoin (see migration 0019 / character_sql.go).
--
-- Schema choices:
--   * mob_template_id is a soft FK; UNIQUE so one template = one
--     banker. Same convention as shops (migration 0030).
--   * open_hour == close_hour means always open (24h). Otherwise the
--     banker is open when wall-hour ∈ [OpenHour, CloseHour); wraps
--     across midnight when CloseHour < OpenHour.
--
-- Forward-only per CLAUDE.md (no down migration).

CREATE TABLE bankers (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    mob_template_id INTEGER NOT NULL UNIQUE,
    open_hour       INTEGER NOT NULL DEFAULT 0 CHECK(open_hour  BETWEEN 0 AND 23),
    close_hour      INTEGER NOT NULL DEFAULT 0 CHECK(close_hour BETWEEN 0 AND 23)
);

CREATE INDEX idx_bankers_mob_template ON bankers(mob_template_id);
