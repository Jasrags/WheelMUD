package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SQLiteCharacterRepo struct {
	db *sql.DB
}

func NewSQLiteCharacterRepo(db *sql.DB) *SQLiteCharacterRepo {
	return &SQLiteCharacterRepo{db: db}
}

func (r *SQLiteCharacterRepo) Create(ctx context.Context, c Character) (Character, error) {
	c.NameLower = strings.ToLower(c.Name)
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO characters(account_id, name, name_lower, created_at)
		 VALUES (?, ?, ?, ?)`,
		c.AccountID, c.Name, c.NameLower, c.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Character{}, ErrDuplicateCharacterName
		}
		return Character{}, fmt.Errorf("insert character: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Character{}, fmt.Errorf("last insert id: %w", err)
	}
	c.ID = id
	return c, nil
}

func (r *SQLiteCharacterRepo) FindByName(ctx context.Context, name string) (Character, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, account_id, name, name_lower, created_at, last_played_at
		 FROM characters WHERE name_lower = ?`,
		strings.ToLower(name),
	)
	var (
		c          Character
		lastPlayed sql.NullTime
	)
	err := row.Scan(&c.ID, &c.AccountID, &c.Name, &c.NameLower, &c.CreatedAt, &lastPlayed)
	if errors.Is(err, sql.ErrNoRows) {
		return Character{}, ErrCharacterNotFound
	}
	if err != nil {
		return Character{}, fmt.Errorf("scan character: %w", err)
	}
	if lastPlayed.Valid {
		t := lastPlayed.Time
		c.LastPlayedAt = &t
	}
	return c, nil
}

func (r *SQLiteCharacterRepo) ListByAccount(ctx context.Context, accountID int64) ([]Character, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, account_id, name, name_lower, created_at, last_played_at
		 FROM characters
		 WHERE account_id = ?
		 ORDER BY last_played_at DESC NULLS LAST, name_lower`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list characters: %w", err)
	}
	defer rows.Close()
	var out []Character
	for rows.Next() {
		var (
			c          Character
			lastPlayed sql.NullTime
		)
		if err := rows.Scan(&c.ID, &c.AccountID, &c.Name, &c.NameLower, &c.CreatedAt, &lastPlayed); err != nil {
			return nil, fmt.Errorf("scan character row: %w", err)
		}
		if lastPlayed.Valid {
			t := lastPlayed.Time
			c.LastPlayedAt = &t
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *SQLiteCharacterRepo) RecordPlay(ctx context.Context, id int64, when time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE characters SET last_played_at = ? WHERE id = ?`,
		when, id,
	)
	if err != nil {
		return fmt.Errorf("record play: %w", err)
	}
	return nil
}
