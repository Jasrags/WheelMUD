-- 0044_create_triggers.sql
--
-- Trigger / event subsystem (§15 / Phase F #29). One row per
-- declarative trigger attached to a mob_template or a room. The
-- trigger.Dispatcher subscribes to existing eventbus events
-- (world.PlayerEntered / PlayerSaid, combat.CombatHit / CombatDeath /
-- CharacterDied) and to the tick.Buckets.Phase pulse, looks up the
-- triggers attached to the relevant owners, and invokes a registered
-- ActionHandler.
--
-- This slice ships the pure dispatch surface. The action vocabulary
-- is intentionally tiny in V1 (`noop`/`say`/`emote`) — consumers
-- (NPC dialogue #30, quests #31, embedded Lua #32) extend the
-- handler registry but the schema and dispatcher do not change.
--
-- Schema choices:
--   * owner_kind is checked against ('mob_template','room'); items
--     are deferred until #32 when the action surface stabilises.
--   * owner_id is a soft FK (no REFERENCES) — mirrors admin_audit /
--     account_logins / weave_teachers convention. The world loader
--     rewrites all rows on every LoadAndSync.
--   * event is checked against the five canonical names
--     ('on_enter','on_say','on_attack','on_death','on_tick').
--   * match is event-specific text: `on_say` keyword (substring,
--     case-insensitive); `on_tick` bucket name (`phase`/`combat`/
--     `regen`/etc., default `phase`); ignored by other events.
--   * action is the ActionKind name. Validated against the
--     in-process actions registry at trigger.Registry.Reload time;
--     the schema is open-vocabulary so consumers can add handlers
--     without a migration.
--   * payload is action-defined JSON (e.g. {"text": "..."} for
--     say/emote). Default '{}'.
--   * priority orders triggers attached to the same (owner, event);
--     higher fires first.
--
-- The owner+event index is the dispatcher's hot path — every
-- relevant bus event resolves owners then asks for that owner's
-- triggers for the mapped event.
--
-- Forward-only per CLAUDE.md (no down migration).

CREATE TABLE triggers (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_kind  TEXT    NOT NULL CHECK (owner_kind IN ('mob_template','room')),
    owner_id    INTEGER NOT NULL,
    event       TEXT    NOT NULL CHECK (event IN ('on_enter','on_say','on_attack','on_death','on_tick')),
    match       TEXT    NOT NULL DEFAULT '',
    action      TEXT    NOT NULL,
    payload     TEXT    NOT NULL DEFAULT '{}',
    priority    INTEGER NOT NULL DEFAULT 0,
    created_at  INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX idx_triggers_owner_event ON triggers(owner_kind, owner_id, event);
CREATE INDEX idx_triggers_event ON triggers(event);
