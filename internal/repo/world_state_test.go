package repo

import (
	"context"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runWorldStateRepoTests(t *testing.T, name string, newRepo func(t *testing.T) WorldStateRepo) {
	t.Helper()
	t.Run(name+"/default_at_noon", func(t *testing.T) {
		r := newRepo(t)
		got, err := r.GetTicks(context.Background())
		if err != nil {
			t.Fatalf("GetTicks: %v", err)
		}
		if got != 675 {
			t.Fatalf("default ticks = %d, want 675 (noon)", got)
		}
	})
	t.Run(name+"/round_trip", func(t *testing.T) {
		r := newRepo(t)
		if err := r.SetTicks(context.Background(), 12345); err != nil {
			t.Fatalf("SetTicks: %v", err)
		}
		got, err := r.GetTicks(context.Background())
		if err != nil {
			t.Fatalf("GetTicks: %v", err)
		}
		if got != 12345 {
			t.Fatalf("ticks = %d, want 12345", got)
		}
	})
}

func TestMemoryWorldStateRepo(t *testing.T) {
	runWorldStateRepoTests(t, "memory", func(t *testing.T) WorldStateRepo {
		return NewMemoryWorldStateRepo()
	})
}

func TestSQLiteWorldStateRepo(t *testing.T) {
	runWorldStateRepoTests(t, "sqlite", func(t *testing.T) WorldStateRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteWorldStateRepo(conn)
	})
}
