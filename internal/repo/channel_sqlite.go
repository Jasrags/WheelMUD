package repo

import (
	"context"
	"database/sql"
	"fmt"
)

type SQLiteChannelRepo struct {
	db *sql.DB
}

func NewSQLiteChannelRepo(db *sql.DB) *SQLiteChannelRepo {
	return &SQLiteChannelRepo{db: db}
}

func (r *SQLiteChannelRepo) List(ctx context.Context) ([]Channel, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, min_level, color FROM channels ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()
	var out []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.Name, &c.MinLevel, &c.Color); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
