package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type SQLiteMobRepo struct {
	db *sql.DB
}

func NewSQLiteMobRepo(db *sql.DB) *SQLiteMobRepo {
	return &SQLiteMobRepo{db: db}
}

func (r *SQLiteMobRepo) Create(ctx context.Context, m Mob) (Mob, error) {
	if m.ExternalID == "" {
		return Mob{}, ErrInvalidExternalID
	}
	if m.NameLower == "" {
		m.NameLower = strings.ToLower(m.Name)
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	var roomID any
	if m.RoomID != 0 {
		roomID = m.RoomID
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO mobs(external_id, name, name_lower, short_desc, room_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		m.ExternalID, m.Name, m.NameLower, m.ShortDesc, roomID, m.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Mob{}, ErrDuplicateExternalID
		}
		return Mob{}, fmt.Errorf("insert mob: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Mob{}, fmt.Errorf("last insert id: %w", err)
	}
	m.ID = id
	return m, nil
}

func (r *SQLiteMobRepo) ListInRoom(ctx context.Context, roomID int64) ([]Mob, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, external_id, name, name_lower, short_desc, room_id, created_at
		 FROM mobs
		 WHERE room_id = ?
		 ORDER BY name_lower`,
		roomID,
	)
	if err != nil {
		return nil, fmt.Errorf("list mobs: %w", err)
	}
	defer rows.Close()
	var out []Mob
	for rows.Next() {
		var (
			m   Mob
			rid sql.NullInt64
		)
		if err := rows.Scan(&m.ID, &m.ExternalID, &m.Name, &m.NameLower, &m.ShortDesc, &rid, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan mob row: %w", err)
		}
		if rid.Valid {
			m.RoomID = rid.Int64
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
