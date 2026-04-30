package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type SQLiteRoomRepo struct {
	db *sql.DB
}

func NewSQLiteRoomRepo(db *sql.DB) *SQLiteRoomRepo {
	return &SQLiteRoomRepo{db: db}
}

func (r *SQLiteRoomRepo) FindByID(ctx context.Context, id int64) (Room, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, short_desc, long_desc, created_at
		 FROM rooms WHERE id = ?`,
		id,
	)
	var room Room
	err := row.Scan(&room.ID, &room.Name, &room.ShortDesc, &room.LongDesc, &room.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Room{}, ErrRoomNotFound
	}
	if err != nil {
		return Room{}, fmt.Errorf("scan room: %w", err)
	}
	return room, nil
}
