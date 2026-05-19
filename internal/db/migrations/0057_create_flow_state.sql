-- +migrate up
-- §O.2 flow_state: per-account, per-flow runtime snapshot for the
-- internal/flow engine. Single row per (account_id, flow_id) so a
-- player has at most one in-flight instance of each flow at a time.
-- Save fires on every step transition, Delete on Completed/Cancelled.
-- Per-account row count is capped in the repo layer (LRU eviction
-- by updated_at) so a stuck wizard can't pile up rows.
--
-- values_json holds the State.Values map verbatim. current_step is
-- the StepID the runner is awaiting input for; empty current_step
-- never appears here because the row is deleted on completion.
-- started_at / updated_at are unix seconds for cheap index scans.
--
-- Forward-only per CLAUDE.md (no down migration).
CREATE TABLE flow_state (
    account_id   INTEGER NOT NULL,
    flow_id      TEXT    NOT NULL,
    current_step TEXT    NOT NULL,
    values_json  TEXT    NOT NULL DEFAULT '{}',
    started_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL,
    PRIMARY KEY (account_id, flow_id),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);
CREATE INDEX flow_state_account_updated_idx
    ON flow_state(account_id, updated_at DESC);
