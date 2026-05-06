package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SQLiteAdminAuditRepo persists admin_audit rows. Append-only — no
// Update, no Delete. Times are stored as unix seconds for cheap range
// scans on the ts index.
type SQLiteAdminAuditRepo struct {
	db *sql.DB
}

func NewSQLiteAdminAuditRepo(db *sql.DB) *SQLiteAdminAuditRepo {
	return &SQLiteAdminAuditRepo{db: db}
}

func (r *SQLiteAdminAuditRepo) Record(ctx context.Context, e AdminAuditEntry) error {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	if e.ActorType == "" {
		e.ActorType = ActorTypeCharacter
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO admin_audit(ts, actor_character_id, actor_account_id, actor_type, actor_name, verb, target, args)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.TS.Unix(), e.ActorCharacterID, e.ActorAccountID, e.ActorType,
		e.ActorName, e.Verb, e.Target, e.Args,
	); err != nil {
		return fmt.Errorf("insert admin_audit: %w", err)
	}
	return nil
}

func (r *SQLiteAdminAuditRepo) List(ctx context.Context, f AdminAuditFilter) ([]AdminAuditEntry, error) {
	var (
		clauses []string
		args    []interface{}
	)
	if !f.Since.IsZero() {
		clauses = append(clauses, "ts >= ?")
		args = append(args, f.Since.Unix())
	}
	if f.Actor != 0 {
		clauses = append(clauses, "actor_character_id = ?")
		args = append(args, f.Actor)
	}
	if f.ActorAccount != 0 {
		clauses = append(clauses, "actor_account_id = ?")
		args = append(args, f.ActorAccount)
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
		limit = DefaultAdminAuditListLimit
	}
	args = append(args, limit)

	q := `SELECT id, ts, actor_character_id, actor_account_id, actor_type, actor_name, verb, target, args
	      FROM admin_audit`
	if len(clauses) > 0 {
		q += " WHERE " + strings.Join(clauses, " AND ")
	}
	q += " ORDER BY ts DESC, id DESC LIMIT ?"

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query admin_audit: %w", err)
	}
	defer rows.Close()

	var out []AdminAuditEntry
	for rows.Next() {
		var (
			e  AdminAuditEntry
			ts int64
		)
		if err := rows.Scan(&e.ID, &ts, &e.ActorCharacterID, &e.ActorAccountID,
			&e.ActorType, &e.ActorName, &e.Verb, &e.Target, &e.Args); err != nil {
			return nil, fmt.Errorf("scan admin_audit: %w", err)
		}
		e.TS = time.Unix(ts, 0).UTC()
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin_audit: %w", err)
	}
	return out, nil
}
