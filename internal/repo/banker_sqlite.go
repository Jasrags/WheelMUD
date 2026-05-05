package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SQLiteBankerRepo persists banker rows. Created in migration 0031.
//
// The constructor takes DBTX so the world loader can pass an
// in-flight *sql.Tx and the runtime can pass a pooled *sql.DB.
type SQLiteBankerRepo struct {
	db DBTX
}

func NewSQLiteBankerRepo(db DBTX) *SQLiteBankerRepo {
	return &SQLiteBankerRepo{db: db}
}

const bankerSelectCols = `id, mob_template_id, open_hour, close_hour`

func scanBanker(scanner interface{ Scan(dest ...interface{}) error }) (Banker, error) {
	var b Banker
	if err := scanner.Scan(&b.ID, &b.MobTemplateID, &b.OpenHour, &b.CloseHour); err != nil {
		return Banker{}, err
	}
	return b, nil
}

func (r *SQLiteBankerRepo) Create(ctx context.Context, b Banker) (Banker, error) {
	if b.MobTemplateID == 0 {
		return Banker{}, ErrInvalidExternalID
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO bankers(mob_template_id, open_hour, close_hour)
		 VALUES (?, ?, ?)`,
		b.MobTemplateID, b.OpenHour, b.CloseHour)
	if err != nil {
		if isUniqueViolation(err) {
			return Banker{}, ErrDuplicateExternalID
		}
		return Banker{}, fmt.Errorf("insert banker: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Banker{}, fmt.Errorf("lastInsertId banker: %w", err)
	}
	b.ID = id
	return b, nil
}

func (r *SQLiteBankerRepo) GetByMobTemplateID(ctx context.Context, mobTemplateID int64) (Banker, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+bankerSelectCols+` FROM bankers WHERE mob_template_id = ?`, mobTemplateID)
	b, err := scanBanker(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Banker{}, ErrBankerNotFound
	}
	if err != nil {
		return Banker{}, fmt.Errorf("get banker: %w", err)
	}
	return b, nil
}

func (r *SQLiteBankerRepo) ListBankers(ctx context.Context) ([]Banker, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+bankerSelectCols+` FROM bankers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list bankers: %w", err)
	}
	defer rows.Close()
	var out []Banker
	for rows.Next() {
		b, err := scanBanker(rows)
		if err != nil {
			return nil, fmt.Errorf("scan banker: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bankers: %w", err)
	}
	return out, nil
}
