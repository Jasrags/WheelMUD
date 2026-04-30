package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

type mobRepoFixture struct {
	mobs  MobRepo
	rooms RoomRepo
}

func runMobRepoTests(t *testing.T, name string, newFix func(t *testing.T) mobRepoFixture) {
	t.Helper()

	makeRoom := func(t *testing.T, fix mobRepoFixture) int64 {
		t.Helper()
		r, err := fix.rooms.Create(context.Background(), Room{ExternalID: "a", Name: "A"})
		if err != nil {
			t.Fatalf("create room: %v", err)
		}
		return r.ID
	}

	t.Run(name+"/create_and_list", func(t *testing.T) {
		fix := newFix(t)
		roomID := makeRoom(t, fix)
		ctx := context.Background()
		if _, err := fix.mobs.Create(ctx, Mob{ExternalID: "crier", Name: "a town crier", RoomID: roomID}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := fix.mobs.ListInRoom(ctx, roomID)
		if err != nil {
			t.Fatalf("ListInRoom: %v", err)
		}
		if len(got) != 1 || got[0].Name != "a town crier" {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run(name+"/create_rejects_empty_external_id", func(t *testing.T) {
		fix := newFix(t)
		_, err := fix.mobs.Create(context.Background(), Mob{Name: "ghost"})
		if !errors.Is(err, ErrInvalidExternalID) {
			t.Fatalf("err = %v, want ErrInvalidExternalID", err)
		}
	})

	t.Run(name+"/create_duplicate_external_id", func(t *testing.T) {
		fix := newFix(t)
		ctx := context.Background()
		if _, err := fix.mobs.Create(ctx, Mob{ExternalID: "dup", Name: "a"}); err != nil {
			t.Fatalf("first Create: %v", err)
		}
		_, err := fix.mobs.Create(ctx, Mob{ExternalID: "dup", Name: "b"})
		if !errors.Is(err, ErrDuplicateExternalID) {
			t.Fatalf("err = %v, want ErrDuplicateExternalID", err)
		}
	})

	t.Run(name+"/empty_room", func(t *testing.T) {
		fix := newFix(t)
		got, err := fix.mobs.ListInRoom(context.Background(), 99999)
		if err != nil {
			t.Fatalf("ListInRoom: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d mobs, want 0", len(got))
		}
	})
}

func TestMemoryMobRepo(t *testing.T) {
	runMobRepoTests(t, "memory", func(t *testing.T) mobRepoFixture {
		return mobRepoFixture{mobs: NewMemoryMobRepo(), rooms: NewMemoryRoomRepo()}
	})
}

func TestSQLiteMobRepo(t *testing.T) {
	runMobRepoTests(t, "sqlite", func(t *testing.T) mobRepoFixture {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return mobRepoFixture{
			mobs:  NewSQLiteMobRepo(conn),
			rooms: NewSQLiteRoomRepo(conn),
		}
	})
}
