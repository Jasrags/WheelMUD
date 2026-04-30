package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type SQLiteItemRepo struct {
	db *sql.DB
}

func NewSQLiteItemRepo(db *sql.DB) *SQLiteItemRepo {
	return &SQLiteItemRepo{db: db}
}

func (r *SQLiteItemRepo) Create(ctx context.Context, i Item) (Item, error) {
	if i.ExternalID == "" {
		return Item{}, ErrInvalidExternalID
	}
	if i.NameLower == "" {
		i.NameLower = strings.ToLower(i.Name)
	}
	if i.CreatedAt.IsZero() {
		i.CreatedAt = time.Now().UTC()
	}
	var roomID any
	if i.RoomID != 0 {
		roomID = i.RoomID
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO items(external_id, name, name_lower, short_desc, room_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		i.ExternalID, i.Name, i.NameLower, i.ShortDesc, roomID, i.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Item{}, ErrDuplicateExternalID
		}
		return Item{}, fmt.Errorf("insert item: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Item{}, fmt.Errorf("last insert id: %w", err)
	}
	i.ID = id
	return i, nil
}

func (r *SQLiteItemRepo) ListInRoom(ctx context.Context, roomID int64) ([]Item, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, external_id, name, name_lower, short_desc, room_id, created_at
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
		if err := rows.Scan(&i.ID, &i.ExternalID, &i.Name, &i.NameLower, &i.ShortDesc, &rid, &i.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan item row: %w", err)
		}
		if rid.Valid {
			i.RoomID = rid.Int64
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
