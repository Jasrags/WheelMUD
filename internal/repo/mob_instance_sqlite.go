package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

type SQLiteMobInstanceRepo struct {
	db DBTX
}

// NewSQLiteMobInstanceRepo builds a repo bound to the given queryer.
// Pass a *sql.DB for the runtime path, or a *sql.Tx when batching
// inserts inside a transaction (the world loader does this).
func NewSQLiteMobInstanceRepo(db DBTX) *SQLiteMobInstanceRepo {
	return &SQLiteMobInstanceRepo{db: db}
}

func (r *SQLiteMobInstanceRepo) Create(ctx context.Context, m creature.MobInstance) (creature.MobInstance, error) {
	if m.TemplateID == 0 {
		return creature.MobInstance{}, fmt.Errorf("mob_instance.Create: TemplateID required")
	}
	if m.SpawnedAt.IsZero() {
		m.SpawnedAt = time.Now().UTC()
	}
	var roomID any
	if m.Core.CurrentRoomID != 0 {
		roomID = m.Core.CurrentRoomID
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO mob_instances(
			template_id, room_id,
			hp_current, subdual, conditions, position_flags,
			spawned_at, bound_reset_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.TemplateID, roomID,
		m.Core.HPCurrent, m.Core.Subdual, m.Core.Conditions, m.Core.Position,
		m.SpawnedAt, m.BoundResetID,
	)
	if err != nil {
		return creature.MobInstance{}, fmt.Errorf("insert mob_instance: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return creature.MobInstance{}, fmt.Errorf("last insert id: %w", err)
	}
	m.ID = id
	m.Core.ID = id
	return m, nil
}

// instanceSelect joins mob_instances with mob_templates so the
// returned MobInstance.Core has the template-derived fields callers
// need to render (Name, ShortDesc, HPMax, Defense). Live mutable
// fields (HPCurrent, conditions, position) come from the instance
// row. Bigger Core fields (abilities, saves, full DR/Resists) stay
// unloaded — callers that need them fetch the template via
// MobTemplateRepo.GetByID using TemplateID.
const instanceSelect = `
	SELECT i.id, i.template_id, i.room_id,
	       i.hp_current, i.subdual, i.conditions, i.position_flags,
	       i.spawned_at, i.bound_reset_id,
	       t.name, t.short_desc, t.hp_max, t.defense
	FROM mob_instances i
	JOIN mob_templates t ON t.id = i.template_id`

func (r *SQLiteMobInstanceRepo) GetByID(ctx context.Context, id int64) (creature.MobInstance, error) {
	row := r.db.QueryRowContext(ctx, instanceSelect+` WHERE i.id = ?`, id)
	m, err := scanInstance(row)
	if errors.Is(err, sql.ErrNoRows) {
		return creature.MobInstance{}, ErrInstanceNotFound
	}
	return m, err
}

func (r *SQLiteMobInstanceRepo) ListInRoom(ctx context.Context, roomID int64) ([]creature.MobInstance, error) {
	rows, err := r.db.QueryContext(ctx, instanceSelect+` WHERE i.room_id = ? ORDER BY i.id`, roomID)
	if err != nil {
		return nil, fmt.Errorf("list mob_instances: %w", err)
	}
	defer rows.Close()
	var out []creature.MobInstance
	for rows.Next() {
		m, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *SQLiteMobInstanceRepo) UpdateLive(ctx context.Context, id int64, hp, subdual int32, cond creature.Condition, pos creature.PositionFlags) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE mob_instances SET hp_current = ?, subdual = ?, conditions = ?, position_flags = ?
		 WHERE id = ?`,
		hp, subdual, cond, pos, id,
	)
	if err != nil {
		return fmt.Errorf("update mob_instance live: %w", err)
	}
	return checkRowsAffected(res, ErrInstanceNotFound)
}

func (r *SQLiteMobInstanceRepo) UpdateRoom(ctx context.Context, id, roomID int64) error {
	var arg any
	if roomID != 0 {
		arg = roomID
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE mob_instances SET room_id = ? WHERE id = ?`, arg, id,
	)
	if err != nil {
		return fmt.Errorf("update mob_instance room: %w", err)
	}
	return checkRowsAffected(res, ErrInstanceNotFound)
}

func (r *SQLiteMobInstanceRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM mob_instances WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete mob_instance: %w", err)
	}
	return checkRowsAffected(res, ErrInstanceNotFound)
}

// scanner is the subset of *sql.Row / *sql.Rows that scanInstance
// needs, so the helper can serve both single-row and iter callers.
type scanner interface {
	Scan(dest ...any) error
}

func scanInstance(s scanner) (creature.MobInstance, error) {
	var (
		m         creature.MobInstance
		rid       sql.NullInt64
		shortDesc string
	)
	if err := s.Scan(
		&m.ID, &m.TemplateID, &rid,
		&m.Core.HPCurrent, &m.Core.Subdual, &m.Core.Conditions, &m.Core.Position,
		&m.SpawnedAt, &m.BoundResetID,
		&m.Core.Name, &shortDesc, &m.Core.HPMax, &m.Core.Defense,
	); err != nil {
		return creature.MobInstance{}, err
	}
	if rid.Valid {
		m.Core.CurrentRoomID = rid.Int64
	}
	m.Core.ID = m.ID
	_ = shortDesc // not stored on Core today; reserved for `examine` (§10)
	return m, nil
}

func checkRowsAffected(res sql.Result, notFound error) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return notFound
	}
	return nil
}
