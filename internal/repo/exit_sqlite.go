package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type SQLiteExitRepo struct {
	db *sql.DB
}

func NewSQLiteExitRepo(db *sql.DB) *SQLiteExitRepo {
	return &SQLiteExitRepo{db: db}
}

const exitSelectCols = `id, from_room_id, to_room_id, direction, ` +
	`closed, locked, pickable, hidden, nopass, ` +
	`key_external_id, lock_difficulty, description, ` +
	`authored_closed, authored_locked`

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
			key_external_id, lock_difficulty, description,
			authored_closed, authored_locked)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.FromRoomID, e.ToRoomID, e.Direction,
		boolToInt(e.Flags.Closed), boolToInt(e.Flags.Locked),
		boolToInt(e.Flags.Pickable), boolToInt(e.Flags.Hidden),
		boolToInt(e.Flags.NoPass),
		e.KeyExternalID, e.LockDifficulty, e.Description,
		boolToInt(e.Flags.AuthoredClosed), boolToInt(e.Flags.AuthoredLocked),
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

func (r *SQLiteExitRepo) UpdateFlags(ctx context.Context, exitID int64, closed, locked bool) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE exits SET closed = ?, locked = ? WHERE id = ?`,
		boolToInt(closed), boolToInt(locked), exitID,
	)
	if err != nil {
		return fmt.Errorf("update exit flags: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update exit flags rows: %w", err)
	}
	if n == 0 {
		return ErrExitNotFound
	}
	return nil
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
	var authoredClosed, authoredLocked int
	if err := s.Scan(
		&e.ID, &e.FromRoomID, &e.ToRoomID, &e.Direction,
		&closed, &locked, &pickable, &hidden, &nopassFlag,
		&e.KeyExternalID, &e.LockDifficulty, &e.Description,
		&authoredClosed, &authoredLocked,
	); err != nil {
		return err
	}
	e.Flags = ExitFlags{
		Closed:         closed != 0,
		Locked:         locked != 0,
		Pickable:       pickable != 0,
		Hidden:         hidden != 0,
		NoPass:         nopassFlag != 0,
		AuthoredClosed: authoredClosed != 0,
		AuthoredLocked: authoredLocked != 0,
	}
	return nil
}

// RestoreAuthored issues one zone-scoped UPDATE that snaps every
// exit's runtime closed/locked back to its authored value, but only
// for rows currently divergent — keeps the row count returned to
// callers honest for telemetry.
//
// fromRoomIDs are DB-internal int64 keys (typically from
// RoomRepo.ListIDsByZone), never user input — the IN-clause below
// uses parameterised placeholders for them anyway, so no SQL
// injection surface exists either way.
func (r *SQLiteExitRepo) RestoreAuthored(ctx context.Context, fromRoomIDs []int64) (int, error) {
	if len(fromRoomIDs) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(fromRoomIDs))
	args := make([]any, len(fromRoomIDs))
	for i, id := range fromRoomIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE exits
		 SET closed = authored_closed, locked = authored_locked
		 WHERE from_room_id IN (`+strings.Join(placeholders, ",")+`)
		   AND (closed != authored_closed OR locked != authored_locked)`,
		args...,
	)
	if err != nil {
		return 0, fmt.Errorf("restore authored exits: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("restore authored exits rows: %w", err)
	}
	return int(n), nil
}
