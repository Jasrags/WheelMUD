package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runExitRepoTests(t *testing.T, name string, newRepo func(t *testing.T) ExitRepo) {
	t.Helper()
	t.Run(name+"/list_from_starter", func(t *testing.T) {
		r := newRepo(t)
		got, err := r.ListFrom(context.Background(), StarterRoomID)
		if err != nil {
			t.Fatalf("ListFrom: %v", err)
		}
		// The seed zone gives the starter at least one exit.
		if len(got) == 0 {
			t.Fatal("starter room has no exits; seed migration probably regressed")
		}
		// Sorted by direction.
		for i := 1; i < len(got); i++ {
			if got[i-1].Direction > got[i].Direction {
				t.Fatalf("exits not sorted by direction: %+v", got)
			}
		}
	})

	t.Run(name+"/list_from_unknown_room", func(t *testing.T) {
		r := newRepo(t)
		got, err := r.ListFrom(context.Background(), 99999)
		if err != nil {
			t.Fatalf("ListFrom: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d exits, want 0", len(got))
		}
	})

	t.Run(name+"/find_by_direction", func(t *testing.T) {
		r := newRepo(t)
		exits, err := r.ListFrom(context.Background(), StarterRoomID)
		if err != nil || len(exits) == 0 {
			t.Fatalf("setup ListFrom: %v / %d", err, len(exits))
		}
		want := exits[0]
		got, err := r.FindByDirection(context.Background(), StarterRoomID, want.Direction)
		if err != nil {
			t.Fatalf("FindByDirection: %v", err)
		}
		if got != want {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run(name+"/find_by_direction_missing", func(t *testing.T) {
		r := newRepo(t)
		// 'q' isn't a valid direction in the SQL CHECK constraint, but
		// FindByDirection just returns ErrExitNotFound — the lookup never
		// inserts.
		_, err := r.FindByDirection(context.Background(), StarterRoomID, "q")
		if !errors.Is(err, ErrExitNotFound) {
			t.Fatalf("err = %v, want ErrExitNotFound", err)
		}
	})
}

func TestMemoryExitRepo(t *testing.T) {
	runExitRepoTests(t, "memory", func(t *testing.T) ExitRepo {
		r := NewMemoryExitRepo()
		r.Insert(Exit{FromRoomID: StarterRoomID, ToRoomID: 2, Direction: DirNorth})
		r.Insert(Exit{FromRoomID: StarterRoomID, ToRoomID: 3, Direction: DirSouth})
		return r
	})
}

func TestSQLiteExitRepo(t *testing.T) {
	runExitRepoTests(t, "sqlite", func(t *testing.T) ExitRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteExitRepo(conn)
	})
}
