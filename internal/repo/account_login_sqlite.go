package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SQLiteAccountLoginRepo persists account_logins rows. Append-only — no
// Update, no Delete. Times are stored as unix seconds for cheap range
// scans on the (account_id, ts) index.
type SQLiteAccountLoginRepo struct {
	db *sql.DB
}

func NewSQLiteAccountLoginRepo(db *sql.DB) *SQLiteAccountLoginRepo {
	return &SQLiteAccountLoginRepo{db: db}
}

func (r *SQLiteAccountLoginRepo) Record(ctx context.Context, e AccountLoginEntry) error {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO account_logins(account_id, ts, remote_address, outcome, info)
		 VALUES (?, ?, ?, ?, ?)`,
		e.AccountID, e.At.Unix(), e.RemoteAddress, e.Outcome, e.Info,
	); err != nil {
		return fmt.Errorf("insert account_logins: %w", err)
	}
	return nil
}

func (r *SQLiteAccountLoginRepo) ListRecentByAccount(ctx context.Context, accountID int64, limit int) ([]AccountLoginEntry, error) {
	if limit <= 0 {
		limit = DefaultAccountLoginListLimit
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, account_id, ts, remote_address, outcome, info
		 FROM account_logins
		 WHERE account_id = ?
		 ORDER BY ts DESC, id DESC
		 LIMIT ?`,
		accountID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query account_logins: %w", err)
	}
	defer rows.Close()

	var out []AccountLoginEntry
	for rows.Next() {
		var (
			e  AccountLoginEntry
			ts int64
		)
		if err := rows.Scan(&e.ID, &e.AccountID, &ts,
			&e.RemoteAddress, &e.Outcome, &e.Info); err != nil {
			return nil, fmt.Errorf("scan account_logins: %w", err)
		}
		e.At = time.Unix(ts, 0).UTC()
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account_logins: %w", err)
	}
	return out, nil
}
