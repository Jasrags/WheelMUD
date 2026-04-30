package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

// exitRepoFixture wires both an ExitRepo and a RoomRepo so tests can
// create rooms (the exit FK requires real room ids on the SQLite side)
// before exercising the exit interface.
type exitRepoFixture struct {
	exits ExitRepo
	rooms RoomRepo
}

func runExitRepoTests(t *testing.T, name string, newFix func(t *testing.T) exitRepoFixture) {
	t.Helper()

	makeRooms := func(t *testing.T, fix exitRepoFixture) (a, b int64) {
		t.Helper()
		ctx := context.Background()
		ra, err := fix.rooms.Create(ctx, Room{ExternalID: "a", Name: "A"})
		if err != nil {
			t.Fatalf("create A: %v", err)
		}
		rb, err := fix.rooms.Create(ctx, Room{ExternalID: "b", Name: "B"})
		if err != nil {
			t.Fatalf("create B: %v", err)
		}
		return ra.ID, rb.ID
	}

	t.Run(name+"/create_then_list_and_find", func(t *testing.T) {
		fix := newFix(t)
		a, b := makeRooms(t, fix)
		ctx := context.Background()
		_, err := fix.exits.Create(ctx, Exit{FromRoomID: a, ToRoomID: b, Direction: DirNorth})
		if err != nil {
			t.Fatalf("Create north: %v", err)
		}
		_, err = fix.exits.Create(ctx, Exit{FromRoomID: a, ToRoomID: b, Direction: DirEast})
		if err != nil {
			t.Fatalf("Create east: %v", err)
		}

		got, err := fix.exits.ListFrom(ctx, a)
		if err != nil {
			t.Fatalf("ListFrom: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("ListFrom got %d, want 2", len(got))
		}
		// Sorted by direction.
		if got[0].Direction != DirEast || got[1].Direction != DirNorth {
			t.Fatalf("not sorted: %+v", got)
		}

		north, err := fix.exits.FindByDirection(ctx, a, DirNorth)
		if err != nil {
			t.Fatalf("FindByDirection: %v", err)
		}
		if north.ToRoomID != b {
			t.Fatalf("ToRoomID = %d, want %d", north.ToRoomID, b)
		}
	})

	t.Run(name+"/create_duplicate_direction", func(t *testing.T) {
		fix := newFix(t)
		a, b := makeRooms(t, fix)
		ctx := context.Background()
		if _, err := fix.exits.Create(ctx, Exit{FromRoomID: a, ToRoomID: b, Direction: DirNorth}); err != nil {
			t.Fatalf("first Create: %v", err)
		}
		_, err := fix.exits.Create(ctx, Exit{FromRoomID: a, ToRoomID: b, Direction: DirNorth})
		if !errors.Is(err, ErrDuplicateExit) {
			t.Fatalf("err = %v, want ErrDuplicateExit", err)
		}
	})

	t.Run(name+"/list_empty_room_is_not_an_error", func(t *testing.T) {
		fix := newFix(t)
		got, err := fix.exits.ListFrom(context.Background(), 99999)
		if err != nil {
			t.Fatalf("ListFrom: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d exits, want 0", len(got))
		}
	})

	t.Run(name+"/find_by_direction_missing", func(t *testing.T) {
		fix := newFix(t)
		_, err := fix.exits.FindByDirection(context.Background(), 99999, DirNorth)
		if !errors.Is(err, ErrExitNotFound) {
			t.Fatalf("err = %v, want ErrExitNotFound", err)
		}
	})
}

func TestMemoryExitRepo(t *testing.T) {
	runExitRepoTests(t, "memory", func(t *testing.T) exitRepoFixture {
		return exitRepoFixture{exits: NewMemoryExitRepo(), rooms: NewMemoryRoomRepo()}
	})
}

func TestSQLiteExitRepo(t *testing.T) {
	runExitRepoTests(t, "sqlite", func(t *testing.T) exitRepoFixture {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return exitRepoFixture{
			exits: NewSQLiteExitRepo(conn),
			rooms: NewSQLiteRoomRepo(conn),
		}
	})
}
