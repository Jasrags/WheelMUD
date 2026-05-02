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

func TestOpen_ZonesSchema(t *testing.T) {
	ctx := context.Background()
	conn, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	// zones table exists and is queryable.
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM zones`).Scan(&count); err != nil {
		t.Fatalf("query zones: %v", err)
	}

	// rooms.zone_id column was added by migration 0016.
	if _, err := conn.ExecContext(ctx, `SELECT zone_id FROM rooms LIMIT 0`); err != nil {
		t.Fatalf("rooms.zone_id missing: %v", err)
	}

	// CHECK rejects an invalid reset_mode.
	_, err = conn.ExecContext(ctx,
		`INSERT INTO zones(external_id, name, reset_mode) VALUES (?, ?, ?)`,
		"bad_mode", "Bad Mode", "blah")
	if err == nil {
		t.Fatal("expected CHECK violation on reset_mode='blah', got nil")
	}

	// CHECK rejects min_level < 1.
	_, err = conn.ExecContext(ctx,
		`INSERT INTO zones(external_id, name, min_level) VALUES (?, ?, ?)`,
		"bad_min", "Bad Min", 0)
	if err == nil {
		t.Fatal("expected CHECK violation on min_level=0, got nil")
	}

	// CHECK rejects max_level < min_level.
	_, err = conn.ExecContext(ctx,
		`INSERT INTO zones(external_id, name, min_level, max_level) VALUES (?, ?, ?, ?)`,
		"bad_range", "Bad Range", 10, 5)
	if err == nil {
		t.Fatal("expected CHECK violation on max_level<min_level, got nil")
	}

	// CHECK rejects negative reset_interval_s.
	_, err = conn.ExecContext(ctx,
		`INSERT INTO zones(external_id, name, reset_interval_s) VALUES (?, ?, ?)`,
		"bad_interval", "Bad Interval", -5)
	if err == nil {
		t.Fatal("expected CHECK violation on reset_interval_s=-5, got nil")
	}

	// Happy-path insert with all defaults works.
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO zones(external_id, name) VALUES (?, ?)`,
		"ok", "OK"); err != nil {
		t.Fatalf("happy-path zone insert: %v", err)
	}

	// UNIQUE on external_id rejects a duplicate.
	_, err = conn.ExecContext(ctx,
		`INSERT INTO zones(external_id, name) VALUES (?, ?)`,
		"ok", "OK Again")
	if err == nil {
		t.Fatal("expected UNIQUE violation on duplicate external_id, got nil")
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
