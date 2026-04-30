package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runRoomRepoTests(t *testing.T, name string, newRepo func(t *testing.T) RoomRepo) {
	t.Helper()
	t.Run(name+"/create_and_find", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		got, err := r.Create(ctx, Room{ExternalID: "plaza.fountain", Name: "Town Plaza", LongDesc: "Cobblestones."})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if got.ID == 0 {
			t.Fatal("ID not assigned")
		}

		byID, err := r.FindByID(ctx, got.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if byID.ExternalID != "plaza.fountain" || byID.Name != "Town Plaza" {
			t.Fatalf("FindByID round-trip: %+v", byID)
		}

		byExt, err := r.FindByExternalID(ctx, "plaza.fountain")
		if err != nil {
			t.Fatalf("FindByExternalID: %v", err)
		}
		if byExt.ID != got.ID {
			t.Fatalf("FindByExternalID = %d, want %d", byExt.ID, got.ID)
		}
	})

	t.Run(name+"/create_with_pinned_id", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		got, err := r.Create(ctx, Room{ID: StarterRoomID, ExternalID: "plaza.fountain", Name: "Plaza"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if got.ID != StarterRoomID {
			t.Fatalf("got ID %d, want %d", got.ID, StarterRoomID)
		}
	})

	t.Run(name+"/create_rejects_empty_external_id", func(t *testing.T) {
		r := newRepo(t)
		_, err := r.Create(context.Background(), Room{Name: "Anywhere"})
		if !errors.Is(err, ErrInvalidExternalID) {
			t.Fatalf("err = %v, want ErrInvalidExternalID", err)
		}
	})

	t.Run(name+"/create_duplicate_external_id", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Room{ExternalID: "x", Name: "first"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		_, err := r.Create(ctx, Room{ExternalID: "x", Name: "second"})
		if !errors.Is(err, ErrDuplicateExternalID) {
			t.Fatalf("err = %v, want ErrDuplicateExternalID", err)
		}
	})

	t.Run(name+"/find_missing", func(t *testing.T) {
		r := newRepo(t)
		_, err := r.FindByID(context.Background(), 99999)
		if !errors.Is(err, ErrRoomNotFound) {
			t.Fatalf("FindByID err = %v", err)
		}
		_, err = r.FindByExternalID(context.Background(), "nope")
		if !errors.Is(err, ErrRoomNotFound) {
			t.Fatalf("FindByExternalID err = %v", err)
		}
	})
}

func TestMemoryRoomRepo(t *testing.T) {
	runRoomRepoTests(t, "memory", func(t *testing.T) RoomRepo {
		return NewMemoryRoomRepo()
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
