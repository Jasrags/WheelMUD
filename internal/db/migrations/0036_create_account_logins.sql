-- 0036_create_account_logins.sql
--
-- account_logins — append-only per-account authentication-event log
-- backing the §6 post-login account-menu "security" sub-menu (slice 4).
-- One row per login outcome (success / failure / lockout) and per
-- account-menu kick-sessions invocation.
--
-- Schema choices mirror admin_audit (0029):
--   * No FK on account_id — accounts may be deleted in the future; the
--     audit trail must outlive them. account_id = 0 is reserved for
--     system rows (none written today).
--   * ts is unix seconds (UTC) for cheap range scans.
--   * outcome is TEXT (not enum) so a forensic reader sees the value
--     directly. Values: 'success' | 'failure' | 'lockout' | 'kick'.
--   * info is a short fixed-vocabulary note ("wrong password",
--     "kicked by other-session"). NEVER carries the typed password.
--   * Indexed by (account_id, ts) so the menu's
--     ListRecentByAccount(accountID, limit) is a tail scan.
--
-- Forward-only per CLAUDE.md (no down migration).

CREATE TABLE account_logins (
    id              INTEGER PRIMARY KEY,
    account_id      INTEGER NOT NULL,
    ts              INTEGER NOT NULL,
    remote_address  TEXT    NOT NULL DEFAULT '',
    outcome         TEXT    NOT NULL,
    info            TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX account_logins_account_idx ON account_logins(account_id, ts);
