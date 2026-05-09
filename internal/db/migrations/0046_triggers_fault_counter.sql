-- 0046_triggers_fault_counter.sql
--
-- Per-trigger consecutive-fault counter + auto-disable flag (§15 /
-- Phase F #32 slice 1). Both columns are engine-managed; the world
-- loader resets `consecutive_faults` to 0 and `disabled` to 0 on
-- every LoadAndSync so a re-deploy always starts triggers in the
-- enabled state.
--
-- Behavior at runtime (see internal/trigger/runner.go):
--   * Before invoking a handler: skip if disabled = 1.
--   * If a Lua action handler reports a fault (syntax / runtime /
--     instruction-cap / timeout), increment consecutive_faults.
--     A successful invocation resets it to 0.
--   * At threshold (5 consecutive faults) the engine sets
--     disabled = 1 and persists. Recovery is admin-driven (direct
--     SQL or future `tedit` reset).
--
-- The trigger surface intentionally does not yet have a recovery
-- verb — Slice 4 / Phase G #34 OLC work introduces `tedit` with a
-- "reset faults" subcommand. For now operators reset via
--   UPDATE triggers SET consecutive_faults = 0, disabled = 0
--   WHERE id = ?;
--
-- Forward-only per CLAUDE.md (no down migration).

ALTER TABLE triggers ADD COLUMN consecutive_faults INTEGER NOT NULL DEFAULT 0;
ALTER TABLE triggers ADD COLUMN disabled           INTEGER NOT NULL DEFAULT 0;
