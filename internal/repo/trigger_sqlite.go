package repo

import (
	"context"
	"fmt"
	"strings"
)

// SQLiteTriggerRepo persists triggers rows. Created in migration
// 0044.
type SQLiteTriggerRepo struct {
	db DBTX
}

func NewSQLiteTriggerRepo(db DBTX) *SQLiteTriggerRepo {
	return &SQLiteTriggerRepo{db: db}
}

const triggerSelectCols = `id, owner_kind, owner_id, event, match, action, payload, priority, consecutive_faults, disabled`

func scanTrigger(scanner interface {
	Scan(dest ...interface{}) error
}) (Trigger, error) {
	var t Trigger
	var kind, event string
	var disabled int
	if err := scanner.Scan(&t.ID, &kind, &t.OwnerID, &event, &t.Match, &t.Action, &t.Payload, &t.Priority, &t.ConsecutiveFaults, &disabled); err != nil {
		return Trigger{}, err
	}
	t.OwnerKind = TriggerOwnerKind(kind)
	t.Event = TriggerEvent(event)
	t.Disabled = disabled != 0
	return t, nil
}

func validateTrigger(t Trigger) error {
	if !ValidTriggerOwnerKind(t.OwnerKind) {
		return fmt.Errorf("%w: owner_kind=%q", ErrInvalidTrigger, t.OwnerKind)
	}
	if t.OwnerID == 0 {
		return fmt.Errorf("%w: owner_id must be non-zero", ErrInvalidTrigger)
	}
	if !ValidTriggerEvent(t.Event) {
		return fmt.Errorf("%w: event=%q", ErrInvalidTrigger, t.Event)
	}
	if strings.TrimSpace(t.Action) == "" {
		return fmt.Errorf("%w: action must be non-empty", ErrInvalidTrigger)
	}
	return nil
}

func (r *SQLiteTriggerRepo) Create(ctx context.Context, t Trigger) (Trigger, error) {
	if err := validateTrigger(t); err != nil {
		return Trigger{}, err
	}
	if t.Payload == "" {
		t.Payload = "{}"
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO triggers(owner_kind, owner_id, event, match, action, payload, priority)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		string(t.OwnerKind), t.OwnerID, string(t.Event), t.Match, t.Action, t.Payload, t.Priority)
	if err != nil {
		return Trigger{}, fmt.Errorf("insert trigger: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Trigger{}, fmt.Errorf("lastInsertId trigger: %w", err)
	}
	t.ID = id
	return t, nil
}

func (r *SQLiteTriggerRepo) ListByOwner(ctx context.Context, kind TriggerOwnerKind, ownerID int64) ([]Trigger, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+triggerSelectCols+` FROM triggers
		 WHERE owner_kind = ? AND owner_id = ?
		 ORDER BY priority DESC, id ASC`,
		string(kind), ownerID)
	if err != nil {
		return nil, fmt.Errorf("list triggers by owner: %w", err)
	}
	defer rows.Close()
	var out []Trigger
	for rows.Next() {
		t, err := scanTrigger(rows)
		if err != nil {
			return nil, fmt.Errorf("scan trigger: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate triggers: %w", err)
	}
	return out, nil
}

func (r *SQLiteTriggerRepo) ListAll(ctx context.Context) ([]Trigger, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+triggerSelectCols+` FROM triggers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list triggers: %w", err)
	}
	defer rows.Close()
	var out []Trigger
	for rows.Next() {
		t, err := scanTrigger(rows)
		if err != nil {
			return nil, fmt.Errorf("scan trigger: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate triggers: %w", err)
	}
	return out, nil
}

func (r *SQLiteTriggerRepo) RecordTriggerFault(ctx context.Context, id int64, faults int, disabled bool) error {
	d := 0
	if disabled {
		d = 1
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE triggers SET consecutive_faults = ?, disabled = ? WHERE id = ?`,
		faults, d, id)
	if err != nil {
		return fmt.Errorf("record trigger fault: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrTriggerNotFound
	}
	return nil
}

func (r *SQLiteTriggerRepo) ResetAllFaults(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE triggers SET consecutive_faults = 0, disabled = 0`); err != nil {
		return fmt.Errorf("reset trigger faults: %w", err)
	}
	return nil
}

func (r *SQLiteTriggerRepo) DeleteByOwner(ctx context.Context, kind TriggerOwnerKind, ownerID int64) error {
	if !ValidTriggerOwnerKind(kind) {
		return fmt.Errorf("%w: owner_kind=%q", ErrInvalidTrigger, kind)
	}
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM triggers WHERE owner_kind = ? AND owner_id = ?`,
		string(kind), ownerID); err != nil {
		return fmt.Errorf("delete triggers by owner: %w", err)
	}
	return nil
}
