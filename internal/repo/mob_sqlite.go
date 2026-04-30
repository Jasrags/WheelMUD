package repo

import (
	"context"
	"database/sql"
	"fmt"
)

type SQLiteMobRepo struct {
	db *sql.DB
}

func NewSQLiteMobRepo(db *sql.DB) *SQLiteMobRepo {
	return &SQLiteMobRepo{db: db}
}

func (r *SQLiteMobRepo) ListInRoom(ctx context.Context, roomID int64) ([]Mob, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, name_lower, short_desc, room_id, created_at
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
		if err := rows.Scan(&m.ID, &m.Name, &m.NameLower, &m.ShortDesc, &rid, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan mob row: %w", err)
		}
		if rid.Valid {
			m.RoomID = rid.Int64
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
