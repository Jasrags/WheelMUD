package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SQLiteAccountRepo persists accounts in a SQL database. The interface is
// driver-agnostic — tested with modernc.org/sqlite, but any database/sql
// driver that supports the standard `?` placeholder works.
type SQLiteAccountRepo struct {
	db *sql.DB
}

func NewSQLiteAccountRepo(db *sql.DB) *SQLiteAccountRepo {
	return &SQLiteAccountRepo{db: db}
}

func (r *SQLiteAccountRepo) Create(ctx context.Context, a Account) (Account, error) {
	a.UsernameLower = strings.ToLower(a.Username)
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO accounts(username, username_lower, password_hash, created_at)
		 VALUES (?, ?, ?, ?)`,
		a.Username, a.UsernameLower, a.PasswordHash, a.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Account{}, ErrDuplicateUsername
		}
		return Account{}, fmt.Errorf("insert account: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Account{}, fmt.Errorf("last insert id: %w", err)
	}
	a.ID = id
	return a, nil
}

func (r *SQLiteAccountRepo) FindByUsername(ctx context.Context, username string) (Account, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, username, username_lower, password_hash, created_at,
		        last_login_at, failed_login_count, locked_until
		 FROM accounts
		 WHERE username_lower = ?`,
		strings.ToLower(username),
	)
	var (
		a           Account
		lastLogin   sql.NullTime
		lockedUntil sql.NullTime
	)
	err := row.Scan(&a.ID, &a.Username, &a.UsernameLower, &a.PasswordHash,
		&a.CreatedAt, &lastLogin, &a.FailedLoginCount, &lockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("scan account: %w", err)
	}
	if lastLogin.Valid {
		t := lastLogin.Time
		a.LastLoginAt = &t
	}
	if lockedUntil.Valid {
		t := lockedUntil.Time
		a.LockedUntil = &t
	}
	return a, nil
}

func (r *SQLiteAccountRepo) RecordLoginSuccess(ctx context.Context, id int64, when time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE accounts
		 SET last_login_at = ?, failed_login_count = 0, locked_until = NULL
		 WHERE id = ?`,
		when, id,
	)
	if err != nil {
		return fmt.Errorf("record login success: %w", err)
	}
	return nil
}

func (r *SQLiteAccountRepo) RecordLoginFailure(ctx context.Context, id int64, lockedUntil time.Time) error {
	if lockedUntil.IsZero() {
		_, err := r.db.ExecContext(ctx,
			`UPDATE accounts SET failed_login_count = failed_login_count + 1 WHERE id = ?`,
			id,
		)
		if err != nil {
			return fmt.Errorf("record login failure: %w", err)
		}
		return nil
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE accounts
		 SET failed_login_count = failed_login_count + 1, locked_until = ?
		 WHERE id = ?`,
		lockedUntil, id,
	)
	if err != nil {
		return fmt.Errorf("record login failure with lockout: %w", err)
	}
	return nil
}

func (r *SQLiteAccountRepo) UpdatePasswordHash(ctx context.Context, id int64, newHash string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE accounts SET password_hash = ? WHERE id = ?`,
		newHash, id,
	)
	if err != nil {
		return fmt.Errorf("update password hash: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update password hash rows: %w", err)
	}
	if n == 0 {
		return ErrAccountNotFound
	}
	return nil
}

// isUniqueViolation detects a SQLite unique-constraint error without
// importing the driver-specific error type. modernc.org/sqlite formats
// unique violations as "constraint failed: UNIQUE constraint failed: ...".
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed: UNIQUE")
}
