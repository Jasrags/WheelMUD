package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type SQLiteRoomRepo struct {
	db *sql.DB
}

func NewSQLiteRoomRepo(db *sql.DB) *SQLiteRoomRepo {
	return &SQLiteRoomRepo{db: db}
}

func (r *SQLiteRoomRepo) FindByID(ctx context.Context, id int64) (Room, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, external_id, name, short_desc, long_desc, created_at
		 FROM rooms WHERE id = ?`,
		id,
	)
	return scanRoom(row)
}

func (r *SQLiteRoomRepo) FindByExternalID(ctx context.Context, externalID string) (Room, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, external_id, name, short_desc, long_desc, created_at
		 FROM rooms WHERE external_id = ?`,
		externalID,
	)
	return scanRoom(row)
}

func (r *SQLiteRoomRepo) Create(ctx context.Context, room Room) (Room, error) {
	if room.ExternalID == "" {
		return Room{}, ErrInvalidExternalID
	}
	if room.CreatedAt.IsZero() {
		room.CreatedAt = time.Now().UTC()
	}

	if room.ID != 0 {
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO rooms(id, external_id, name, short_desc, long_desc, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			room.ID, room.ExternalID, room.Name, room.ShortDesc, room.LongDesc, room.CreatedAt,
		)
		if err != nil {
			return Room{}, mapRoomInsertErr(err)
		}
		return room, nil
	}

	res, err := r.db.ExecContext(ctx,
		`INSERT INTO rooms(external_id, name, short_desc, long_desc, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		room.ExternalID, room.Name, room.ShortDesc, room.LongDesc, room.CreatedAt,
	)
	if err != nil {
		return Room{}, mapRoomInsertErr(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Room{}, fmt.Errorf("last insert id: %w", err)
	}
	room.ID = id
	return room, nil
}

func scanRoom(row *sql.Row) (Room, error) {
	var room Room
	err := row.Scan(&room.ID, &room.ExternalID, &room.Name, &room.ShortDesc, &room.LongDesc, &room.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Room{}, ErrRoomNotFound
	}
	if err != nil {
		return Room{}, fmt.Errorf("scan room: %w", err)
	}
	return room, nil
}

func mapRoomInsertErr(err error) error {
	if isUniqueViolation(err) {
		return ErrDuplicateExternalID
	}
	return fmt.Errorf("insert room: %w", err)
}
