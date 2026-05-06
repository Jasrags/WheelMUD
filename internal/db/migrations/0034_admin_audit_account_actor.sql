-- 0034_admin_audit_account_actor.sql
--
-- Extends admin_audit so the post-login account menu (slice 1b
-- onward — delete-character, password change, settings, security)
-- can record rows attributed to the account itself rather than a
-- character. The session at that point is pre-promotion: AccountID
-- is set, CharacterID is 0.
--
-- Schema choices:
--   * actor_account_id is the new join key for account-mode rows.
--     Existing rows backfill to 0; new character-mode rows continue
--     to leave it 0.
--   * actor_type discriminates 'character' (default — every existing
--     row + every future character-mode admin verb) from 'account'
--     (account-menu actions). Stored as TEXT for forensic legibility
--     when reading the table directly.
--   * The account_idx covers (actor_account_id, ts) so a future
--     "show me everything this account did" view scans the right
--     side of the table.
--
-- Forward-only per CLAUDE.md (no down migration).

ALTER TABLE admin_audit ADD COLUMN actor_account_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE admin_audit ADD COLUMN actor_type       TEXT    NOT NULL DEFAULT 'character';
CREATE INDEX admin_audit_account_idx ON admin_audit(actor_account_id, ts);
