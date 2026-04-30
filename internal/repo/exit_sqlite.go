package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type SQLiteExitRepo struct {
	db *sql.DB
}

func NewSQLiteExitRepo(db *sql.DB) *SQLiteExitRepo {
	return &SQLiteExitRepo{db: db}
}

func (r *SQLiteExitRepo) ListFrom(ctx context.Context, fromRoomID int64) ([]Exit, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, from_room_id, to_room_id, direction
		 FROM exits
		 WHERE from_room_id = ?
		 ORDER BY direction`,
		fromRoomID,
	)
	if err != nil {
		return nil, fmt.Errorf("list exits: %w", err)
	}
	defer rows.Close()
	var out []Exit
	for rows.Next() {
		var e Exit
		if err := rows.Scan(&e.ID, &e.FromRoomID, &e.ToRoomID, &e.Direction); err != nil {
			return nil, fmt.Errorf("scan exit row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *SQLiteExitRepo) Create(ctx context.Context, e Exit) (Exit, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO exits(from_room_id, to_room_id, direction)
		 VALUES (?, ?, ?)`,
		e.FromRoomID, e.ToRoomID, e.Direction,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Exit{}, ErrDuplicateExit
		}
		return Exit{}, fmt.Errorf("insert exit: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Exit{}, fmt.Errorf("last insert id: %w", err)
	}
	e.ID = id
	return e, nil
}

func (r *SQLiteExitRepo) FindByDirection(ctx context.Context, fromRoomID int64, direction string) (Exit, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, from_room_id, to_room_id, direction
		 FROM exits
		 WHERE from_room_id = ? AND direction = ?`,
		fromRoomID, direction,
	)
	var e Exit
	err := row.Scan(&e.ID, &e.FromRoomID, &e.ToRoomID, &e.Direction)
	if errors.Is(err, sql.ErrNoRows) {
		return Exit{}, ErrExitNotFound
	}
	if err != nil {
		return Exit{}, fmt.Errorf("scan exit: %w", err)
	}
	return e, nil
}
