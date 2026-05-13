package repo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runBuilderZoneRepoTests(t *testing.T, name string, newRepo func(t *testing.T) BuilderZoneRepo) {
	t.Helper()

	t.Run(name+"/grant_then_has", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if err := r.Grant(ctx, 7, 42, 1, time.Time{}); err != nil {
			t.Fatalf("grant: %v", err)
		}
		ok, err := r.Has(ctx, 7, 42)
		if err != nil {
			t.Fatalf("has: %v", err)
		}
		if !ok {
			t.Fatal("Has returned false for granted pair")
		}
		ok, err = r.Has(ctx, 7, 99)
		if err != nil {
			t.Fatalf("has unrelated: %v", err)
		}
		if ok {
			t.Fatal("Has returned true for unrelated zone")
		}
	})

	t.Run(name+"/grant_is_idempotent", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		second := first.Add(time.Hour)
		if err := r.Grant(ctx, 7, 42, 1, first); err != nil {
			t.Fatalf("first grant: %v", err)
		}
		if err := r.Grant(ctx, 7, 42, 2, second); err != nil {
			t.Fatalf("second grant: %v", err)
		}
		got, err := r.ListForCharacter(ctx, 7)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1 (idempotent upsert)", len(got))
		}
		if got[0].GrantedBy != 2 {
			t.Fatalf("GrantedBy = %d, want 2 (refresh)", got[0].GrantedBy)
		}
		if !got[0].GrantedAt.Equal(second) {
			t.Fatalf("GrantedAt = %v, want %v (refresh)", got[0].GrantedAt, second)
		}
	})

	t.Run(name+"/revoke_removes_then_errs_missing", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if err := r.Grant(ctx, 7, 42, 1, time.Time{}); err != nil {
			t.Fatalf("grant: %v", err)
		}
		if err := r.Revoke(ctx, 7, 42); err != nil {
			t.Fatalf("revoke: %v", err)
		}
		ok, err := r.Has(ctx, 7, 42)
		if err != nil {
			t.Fatalf("has post-revoke: %v", err)
		}
		if ok {
			t.Fatal("Has returned true after Revoke")
		}
		err = r.Revoke(ctx, 7, 42)
		if !errors.Is(err, ErrBuilderZoneNotFound) {
			t.Fatalf("second revoke err = %v, want ErrBuilderZoneNotFound", err)
		}
	})

	t.Run(name+"/list_for_character_sorted", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		// Grant in unsorted order; List must return ZoneID ascending.
		for _, z := range []int64{30, 10, 20} {
			if err := r.Grant(ctx, 7, z, 1, time.Time{}); err != nil {
				t.Fatalf("grant %d: %v", z, err)
			}
		}
		got, err := r.ListForCharacter(ctx, 7)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		want := []int64{10, 20, 30}
		if len(got) != len(want) {
			t.Fatalf("len = %d, want %d", len(got), len(want))
		}
		for i, bz := range got {
			if bz.ZoneID != want[i] {
				t.Fatalf("position %d zone = %d, want %d", i, bz.ZoneID, want[i])
			}
		}
	})

	t.Run(name+"/list_for_zone_isolated", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		// Two characters granted on zone 42; a third on a different zone.
		if err := r.Grant(ctx, 7, 42, 1, time.Time{}); err != nil {
			t.Fatalf("grant 7,42: %v", err)
		}
		if err := r.Grant(ctx, 3, 42, 1, time.Time{}); err != nil {
			t.Fatalf("grant 3,42: %v", err)
		}
		if err := r.Grant(ctx, 9, 99, 1, time.Time{}); err != nil {
			t.Fatalf("grant 9,99: %v", err)
		}
		got, err := r.ListForZone(ctx, 42)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0].CharacterID != 3 || got[1].CharacterID != 7 {
			t.Fatalf("order = %d, %d, want 3, 7", got[0].CharacterID, got[1].CharacterID)
		}
	})

	t.Run(name+"/list_empty_returns_nil", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		got, err := r.ListForCharacter(ctx, 999)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("len = %d, want 0", len(got))
		}
	})
}

func TestMemoryBuilderZoneRepo(t *testing.T) {
	runBuilderZoneRepoTests(t, "memory", func(t *testing.T) BuilderZoneRepo {
		return NewMemoryBuilderZoneRepo()
	})
}

func TestSQLiteBuilderZoneRepo(t *testing.T) {
	runBuilderZoneRepoTests(t, "sqlite", func(t *testing.T) BuilderZoneRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteBuilderZoneRepo(conn)
	})
}
