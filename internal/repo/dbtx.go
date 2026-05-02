package repo

import (
	"context"
	"database/sql"
)

// DBTX is the subset of *sql.DB / *sql.Tx that the SQLite repos use.
// Both standard types satisfy it, so a repo can be constructed
// against either a pooled connection (the runtime case) or an
// in-flight transaction (the world loader's atomic insert case).
//
// Kept narrow on purpose — only the three methods the creature
// repos actually call. Existing single-aggregate repos still use
// *sql.DB directly; widening them is a follow-up.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}
