package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SQLiteTrainerRepo persists trainer rows. Created in migration 0038.
//
// The constructor takes DBTX so the world loader can pass an in-flight
// *sql.Tx and the runtime can pass a pooled *sql.DB.
type SQLiteTrainerRepo struct {
	db DBTX
}

func NewSQLiteTrainerRepo(db DBTX) *SQLiteTrainerRepo {
	return &SQLiteTrainerRepo{db: db}
}

const trainerSelectCols = `id, mob_template_id, class_id`

func scanTrainer(scanner interface {
	Scan(dest ...interface{}) error
}) (Trainer, error) {
	var t Trainer
	if err := scanner.Scan(&t.ID, &t.MobTemplateID, &t.ClassID); err != nil {
		return Trainer{}, err
	}
	return t, nil
}

func (r *SQLiteTrainerRepo) Create(ctx context.Context, t Trainer) (Trainer, error) {
	if t.MobTemplateID == 0 {
		return Trainer{}, ErrInvalidExternalID
	}
	if t.ClassID == "" {
		return Trainer{}, ErrInvalidExternalID
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO trainers(mob_template_id, class_id) VALUES (?, ?)`,
		t.MobTemplateID, t.ClassID)
	if err != nil {
		if isUniqueViolation(err) {
			return Trainer{}, ErrDuplicateExternalID
		}
		return Trainer{}, fmt.Errorf("insert trainer: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Trainer{}, fmt.Errorf("lastInsertId trainer: %w", err)
	}
	t.ID = id
	return t, nil
}

func (r *SQLiteTrainerRepo) GetByMobTemplateID(ctx context.Context, mobTemplateID int64) (Trainer, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+trainerSelectCols+` FROM trainers WHERE mob_template_id = ?`, mobTemplateID)
	t, err := scanTrainer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Trainer{}, ErrTrainerNotFound
	}
	if err != nil {
		return Trainer{}, fmt.Errorf("get trainer: %w", err)
	}
	return t, nil
}

func (r *SQLiteTrainerRepo) ListTrainers(ctx context.Context) ([]Trainer, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+trainerSelectCols+` FROM trainers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list trainers: %w", err)
	}
	defer rows.Close()
	var out []Trainer
	for rows.Next() {
		t, err := scanTrainer(rows)
		if err != nil {
			return nil, fmt.Errorf("scan trainer: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trainers: %w", err)
	}
	return out, nil
}
