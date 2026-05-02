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

const exitSelectCols = `id, from_room_id, to_room_id, direction, ` +
	`closed, locked, pickable, hidden, nopass, ` +
	`key_external_id, lock_difficulty, description`

func (r *SQLiteExitRepo) ListFrom(ctx context.Context, fromRoomID int64) ([]Exit, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+exitSelectCols+` FROM exits WHERE from_room_id = ? ORDER BY direction`,
		fromRoomID,
	)
	if err != nil {
		return nil, fmt.Errorf("list exits: %w", err)
	}
	defer rows.Close()
	var out []Exit
	for rows.Next() {
		var e Exit
		if err := scanExitInto(rows, &e); err != nil {
			return nil, fmt.Errorf("scan exit row: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *SQLiteExitRepo) Create(ctx context.Context, e Exit) (Exit, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO exits(from_room_id, to_room_id, direction,
			closed, locked, pickable, hidden, nopass,
			key_external_id, lock_difficulty, description)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.FromRoomID, e.ToRoomID, e.Direction,
		boolToInt(e.Flags.Closed), boolToInt(e.Flags.Locked),
		boolToInt(e.Flags.Pickable), boolToInt(e.Flags.Hidden),
		boolToInt(e.Flags.NoPass),
		e.KeyExternalID, e.LockDifficulty, e.Description,
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
		`SELECT `+exitSelectCols+` FROM exits WHERE from_room_id = ? AND direction = ?`,
		fromRoomID, direction,
	)
	var e Exit
	if err := scanExitInto(row, &e); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Exit{}, ErrExitNotFound
		}
		return Exit{}, fmt.Errorf("scan exit: %w", err)
	}
	return e, nil
}

// rowScanner is the common surface of *sql.Row and *sql.Rows. Used so
// scanExitInto can serve both single-row and result-set callers from
// one place; column-list changes only need to land here.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanExitInto populates *e from the next row of s. Returns the raw
// driver error (including sql.ErrNoRows) so callers can translate to
// the right domain error themselves.
func scanExitInto(s rowScanner, e *Exit) error {
	var closed, locked, pickable, hidden, nopassFlag int
	if err := s.Scan(
		&e.ID, &e.FromRoomID, &e.ToRoomID, &e.Direction,
		&closed, &locked, &pickable, &hidden, &nopassFlag,
		&e.KeyExternalID, &e.LockDifficulty, &e.Description,
	); err != nil {
		return err
	}
	e.Flags = ExitFlags{
		Closed:   closed != 0,
		Locked:   locked != 0,
		Pickable: pickable != 0,
		Hidden:   hidden != 0,
		NoPass:   nopassFlag != 0,
	}
	return nil
}
