// Package db wires the SQLite driver and applies embedded migrations.
//
// Migrations live in /migrations as `NNNN_description.sql`, applied in
// lexical order. Each file is one statement batch; we track applied
// versions in a `schema_migrations` table.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	_ "modernc.org/sqlite" // SQL driver registration
)

//go:embed all:migrations
var migrationFS embed.FS

const migrationsDir = "migrations"

// Open opens a SQLite connection at dsn (file path or `:memory:`), enables
// the pragmas the project relies on, and applies any pending migrations.
// The returned *sql.DB is safe for concurrent use.
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dsn, err)
	}
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := conn.ExecContext(ctx, pragma); err != nil {
			conn.Close()
			return nil, fmt.Errorf("exec %q: %w", pragma, err)
		}
	}
	if err := Migrate(ctx, conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// Migrate applies any embedded migrations that haven't been recorded in
// schema_migrations yet. Safe to call repeatedly; idempotent.
func Migrate(ctx context.Context, conn *sql.DB) error {
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version TEXT PRIMARY KEY,
        applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
    )`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	applied, err := loadAppliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	files, err := listMigrationFiles()
	if err != nil {
		return err
	}

	for _, f := range files {
		version := strings.TrimSuffix(f, ".sql")
		if applied[version] {
			continue
		}
		body, err := migrationFS.ReadFile(migrationsDir + "/" + f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		if err := applyOne(ctx, conn, version, string(body)); err != nil {
			return err
		}
	}
	return nil
}

func loadAppliedVersions(ctx context.Context, conn *sql.DB) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

func listMigrationFiles() ([]string, error) {
	entries, err := fs.ReadDir(migrationFS, migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	return files, nil
}

func applyOne(ctx context.Context, conn *sql.DB, version, body string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for %s: %w", version, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("apply %s: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES (?)`, version); err != nil {
		return fmt.Errorf("record %s: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s: %w", version, err)
	}
	return nil
}
