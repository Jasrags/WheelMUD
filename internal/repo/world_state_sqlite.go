package repo

import (
	"context"
	"database/sql"
	"fmt"
)

type SQLiteWorldStateRepo struct {
	db *sql.DB
}

func NewSQLiteWorldStateRepo(db *sql.DB) *SQLiteWorldStateRepo {
	return &SQLiteWorldStateRepo{db: db}
}

func (r *SQLiteWorldStateRepo) GetTicks(ctx context.Context) (int64, error) {
	var ticks int64
	err := r.db.QueryRowContext(ctx,
		`SELECT ticks FROM world_state WHERE id = 1`).Scan(&ticks)
	if err != nil {
		return 0, fmt.Errorf("get world ticks: %w", err)
	}
	return ticks, nil
}

func (r *SQLiteWorldStateRepo) SetTicks(ctx context.Context, ticks int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE world_state SET ticks = ? WHERE id = 1`,
		ticks,
	)
	if err != nil {
		return fmt.Errorf("set world ticks: %w", err)
	}
	return nil
}
