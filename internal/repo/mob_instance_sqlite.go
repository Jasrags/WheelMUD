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

func (r *SQLiteMobInstanceRepo) ListSpawned(ctx context.Context, limit int) ([]creature.MobInstance, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx,
		instanceSelect+` WHERE i.room_id IS NOT NULL AND i.room_id != 0 ORDER BY i.id ASC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list spawned mob_instances: %w", err)
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

// UpdateRoom executes UPDATE + INSERT trail + cap-prune as three
// statements over DBTX. DBTX has no BeginTx, so callers that need
// strict atomicity (e.g. zone-reset relocations) must construct the
// repo with a *sql.Tx. Outside a tx, a partial failure between the
// INSERT and the cap-prune leaves the table at most one row over
// MobTrailCap until the next successful UpdateRoom on that mob —
// observable drift, not corruption.
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
	if err := checkRowsAffected(res, ErrInstanceNotFound); err != nil {
		return err
	}
	if roomID == 0 {
		// Removed from world but not despawned. No trail entry; the
		// mob is no longer in any room to leave a footprint in.
		return nil
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO mob_trails(mob_id, room_id) VALUES (?, ?)`, id, roomID,
	); err != nil {
		return fmt.Errorf("insert mob_trail: %w", err)
	}
	// Cap the ring buffer at MobTrailCap. The ORDER BY + LIMIT must
	// stay inside the subquery — SQLite's outer DELETE does not
	// accept ORDER BY/LIMIT directly, and lifting them would change
	// semantics. id DESC tie-breaks same-second inserts, since
	// CURRENT_TIMESTAMP only resolves to the second.
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM mob_trails
		 WHERE mob_id = ?
		   AND id NOT IN (
		       SELECT id FROM mob_trails
		       WHERE mob_id = ?
		       ORDER BY ts DESC, id DESC
		       LIMIT ?
		   )`,
		id, id, MobTrailCap,
	); err != nil {
		return fmt.Errorf("prune mob_trails: %w", err)
	}
	return nil
}

func (r *SQLiteMobInstanceRepo) RecentTrails(ctx context.Context, mobID int64, limit int) ([]MobTrail, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT mob_id, room_id, ts
		 FROM mob_trails
		 WHERE mob_id = ?
		 ORDER BY ts DESC, id DESC
		 LIMIT ?`,
		mobID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query mob_trails: %w", err)
	}
	defer rows.Close()
	var out []MobTrail
	for rows.Next() {
		var t MobTrail
		if err := rows.Scan(&t.MobID, &t.RoomID, &t.At); err != nil {
			return nil, fmt.Errorf("scan mob_trail: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
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
