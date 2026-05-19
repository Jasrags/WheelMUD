package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SQLiteFlowStateRepo persists flow_state rows. Save uses an UPSERT
// so update-in-place doesn't touch the cap-eviction path. The cap
// (MaxFlowStatesPerAccount) is enforced inside a transaction so an
// eviction and the new insert land atomically.
type SQLiteFlowStateRepo struct {
	db *sql.DB
}

func NewSQLiteFlowStateRepo(db *sql.DB) *SQLiteFlowStateRepo {
	return &SQLiteFlowStateRepo{db: db}
}

func (r *SQLiteFlowStateRepo) Save(ctx context.Context, fs FlowState) error {
	if fs.AccountID == 0 || fs.FlowID == "" {
		return fmt.Errorf("flow_state: AccountID and FlowID required")
	}
	if fs.UpdatedAt.IsZero() {
		fs.UpdatedAt = time.Now().UTC()
	}
	if fs.StartedAt.IsZero() {
		fs.StartedAt = fs.UpdatedAt
	}
	if fs.Values == nil {
		fs.Values = map[string]string{}
	}
	valuesJSON, err := json.Marshal(fs.Values)
	if err != nil {
		return fmt.Errorf("marshal flow_state.values: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin flow_state tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Detect update-vs-insert: an existing row never triggers
	// eviction, so we only enforce the cap on inserts.
	var probe int
	probeErr := tx.QueryRowContext(ctx,
		`SELECT 1 FROM flow_state WHERE account_id = ? AND flow_id = ?`,
		fs.AccountID, fs.FlowID,
	).Scan(&probe)
	if probeErr != nil && !errors.Is(probeErr, sql.ErrNoRows) {
		return fmt.Errorf("probe flow_state: %w", probeErr)
	}
	inserting := errors.Is(probeErr, sql.ErrNoRows)

	if inserting {
		// Count current rows for this account; if at cap, evict the
		// oldest. The composite-PK upsert below handles the insert.
		var count int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM flow_state WHERE account_id = ?`,
			fs.AccountID,
		).Scan(&count); err != nil {
			return fmt.Errorf("count flow_state: %w", err)
		}
		if count >= MaxFlowStatesPerAccount {
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM flow_state
				 WHERE account_id = ?
				 AND flow_id = (
				   SELECT flow_id FROM flow_state
				   WHERE account_id = ?
				   ORDER BY updated_at ASC, flow_id ASC
				   LIMIT 1
				 )`,
				fs.AccountID, fs.AccountID,
			); err != nil {
				return fmt.Errorf("evict flow_state: %w", err)
			}
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO flow_state(account_id, flow_id, current_step, values_json, started_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, flow_id) DO UPDATE SET
		   current_step = excluded.current_step,
		   values_json  = excluded.values_json,
		   updated_at   = excluded.updated_at`,
		fs.AccountID, fs.FlowID, fs.CurrentStep, string(valuesJSON),
		fs.StartedAt.Unix(), fs.UpdatedAt.Unix(),
	); err != nil {
		return fmt.Errorf("upsert flow_state: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit flow_state: %w", err)
	}
	return nil
}

func (r *SQLiteFlowStateRepo) Load(ctx context.Context, accountID int64, flowID string) (FlowState, error) {
	var (
		fs                   FlowState
		valuesJSON           string
		startedAt, updatedAt int64
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT account_id, flow_id, current_step, values_json, started_at, updated_at
		 FROM flow_state WHERE account_id = ? AND flow_id = ?`,
		accountID, flowID,
	).Scan(&fs.AccountID, &fs.FlowID, &fs.CurrentStep, &valuesJSON, &startedAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return FlowState{}, ErrFlowStateNotFound
	}
	if err != nil {
		return FlowState{}, fmt.Errorf("query flow_state: %w", err)
	}
	if err := json.Unmarshal([]byte(valuesJSON), &fs.Values); err != nil {
		return FlowState{}, fmt.Errorf("unmarshal flow_state.values: %w", err)
	}
	fs.StartedAt = time.Unix(startedAt, 0).UTC()
	fs.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return fs, nil
}

func (r *SQLiteFlowStateRepo) Delete(ctx context.Context, accountID int64, flowID string) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM flow_state WHERE account_id = ? AND flow_id = ?`,
		accountID, flowID,
	); err != nil {
		return fmt.Errorf("delete flow_state: %w", err)
	}
	return nil
}

func (r *SQLiteFlowStateRepo) ListByAccount(ctx context.Context, accountID int64) ([]FlowState, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT account_id, flow_id, current_step, values_json, started_at, updated_at
		 FROM flow_state WHERE account_id = ?
		 ORDER BY updated_at DESC, flow_id ASC`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("query flow_state list: %w", err)
	}
	defer rows.Close()

	var out []FlowState
	for rows.Next() {
		var (
			fs                   FlowState
			valuesJSON           string
			startedAt, updatedAt int64
		)
		if err := rows.Scan(&fs.AccountID, &fs.FlowID, &fs.CurrentStep,
			&valuesJSON, &startedAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan flow_state: %w", err)
		}
		if err := json.Unmarshal([]byte(valuesJSON), &fs.Values); err != nil {
			return nil, fmt.Errorf("unmarshal flow_state.values: %w", err)
		}
		fs.StartedAt = time.Unix(startedAt, 0).UTC()
		fs.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		out = append(out, fs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate flow_state: %w", err)
	}
	return out, nil
}
