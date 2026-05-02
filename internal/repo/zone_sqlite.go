package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type SQLiteZoneRepo struct {
	db *sql.DB
}

func NewSQLiteZoneRepo(db *sql.DB) *SQLiteZoneRepo {
	return &SQLiteZoneRepo{db: db}
}

const zoneSelectCols = `id, external_id, name, builder, ` +
	`min_level, max_level, reset_interval_s, reset_mode, ` +
	`climate, ambient_json`

func (r *SQLiteZoneRepo) Create(ctx context.Context, z Zone) (Zone, error) {
	if z.ResetMode == "" {
		z.ResetMode = ZoneResetEmpty
	}
	if !z.ResetMode.IsValid() {
		return Zone{}, ErrInvalidResetMode
	}
	ambientJSON, err := encodeAmbient(z.Ambient)
	if err != nil {
		return Zone{}, fmt.Errorf("encode ambient: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO zones(external_id, name, builder,
			min_level, max_level, reset_interval_s, reset_mode,
			climate, ambient_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		z.ExternalID, z.Name, z.Builder,
		z.MinLevel, z.MaxLevel, z.ResetIntervalS, string(z.ResetMode),
		z.Climate, ambientJSON,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Zone{}, ErrDuplicateZone
		}
		return Zone{}, fmt.Errorf("insert zone: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Zone{}, fmt.Errorf("last insert id: %w", err)
	}
	z.ID = id
	return z, nil
}

func (r *SQLiteZoneRepo) GetByID(ctx context.Context, id int64) (Zone, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+zoneSelectCols+` FROM zones WHERE id = ?`, id)
	z, err := scanZone(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Zone{}, ErrZoneNotFound
		}
		return Zone{}, fmt.Errorf("scan zone: %w", err)
	}
	return z, nil
}

func (r *SQLiteZoneRepo) GetByExternalID(ctx context.Context, externalID string) (Zone, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+zoneSelectCols+` FROM zones WHERE external_id = ?`, externalID)
	z, err := scanZone(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Zone{}, ErrZoneNotFound
		}
		return Zone{}, fmt.Errorf("scan zone: %w", err)
	}
	return z, nil
}

func (r *SQLiteZoneRepo) List(ctx context.Context) ([]Zone, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+zoneSelectCols+` FROM zones ORDER BY external_id`)
	if err != nil {
		return nil, fmt.Errorf("list zones: %w", err)
	}
	defer rows.Close()
	var out []Zone
	for rows.Next() {
		z, err := scanZone(rows)
		if err != nil {
			return nil, fmt.Errorf("scan zone row: %w", err)
		}
		out = append(out, z)
	}
	return out, rows.Err()
}

func scanZone(s rowScanner) (Zone, error) {
	var (
		z           Zone
		modeStr     string
		ambientJSON string
	)
	if err := s.Scan(
		&z.ID, &z.ExternalID, &z.Name, &z.Builder,
		&z.MinLevel, &z.MaxLevel, &z.ResetIntervalS, &modeStr,
		&z.Climate, &ambientJSON,
	); err != nil {
		return Zone{}, err
	}
	z.ResetMode = ZoneResetMode(modeStr)
	ambient, err := decodeAmbient(ambientJSON)
	if err != nil {
		// A corrupt ambient_json blob is data corruption — surface
		// rather than silently dropping the field.
		return Zone{}, fmt.Errorf("decode ambient_json: %w", err)
	}
	z.Ambient = ambient
	return z, nil
}

// encodeAmbient marshals the ambient line list to its JSON wire form.
// Nil → "[]" so the column NOT NULL contract holds.
func encodeAmbient(lines []string) (string, error) {
	if len(lines) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(lines)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// decodeAmbient parses the ambient_json column back into a string
// slice. An empty string is treated as the empty list rather than an
// error; the schema's DEFAULT '[]' makes this unreachable in practice
// but it keeps the function robust if a future migration relaxes it.
func decodeAmbient(raw string) ([]string, error) {
	if raw == "" || raw == "[]" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}
