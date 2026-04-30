package repo

import (
	"context"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runMobRepoTests(t *testing.T, name string, newRepo func(t *testing.T) MobRepo) {
	t.Helper()
	t.Run(name+"/list_in_starter", func(t *testing.T) {
		r := newRepo(t)
		got, err := r.ListInRoom(context.Background(), StarterRoomID)
		if err != nil {
			t.Fatalf("ListInRoom: %v", err)
		}
		if len(got) == 0 {
			t.Fatal("starter room has no mobs; seed migration probably regressed")
		}
		for i := 1; i < len(got); i++ {
			if got[i-1].NameLower > got[i].NameLower {
				t.Fatalf("mobs not sorted: %+v", got)
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
			t.Fatalf("got %d mobs, want 0", len(got))
		}
	})
}

func TestMemoryMobRepo(t *testing.T) {
	runMobRepoTests(t, "memory", func(t *testing.T) MobRepo {
		r := NewMemoryMobRepo()
		r.Insert(Mob{Name: "a town crier", RoomID: StarterRoomID})
		return r
	})
}

func TestSQLiteMobRepo(t *testing.T) {
	runMobRepoTests(t, "sqlite", func(t *testing.T) MobRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteMobRepo(conn)
	})
}
