package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runRoomRepoTests(t *testing.T, name string, newRepo func(t *testing.T) RoomRepo) {
	t.Helper()
	t.Run(name+"/find_seeded_starter", func(t *testing.T) {
		r := newRepo(t)
		got, err := r.FindByID(context.Background(), StarterRoomID)
		if err != nil {
			t.Fatalf("FindByID(StarterRoomID): %v", err)
		}
		if got.ID != StarterRoomID {
			t.Fatalf("ID = %d, want %d", got.ID, StarterRoomID)
		}
		if got.Name == "" {
			t.Fatal("starter room has empty name; seed migration probably regressed")
		}
		if got.LongDesc == "" {
			t.Fatal("starter room has empty long_desc; seed migration probably regressed")
		}
	})

	t.Run(name+"/missing", func(t *testing.T) {
		r := newRepo(t)
		_, err := r.FindByID(context.Background(), 99999)
		if !errors.Is(err, ErrRoomNotFound) {
			t.Fatalf("err = %v, want ErrRoomNotFound", err)
		}
	})
}

func TestMemoryRoomRepo(t *testing.T) {
	runRoomRepoTests(t, "memory", func(t *testing.T) RoomRepo {
		r := NewMemoryRoomRepo()
		r.Insert(Room{ID: StarterRoomID, Name: "Town Plaza", LongDesc: "Cobblestones."})
		return r
	})
}

func TestSQLiteRoomRepo(t *testing.T) {
	runRoomRepoTests(t, "sqlite", func(t *testing.T) RoomRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteRoomRepo(conn)
	})
}
