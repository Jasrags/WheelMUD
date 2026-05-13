-- 0055_builder_zones.sql
--
-- Phase G #33 — per-zone builder grants. One row per
-- (character, zone) pair authorising that character to mutate the
-- zone via OLC verbs (#34 redit / oedit / medit / zedit). AuthAdmin
-- bypasses this table entirely; AuthPlayer with a row here gains
-- builder powers scoped strictly to the listed zones.
--
-- granted_by snapshots the actor character_id at write time so a
-- later revoke or audit walk has provenance even if the granter
-- has since had their auth_level reduced. granted_at is unix seconds
-- (same shape as admin_audit.ts) for cheap range scans.
--
-- No FK constraints on character_id / zone_id / granted_by — mirrors
-- the soft-FK pattern in admin_audit (0029) and account_logins
-- (0036). Verb layer validates target existence before insert.
--
-- PRIMARY KEY (character_id, zone_id) makes Grant idempotent via
-- INSERT OR REPLACE and gives Has() an O(log n) index hit on the
-- per-character permission check fired from Session.IsBuilderFor.
--
-- idx_builder_zones_zone supports the reverse "who can edit this
-- zone?" view that admin tooling needs for grants viewer.
--
-- Forward-only per CLAUDE.md (no down migration).

CREATE TABLE builder_zones (
    character_id INTEGER NOT NULL,
    zone_id      INTEGER NOT NULL,
    granted_by   INTEGER NOT NULL DEFAULT 0,
    granted_at   INTEGER NOT NULL,
    PRIMARY KEY (character_id, zone_id)
);

CREATE INDEX idx_builder_zones_zone ON builder_zones(zone_id);
