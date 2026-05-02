-- +migrate up
--
-- Extend the existing characters table with the Core stat block and
-- player-only fields from internal/creature.Character. Keeps the
-- account_id / name / created_at / last_played_at columns from
-- 0002_create_characters.sql; current_room_id from 0005.
--
-- SQLite supports ADD COLUMN but not multi-add; one statement each.

ALTER TABLE characters ADD COLUMN size              INTEGER NOT NULL DEFAULT 4;  -- Medium
ALTER TABLE characters ADD COLUMN type              INTEGER NOT NULL DEFAULT 0;  -- Humanoid
ALTER TABLE characters ADD COLUMN gender            INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN alignment         INTEGER NOT NULL DEFAULT 0;

ALTER TABLE characters ADD COLUMN str_cur INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN str_max INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN str_inh INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN dex_cur INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN dex_max INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN dex_inh INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN con_cur INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN con_max INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN con_inh INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN int_cur INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN int_max INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN int_inh INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN wis_cur INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN wis_max INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN wis_inh INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN cha_cur INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN cha_max INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN cha_inh INTEGER NOT NULL DEFAULT 10;

ALTER TABLE characters ADD COLUMN hp_current   INTEGER NOT NULL DEFAULT 1;
ALTER TABLE characters ADD COLUMN hp_max       INTEGER NOT NULL DEFAULT 1;
ALTER TABLE characters ADD COLUMN subdual      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN hit_dice     TEXT    NOT NULL DEFAULT '1d8';

ALTER TABLE characters ADD COLUMN defense      INTEGER NOT NULL DEFAULT 10;
ALTER TABLE characters ADD COLUMN save_fort    INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN save_ref     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN save_will    INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN init_mod     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN bab          INTEGER NOT NULL DEFAULT 0;

ALTER TABLE characters ADD COLUMN speed_base_ft      INTEGER NOT NULL DEFAULT 30;
ALTER TABLE characters ADD COLUMN speed_climb_ft     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN speed_fly_ft       INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN speed_fly_maneuver INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN speed_swim_ft      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN speed_burrow_ft    INTEGER NOT NULL DEFAULT 0;

ALTER TABLE characters ADD COLUMN reach_ft     INTEGER NOT NULL DEFAULT 5;
ALTER TABLE characters ADD COLUMN face_ft      INTEGER NOT NULL DEFAULT 5;
ALTER TABLE characters ADD COLUMN threat_ft    INTEGER NOT NULL DEFAULT 5;

ALTER TABLE characters ADD COLUMN conditions     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN position_flags INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN specials       INTEGER NOT NULL DEFAULT 0;

ALTER TABLE characters ADD COLUMN dr_json      TEXT NOT NULL DEFAULT '[]';
ALTER TABLE characters ADD COLUMN resists_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE characters ADD COLUMN affects_json TEXT NOT NULL DEFAULT '[]';

-- Player-only fields.
ALTER TABLE characters ADD COLUMN race          INTEGER NOT NULL DEFAULT 0;  -- Human
ALTER TABLE characters ADD COLUMN background    INTEGER NOT NULL DEFAULT 7;  -- Midlander
ALTER TABLE characters ADD COLUMN class_levels_json TEXT NOT NULL DEFAULT '{}';

ALTER TABLE characters ADD COLUMN xp                INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN feats_json        TEXT    NOT NULL DEFAULT '[]';
ALTER TABLE characters ADD COLUMN skills_json       TEXT    NOT NULL DEFAULT '{}';
ALTER TABLE characters ADD COLUMN practice_points   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN class_features_json TEXT  NOT NULL DEFAULT '[]';

ALTER TABLE characters ADD COLUMN height_cm   INTEGER NOT NULL DEFAULT 170;
ALTER TABLE characters ADD COLUMN weight_kg   INTEGER NOT NULL DEFAULT 70;
ALTER TABLE characters ADD COLUMN age         INTEGER NOT NULL DEFAULT 20;
ALTER TABLE characters ADD COLUMN handedness  INTEGER NOT NULL DEFAULT 0;

ALTER TABLE characters ADD COLUMN fame         INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN infamy       INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN infamy_share REAL    NOT NULL DEFAULT 0.0;

-- Wealth: stored as base copper pennies (see internal/currency).
ALTER TABLE characters ADD COLUMN coin_cp     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN bank_cp     INTEGER NOT NULL DEFAULT 0;

ALTER TABLE characters ADD COLUMN encumbrance   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN fatigue_until DATETIME;
ALTER TABLE characters ADD COLUMN position      INTEGER NOT NULL DEFAULT 0;  -- Standing
ALTER TABLE characters ADD COLUMN idle_since    DATETIME;

ALTER TABLE characters ADD COLUMN bound_room_id   INTEGER NOT NULL DEFAULT 1;  -- StarterRoomID
ALTER TABLE characters ADD COLUMN played_seconds  INTEGER NOT NULL DEFAULT 0;
ALTER TABLE characters ADD COLUMN last_login      DATETIME;

ALTER TABLE characters ADD COLUMN quest_log_json      TEXT NOT NULL DEFAULT '[]';
ALTER TABLE characters ADD COLUMN dialogue_state_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE characters ADD COLUMN equipment_json      TEXT NOT NULL DEFAULT '{}';
ALTER TABLE characters ADD COLUMN inventory_json      TEXT NOT NULL DEFAULT '[]';
