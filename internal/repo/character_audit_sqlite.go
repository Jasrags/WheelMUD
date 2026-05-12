package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SQLiteCharacterAuditRepo persists character_audit rows. Append-only —
// no Update, no Delete. Times are stored as unix seconds for cheap
// range scans on the ts index.
type SQLiteCharacterAuditRepo struct {
	db *sql.DB
}

func NewSQLiteCharacterAuditRepo(db *sql.DB) *SQLiteCharacterAuditRepo {
	return &SQLiteCharacterAuditRepo{db: db}
}

func (r *SQLiteCharacterAuditRepo) Record(ctx context.Context, e CharacterAuditEntry) error {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	raw := e.Raw
	if len(raw) > CharacterAuditRawCap {
		raw = raw[:CharacterAuditRawCap]
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO character_audit(ts, character_id, character_name, room_id, verb, raw)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		e.TS.Unix(), e.CharacterID, e.CharacterName, e.RoomID, e.Verb, raw,
	); err != nil {
		return fmt.Errorf("insert character_audit: %w", err)
	}
	return nil
}

func (r *SQLiteCharacterAuditRepo) List(ctx context.Context, f CharacterAuditFilter) ([]CharacterAuditEntry, error) {
	var (
		clauses []string
		args    []any
	)
	if !f.Since.IsZero() {
		clauses = append(clauses, "ts >= ?")
		args = append(args, f.Since.Unix())
	}
	if f.Character != 0 {
		clauses = append(clauses, "character_id = ?")
		args = append(args, f.Character)
	}
	if len(f.Verbs) > 0 {
		placeholders := strings.Repeat("?,", len(f.Verbs))
		placeholders = placeholders[:len(placeholders)-1]
		clauses = append(clauses, "verb IN ("+placeholders+")")
		for _, v := range f.Verbs {
			args = append(args, v)
		}
	}

	limit := f.Limit
	if limit <= 0 {
		limit = DefaultCharacterAuditListLimit
	}
	args = append(args, limit)

	q := `SELECT id, ts, character_id, character_name, room_id, verb, raw
	      FROM character_audit`
	if len(clauses) > 0 {
		q += " WHERE " + strings.Join(clauses, " AND ")
	}
	q += " ORDER BY ts DESC, id DESC LIMIT ?"

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query character_audit: %w", err)
	}
	defer rows.Close()

	var out []CharacterAuditEntry
	for rows.Next() {
		var (
			e  CharacterAuditEntry
			ts int64
		)
		if err := rows.Scan(&e.ID, &ts, &e.CharacterID, &e.CharacterName,
			&e.RoomID, &e.Verb, &e.Raw); err != nil {
			return nil, fmt.Errorf("scan character_audit: %w", err)
		}
		e.TS = time.Unix(ts, 0).UTC()
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate character_audit: %w", err)
	}
	return out, nil
}
