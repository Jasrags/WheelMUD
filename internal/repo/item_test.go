package repo

import (
	"context"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runItemRepoTests(t *testing.T, name string, newRepo func(t *testing.T) ItemRepo) {
	t.Helper()
	t.Run(name+"/list_in_starter", func(t *testing.T) {
		r := newRepo(t)
		got, err := r.ListInRoom(context.Background(), StarterRoomID)
		if err != nil {
			t.Fatalf("ListInRoom: %v", err)
		}
		if len(got) == 0 {
			t.Fatal("starter room has no items; seed migration probably regressed")
		}
		// Sorted by name_lower.
		for i := 1; i < len(got); i++ {
			if got[i-1].NameLower > got[i].NameLower {
				t.Fatalf("items not sorted: %+v", got)
			}
		}
	})

	t.Run(name+"/empty_room", func(t *testing.T) {
		r := newRepo(t)
		got, err := r.ListInRoom(context.Background(), 99999)
		if err != nil {
			t.Fatalf("ListInRoom: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d items, want 0", len(got))
		}
	})
}

func TestMemoryItemRepo(t *testing.T) {
	runItemRepoTests(t, "memory", func(t *testing.T) ItemRepo {
		r := NewMemoryItemRepo()
		r.Insert(Item{Name: "a small pebble", RoomID: StarterRoomID})
		return r
	})
}

func TestSQLiteItemRepo(t *testing.T) {
	runItemRepoTests(t, "sqlite", func(t *testing.T) ItemRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteItemRepo(conn)
	})
}
