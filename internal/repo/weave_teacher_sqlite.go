package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// SQLiteWeaveTeacherRepo persists weave_teachers rows. Created in
// migration 0043.
//
// The constructor takes DBTX so the world loader can pass an in-flight
// *sql.Tx and the runtime can pass a pooled *sql.DB.
type SQLiteWeaveTeacherRepo struct {
	db DBTX
}

func NewSQLiteWeaveTeacherRepo(db DBTX) *SQLiteWeaveTeacherRepo {
	return &SQLiteWeaveTeacherRepo{db: db}
}

const weaveTeacherSelectCols = `id, mob_template_id, max_level_taught, affinity_filter`

func scanWeaveTeacher(scanner interface {
	Scan(dest ...interface{}) error
}) (WeaveTeacher, error) {
	var t WeaveTeacher
	var aff int64
	if err := scanner.Scan(&t.ID, &t.MobTemplateID, &t.MaxLevelTaught, &aff); err != nil {
		return WeaveTeacher{}, err
	}
	t.AffinityFilter = creature.PowerSet(uint8(aff))
	return t, nil
}

func (r *SQLiteWeaveTeacherRepo) Create(ctx context.Context, t WeaveTeacher) (WeaveTeacher, error) {
	if t.MobTemplateID == 0 {
		return WeaveTeacher{}, ErrInvalidExternalID
	}
	if t.MaxLevelTaught < 0 || t.MaxLevelTaught > 9 {
		return WeaveTeacher{}, ErrInvalidExternalID
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO weave_teachers(mob_template_id, max_level_taught, affinity_filter) VALUES (?, ?, ?)`,
		t.MobTemplateID, t.MaxLevelTaught, int64(t.AffinityFilter))
	if err != nil {
		if isUniqueViolation(err) {
			return WeaveTeacher{}, ErrDuplicateExternalID
		}
		return WeaveTeacher{}, fmt.Errorf("insert weave teacher: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return WeaveTeacher{}, fmt.Errorf("lastInsertId weave teacher: %w", err)
	}
	t.ID = id
	return t, nil
}

func (r *SQLiteWeaveTeacherRepo) GetByMobTemplateID(ctx context.Context, mobTemplateID int64) (WeaveTeacher, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+weaveTeacherSelectCols+` FROM weave_teachers WHERE mob_template_id = ?`, mobTemplateID)
	t, err := scanWeaveTeacher(row)
	if errors.Is(err, sql.ErrNoRows) {
		return WeaveTeacher{}, ErrWeaveTeacherNotFound
	}
	if err != nil {
		return WeaveTeacher{}, fmt.Errorf("get weave teacher: %w", err)
	}
	return t, nil
}

func (r *SQLiteWeaveTeacherRepo) ListWeaveTeachers(ctx context.Context) ([]WeaveTeacher, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+weaveTeacherSelectCols+` FROM weave_teachers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list weave teachers: %w", err)
	}
	defer rows.Close()
	var out []WeaveTeacher
	for rows.Next() {
		t, err := scanWeaveTeacher(rows)
		if err != nil {
			return nil, fmt.Errorf("scan weave teacher: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate weave teachers: %w", err)
	}
	return out, nil
}
