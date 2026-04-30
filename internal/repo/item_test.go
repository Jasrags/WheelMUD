package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

type itemRepoFixture struct {
	items ItemRepo
	rooms RoomRepo
}

func runItemRepoTests(t *testing.T, name string, newFix func(t *testing.T) itemRepoFixture) {
	t.Helper()

	makeRoom := func(t *testing.T, fix itemRepoFixture) int64 {
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
		if _, err := fix.items.Create(ctx, Item{ExternalID: "pebble", Name: "a small pebble", RoomID: roomID}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := fix.items.ListInRoom(ctx, roomID)
		if err != nil {
			t.Fatalf("ListInRoom: %v", err)
		}
		if len(got) != 1 || got[0].Name != "a small pebble" {
			t.Fatalf("got %+v", got)
		}
		if got[0].NameLower != "a small pebble" {
			t.Fatalf("NameLower = %q", got[0].NameLower)
		}
	})

	t.Run(name+"/create_rejects_empty_external_id", func(t *testing.T) {
		fix := newFix(t)
		_, err := fix.items.Create(context.Background(), Item{Name: "ghost"})
		if !errors.Is(err, ErrInvalidExternalID) {
			t.Fatalf("err = %v, want ErrInvalidExternalID", err)
		}
	})

	t.Run(name+"/create_duplicate_external_id", func(t *testing.T) {
		fix := newFix(t)
		ctx := context.Background()
		if _, err := fix.items.Create(ctx, Item{ExternalID: "dup", Name: "a"}); err != nil {
			t.Fatalf("first Create: %v", err)
		}
		_, err := fix.items.Create(ctx, Item{ExternalID: "dup", Name: "b"})
		if !errors.Is(err, ErrDuplicateExternalID) {
			t.Fatalf("err = %v, want ErrDuplicateExternalID", err)
		}
	})

	t.Run(name+"/empty_room", func(t *testing.T) {
		fix := newFix(t)
		got, err := fix.items.ListInRoom(context.Background(), 99999)
		if err != nil {
			t.Fatalf("ListInRoom: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d items, want 0", len(got))
		}
	})
}

func TestMemoryItemRepo(t *testing.T) {
	runItemRepoTests(t, "memory", func(t *testing.T) itemRepoFixture {
		return itemRepoFixture{items: NewMemoryItemRepo(), rooms: NewMemoryRoomRepo()}
	})
}

func TestSQLiteItemRepo(t *testing.T) {
	runItemRepoTests(t, "sqlite", func(t *testing.T) itemRepoFixture {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return itemRepoFixture{
			items: NewSQLiteItemRepo(conn),
			rooms: NewSQLiteRoomRepo(conn),
		}
	})
}
