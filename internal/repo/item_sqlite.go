package repo

import (
	"context"
	"database/sql"
	"fmt"
)

type SQLiteItemRepo struct {
	db *sql.DB
}

func NewSQLiteItemRepo(db *sql.DB) *SQLiteItemRepo {
	return &SQLiteItemRepo{db: db}
}

func (r *SQLiteItemRepo) ListInRoom(ctx context.Context, roomID int64) ([]Item, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, name_lower, short_desc, room_id, created_at
		 FROM items
		 WHERE room_id = ?
		 ORDER BY name_lower`,
		roomID,
	)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var (
			i   Item
			rid sql.NullInt64
		)
		if err := rows.Scan(&i.ID, &i.Name, &i.NameLower, &i.ShortDesc, &rid, &i.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan item row: %w", err)
		}
		if rid.Valid {
			i.RoomID = rid.Int64
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
