-- +migrate up
--
-- Creature aggregates: mob templates (immutable archetypes) and
-- mob instances (live spawns), plus the channeling sub-record that
-- attaches to either a mob template, a mob instance, or a character.
--
-- The existing flat `mobs` table from 0003_create_world.sql stays in
-- place until the world loader is migrated to spawn from templates;
-- a follow-up migration will drop it.
--
-- Stat-block columns mirror internal/creature.Core. Complex bits
-- (resists, DR, natural attacks, traits, trigger scripts) are JSON
-- text — SQLite has no native JSON type but `json1` is built in.

CREATE TABLE mob_templates (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    external_id              TEXT    NOT NULL UNIQUE,
    name                     TEXT    NOT NULL,
    name_lower               TEXT    NOT NULL,

    -- Core / Type / Gender / Alignment / Size as small ints
    -- matching the iota ordering in internal/creature.
    size                     INTEGER NOT NULL DEFAULT 4,  -- Medium
    type                     INTEGER NOT NULL DEFAULT 0,  -- Humanoid
    gender                   INTEGER NOT NULL DEFAULT 0,  -- None
    alignment                INTEGER NOT NULL DEFAULT 0,  -- Good

    -- Ability scores: current/max/inherent for each of the six.
    str_cur INTEGER NOT NULL DEFAULT 10, str_max INTEGER NOT NULL DEFAULT 10, str_inh INTEGER NOT NULL DEFAULT 10,
    dex_cur INTEGER NOT NULL DEFAULT 10, dex_max INTEGER NOT NULL DEFAULT 10, dex_inh INTEGER NOT NULL DEFAULT 10,
    con_cur INTEGER NOT NULL DEFAULT 10, con_max INTEGER NOT NULL DEFAULT 10, con_inh INTEGER NOT NULL DEFAULT 10,
    int_cur INTEGER NOT NULL DEFAULT 10, int_max INTEGER NOT NULL DEFAULT 10, int_inh INTEGER NOT NULL DEFAULT 10,
    wis_cur INTEGER NOT NULL DEFAULT 10, wis_max INTEGER NOT NULL DEFAULT 10, wis_inh INTEGER NOT NULL DEFAULT 10,
    cha_cur INTEGER NOT NULL DEFAULT 10, cha_max INTEGER NOT NULL DEFAULT 10, cha_inh INTEGER NOT NULL DEFAULT 10,

    hp_max                   INTEGER NOT NULL DEFAULT 1,
    hit_dice                 TEXT    NOT NULL DEFAULT '1d8',

    defense                  INTEGER NOT NULL DEFAULT 10,
    save_fort                INTEGER NOT NULL DEFAULT 0,
    save_ref                 INTEGER NOT NULL DEFAULT 0,
    save_will                INTEGER NOT NULL DEFAULT 0,
    init_mod                 INTEGER NOT NULL DEFAULT 0,
    bab                      INTEGER NOT NULL DEFAULT 0,

    speed_base_ft            INTEGER NOT NULL DEFAULT 30,
    speed_climb_ft           INTEGER NOT NULL DEFAULT 0,
    speed_fly_ft             INTEGER NOT NULL DEFAULT 0,
    speed_fly_maneuver       INTEGER NOT NULL DEFAULT 0,
    speed_swim_ft            INTEGER NOT NULL DEFAULT 0,
    speed_burrow_ft          INTEGER NOT NULL DEFAULT 0,

    reach_ft                 INTEGER NOT NULL DEFAULT 5,
    face_ft                  INTEGER NOT NULL DEFAULT 5,
    threat_ft                INTEGER NOT NULL DEFAULT 5,

    specials                 INTEGER NOT NULL DEFAULT 0,  -- SpecialQuality bitmask

    -- Variable-length data as JSON.
    dr_json                  TEXT    NOT NULL DEFAULT '[]',
    resists_json             TEXT    NOT NULL DEFAULT '[]',
    natural_attacks_json     TEXT    NOT NULL DEFAULT '[]',
    special_attacks_json     TEXT    NOT NULL DEFAULT '[]',
    traits_json              TEXT    NOT NULL DEFAULT '[]',
    advancement_json         TEXT    NOT NULL DEFAULT '[]',
    climate_json             TEXT    NOT NULL DEFAULT '[]',
    terrain_json             TEXT    NOT NULL DEFAULT '[]',
    trigger_scripts_json     TEXT    NOT NULL DEFAULT '[]',

    -- Template-only metadata.
    challenge_code           TEXT    NOT NULL DEFAULT 'A' CHECK (length(challenge_code) = 1),
    organization             TEXT    NOT NULL DEFAULT 'solitary',
    behavior_flags           INTEGER NOT NULL DEFAULT 0,

    loot_table_id            INTEGER NOT NULL DEFAULT 0,
    gold_dice                TEXT    NOT NULL DEFAULT '',
    dialogue_tree_id         INTEGER NOT NULL DEFAULT 0,
    shopkeeper_json          TEXT,                              -- nullable: NULL = not a vendor
    corpse_decay_ticks       INTEGER NOT NULL DEFAULT 600,      -- 10 min @ 1Hz
    respawn_zone_reset_id    INTEGER NOT NULL DEFAULT 0,

    -- Shadowspawn-only.
    shadow_link_myrddraal_id INTEGER NOT NULL DEFAULT 0,
    taint_immune             INTEGER NOT NULL DEFAULT 0,
    fade_link_master_ticks   INTEGER NOT NULL DEFAULT 0,

    short_desc               TEXT    NOT NULL DEFAULT '',
    long_desc                TEXT    NOT NULL DEFAULT '',

    created_at               DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX mob_templates_name_lower_idx ON mob_templates(name_lower);

CREATE TABLE mob_instances (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id     INTEGER NOT NULL REFERENCES mob_templates(id) ON DELETE CASCADE,
    room_id         INTEGER REFERENCES rooms(id) ON DELETE SET NULL,

    -- Live mutable state. Other Core fields are read from the
    -- template until they diverge; on first mutation the runner
    -- snapshots them into a row in mob_instance_overrides (added
    -- in a follow-up migration if/when needed). For v1 we just
    -- track HP, conditions, and position.
    hp_current      INTEGER NOT NULL,
    subdual         INTEGER NOT NULL DEFAULT 0,
    conditions      INTEGER NOT NULL DEFAULT 0,
    position_flags  INTEGER NOT NULL DEFAULT 0,

    spawned_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    bound_reset_id  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX mob_instances_room_idx     ON mob_instances(room_id);
CREATE INDEX mob_instances_template_idx ON mob_instances(template_id);

-- Channeling sub-record. owner_kind discriminates which table the
-- owner_id points at: 1=character, 2=mob_template, 3=mob_instance.
-- Kept as a polymorphic association rather than three separate
-- tables because the schema is identical across owners.
CREATE TABLE channeling (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_kind             INTEGER NOT NULL CHECK (owner_kind IN (1,2,3)),
    owner_id               INTEGER NOT NULL,

    gender_source          INTEGER NOT NULL,         -- Saidin / Saidar
    channeler_type         INTEGER NOT NULL,         -- Initiate / Wilder
    affinities             INTEGER NOT NULL DEFAULT 0, -- PowerSet bitmask

    talents_json           TEXT    NOT NULL DEFAULT '[]',
    weaves_known_json      TEXT    NOT NULL DEFAULT '[]',
    slots_json             TEXT    NOT NULL DEFAULT '[]', -- 10 entries: {cur,max}

    embraced               INTEGER NOT NULL DEFAULT 0,
    embraced_since         DATETIME,
    madness                INTEGER NOT NULL DEFAULT 0,
    stilled                INTEGER NOT NULL DEFAULT 0,

    bonded_warder_id       INTEGER NOT NULL DEFAULT 0,
    bonded_aes_sedai_id    INTEGER NOT NULL DEFAULT 0,
    held_angreal_id        INTEGER NOT NULL DEFAULT 0,
    held_saangreal_id      INTEGER NOT NULL DEFAULT 0,
    circle_id              INTEGER NOT NULL DEFAULT 0,

    aes_sedai_oaths        INTEGER NOT NULL DEFAULT 0, -- OathFlag bitmask
    ageless                INTEGER NOT NULL DEFAULT 0,

    damane_collar_to       INTEGER NOT NULL DEFAULT 0,

    UNIQUE(owner_kind, owner_id)
);
CREATE INDEX channeling_owner_idx ON channeling(owner_kind, owner_id);
