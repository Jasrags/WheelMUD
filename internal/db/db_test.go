package db

import (
	"context"
	"testing"
)

func TestOpen_AppliesMigrationsOnce(t *testing.T) {
	ctx := context.Background()
	conn, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	defer conn.Close()

	// accounts table exists.
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts`).Scan(&count); err != nil {
		t.Fatalf("query accounts: %v", err)
	}

	// schema_migrations records the run.
	var migs int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migs); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if migs == 0 {
		t.Fatal("schema_migrations is empty after Open")
	}

	// Re-running Migrate is a no-op (no error, no duplicate inserts).
	if err := Migrate(ctx, conn); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	var migs2 int
	_ = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migs2)
	if migs2 != migs {
		t.Fatalf("schema_migrations grew on rerun: %d -> %d", migs, migs2)
	}
}

func TestOpen_EnablesPragmas(t *testing.T) {
	ctx := context.Background()
	conn, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	var fk int
	if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}
}
