package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SQLiteBuilderZoneRepo persists builder_zones rows. Times round-trip
// as unix seconds (mirrors admin_audit.ts).
type SQLiteBuilderZoneRepo struct {
	db *sql.DB
}

func NewSQLiteBuilderZoneRepo(db *sql.DB) *SQLiteBuilderZoneRepo {
	return &SQLiteBuilderZoneRepo{db: db}
}

func (r *SQLiteBuilderZoneRepo) Grant(ctx context.Context, characterID, zoneID, grantedBy int64, grantedAt time.Time) error {
	if grantedAt.IsZero() {
		grantedAt = time.Now().UTC()
	}
	// INSERT OR REPLACE keeps Grant idempotent: re-issuing refreshes
	// granted_by / granted_at without surfacing a unique-violation.
	if _, err := r.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO builder_zones(character_id, zone_id, granted_by, granted_at)
		 VALUES (?, ?, ?, ?)`,
		characterID, zoneID, grantedBy, grantedAt.Unix(),
	); err != nil {
		return fmt.Errorf("insert builder_zone: %w", err)
	}
	return nil
}

func (r *SQLiteBuilderZoneRepo) Revoke(ctx context.Context, characterID, zoneID int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM builder_zones WHERE character_id = ? AND zone_id = ?`,
		characterID, zoneID,
	)
	if err != nil {
		return fmt.Errorf("delete builder_zone: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete builder_zone rows affected: %w", err)
	}
	if n == 0 {
		return ErrBuilderZoneNotFound
	}
	return nil
}

func (r *SQLiteBuilderZoneRepo) Has(ctx context.Context, characterID, zoneID int64) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM builder_zones WHERE character_id = ? AND zone_id = ?`,
		characterID, zoneID,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("has builder_zone: %w", err)
	}
	return true, nil
}

func (r *SQLiteBuilderZoneRepo) ListForCharacter(ctx context.Context, characterID int64) ([]BuilderZone, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT character_id, zone_id, granted_by, granted_at
		 FROM builder_zones WHERE character_id = ? ORDER BY zone_id ASC`,
		characterID,
	)
	if err != nil {
		return nil, fmt.Errorf("list builder_zones by character: %w", err)
	}
	defer rows.Close()
	return scanBuilderZones(rows)
}

func (r *SQLiteBuilderZoneRepo) ListForZone(ctx context.Context, zoneID int64) ([]BuilderZone, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT character_id, zone_id, granted_by, granted_at
		 FROM builder_zones WHERE zone_id = ? ORDER BY character_id ASC`,
		zoneID,
	)
	if err != nil {
		return nil, fmt.Errorf("list builder_zones by zone: %w", err)
	}
	defer rows.Close()
	return scanBuilderZones(rows)
}

func scanBuilderZones(rows *sql.Rows) ([]BuilderZone, error) {
	var out []BuilderZone
	for rows.Next() {
		var (
			bz BuilderZone
			ts int64
		)
		if err := rows.Scan(&bz.CharacterID, &bz.ZoneID, &bz.GrantedBy, &ts); err != nil {
			return nil, fmt.Errorf("scan builder_zone: %w", err)
		}
		bz.GrantedAt = time.Unix(ts, 0).UTC()
		out = append(out, bz)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate builder_zones: %w", err)
	}
	return out, nil
}
