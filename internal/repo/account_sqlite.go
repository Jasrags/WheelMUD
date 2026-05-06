package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

// accountSelectCols lists every column read by FindByUsername / FindByID
// in the same order scanAccountRow consumes. settings_json (0035) lives
// at the tail; new columns belong before it only if scanAccountRow
// changes in lockstep.
const accountSelectCols = `id, username, username_lower, password_hash, created_at,
	last_login_at, failed_login_count, locked_until, settings_json`

func (r *SQLiteAccountRepo) Create(ctx context.Context, a Account) (Account, error) {
	a.UsernameLower = strings.ToLower(a.Username)
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	settingsJSON, err := marshalAccountSettings(a.Settings)
	if err != nil {
		return Account{}, fmt.Errorf("marshal account settings: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO accounts(username, username_lower, password_hash, created_at, settings_json)
		 VALUES (?, ?, ?, ?, ?)`,
		a.Username, a.UsernameLower, a.PasswordHash, a.CreatedAt, settingsJSON,
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
		`SELECT `+accountSelectCols+` FROM accounts WHERE username_lower = ?`,
		strings.ToLower(username),
	)
	a, err := scanAccountRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("scan account: %w", err)
	}
	return a, nil
}

func (r *SQLiteAccountRepo) FindByID(ctx context.Context, id int64) (Account, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+accountSelectCols+` FROM accounts WHERE id = ?`,
		id,
	)
	a, err := scanAccountRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("scan account: %w", err)
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

func (r *SQLiteAccountRepo) UpdateSettings(ctx context.Context, id int64, s AccountSettings) error {
	settingsJSON, err := marshalAccountSettings(s)
	if err != nil {
		return fmt.Errorf("marshal account settings: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE accounts SET settings_json = ? WHERE id = ?`,
		settingsJSON, id,
	)
	if err != nil {
		return fmt.Errorf("update account settings: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update account settings rows: %w", err)
	}
	if n == 0 {
		return ErrAccountNotFound
	}
	return nil
}

// scanAccountRow consumes one row from accountSelectCols. Both rowScanner
// shapes (sql.Row / sql.Rows) satisfy the Scan signature, so the helper
// is reused across FindByUsername and FindByID without an interface
// allocation.
type accountRowScanner interface {
	Scan(dest ...any) error
}

func scanAccountRow(row accountRowScanner) (Account, error) {
	var (
		a            Account
		lastLogin    sql.NullTime
		lockedUntil  sql.NullTime
		settingsJSON string
	)
	if err := row.Scan(&a.ID, &a.Username, &a.UsernameLower, &a.PasswordHash,
		&a.CreatedAt, &lastLogin, &a.FailedLoginCount, &lockedUntil,
		&settingsJSON); err != nil {
		return Account{}, err
	}
	if lastLogin.Valid {
		t := lastLogin.Time
		a.LastLoginAt = &t
	}
	if lockedUntil.Valid {
		t := lockedUntil.Time
		a.LockedUntil = &t
	}
	a.Settings = unmarshalAccountSettings(settingsJSON, a.ID)
	return a, nil
}

// marshalAccountSettings encodes s as a compact JSON object. The zero
// value encodes as `{}` (omitempty on every field), so brand-new rows
// land with the same shape the migration's DEFAULT inserts.
func marshalAccountSettings(s AccountSettings) (string, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// unmarshalAccountSettings decodes the persisted blob. Malformed JSON
// is logged and falls back to the zero value rather than failing the
// row load — a settings column corruption shouldn't prevent the
// player from logging in. The accountID is logged for forensics.
func unmarshalAccountSettings(raw string, accountID int64) AccountSettings {
	if raw == "" {
		return AccountSettings{}
	}
	var s AccountSettings
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		slog.Warn("account: malformed settings_json, falling back to defaults",
			"account_id", accountID, "err", err)
		return AccountSettings{}
	}
	return s
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
