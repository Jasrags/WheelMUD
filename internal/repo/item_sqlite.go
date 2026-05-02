package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/currency"
)

type SQLiteItemRepo struct {
	db *sql.DB
}

func NewSQLiteItemRepo(db *sql.DB) *SQLiteItemRepo {
	return &SQLiteItemRepo{db: db}
}

const itemSelectCols = `id, external_id, name, name_lower, short_desc, room_id, ` +
	`type, weight_lbs, value_cp, quality, flags, stats_json, created_at`

func (r *SQLiteItemRepo) Create(ctx context.Context, i Item) (Item, error) {
	if i.ExternalID == "" {
		return Item{}, ErrInvalidExternalID
	}
	if i.NameLower == "" {
		i.NameLower = strings.ToLower(i.Name)
	}
	if i.Type == "" {
		i.Type = ItemTypeTrash
	}
	if !i.Type.IsValid() {
		return Item{}, fmt.Errorf("invalid item type %q", i.Type)
	}
	if i.Quality == "" {
		i.Quality = QualityNormal
	}
	if !i.Quality.IsValid() {
		return Item{}, fmt.Errorf("invalid item quality %q", i.Quality)
	}
	if !statsTypeMatches(i.Type, i.Stats) {
		return Item{}, ErrItemStatsTypeMismatch
	}
	statsJSON, err := encodeStats(i.Stats)
	if err != nil {
		return Item{}, err
	}
	if i.CreatedAt.IsZero() {
		i.CreatedAt = time.Now().UTC()
	}
	var roomID any
	if i.RoomID != 0 {
		roomID = i.RoomID
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO items(external_id, name, name_lower, short_desc, room_id,
			type, weight_lbs, value_cp, quality, flags, stats_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		i.ExternalID, i.Name, i.NameLower, i.ShortDesc, roomID,
		string(i.Type), i.Weight, int64(i.Value), string(i.Quality),
		int64(i.Flags), statsJSON, i.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Item{}, ErrDuplicateExternalID
		}
		return Item{}, fmt.Errorf("insert item: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Item{}, fmt.Errorf("last insert id: %w", err)
	}
	i.ID = id
	return i, nil
}

func (r *SQLiteItemRepo) ListInRoom(ctx context.Context, roomID int64) ([]Item, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+itemSelectCols+` FROM items WHERE room_id = ? ORDER BY name_lower`,
		roomID,
	)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		i, err := scanItemRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// scanItemRow reads one row from a SELECT itemSelectCols result and
// builds a fully decoded Item, including the polymorphic stats blob.
// Centralized so the column list and decode contract live in one
// place — callers just hand it a *sql.Rows / *sql.Row.
func scanItemRow(s rowScanner) (Item, error) {
	var (
		i         Item
		rid       sql.NullInt64
		typeStr   string
		quality   string
		valueCP   int64
		flags     int64
		statsJSON string
	)
	if err := s.Scan(
		&i.ID, &i.ExternalID, &i.Name, &i.NameLower, &i.ShortDesc, &rid,
		&typeStr, &i.Weight, &valueCP, &quality, &flags, &statsJSON, &i.CreatedAt,
	); err != nil {
		return Item{}, fmt.Errorf("scan item row: %w", err)
	}
	if rid.Valid {
		i.RoomID = rid.Int64
	}
	i.Type = ItemType(typeStr)
	i.Quality = ItemQuality(quality)
	i.Value = currency.Amount(valueCP)
	i.Flags = ItemFlags(flags)
	stats, err := decodeStats(i.Type, statsJSON)
	if err != nil {
		return Item{}, err
	}
	i.Stats = stats
	return i, nil
}
