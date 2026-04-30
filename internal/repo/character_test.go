package repo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runCharacterRepoTests(t *testing.T, name string, newRepo func(t *testing.T) (CharacterRepo, AccountRepo)) {
	t.Helper()
	t.Run(name+"/create+find", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		got, err := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Frodo"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if got.ID == 0 {
			t.Fatal("ID not set")
		}
		if got.NameLower != "frodo" {
			t.Fatalf("NameLower = %q", got.NameLower)
		}
		found, err := cr.FindByName(ctx, "FRODO")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if found.ID != got.ID || found.AccountID != acc.ID {
			t.Fatalf("found %+v", found)
		}
	})

	t.Run(name+"/duplicate", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		a1, _ := ar.Create(ctx, Account{Username: "a1", PasswordHash: "h"})
		a2, _ := ar.Create(ctx, Account{Username: "a2", PasswordHash: "h"})
		if _, err := cr.Create(ctx, Character{AccountID: a1.ID, Name: "Sam"}); err != nil {
			t.Fatalf("first create: %v", err)
		}
		_, err := cr.Create(ctx, Character{AccountID: a2.ID, Name: "SAM"})
		if !errors.Is(err, ErrDuplicateCharacterName) {
			t.Fatalf("err = %v, want ErrDuplicateCharacterName", err)
		}
	})

	t.Run(name+"/missing", func(t *testing.T) {
		cr, _ := newRepo(t)
		_, err := cr.FindByName(context.Background(), "ghost")
		if !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("err = %v, want ErrCharacterNotFound", err)
		}
	})

	t.Run(name+"/list_by_account_orders_recent_first", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		// Three characters; only the second has a last_played_at.
		c1, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "alpha"})
		c2, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "beta"})
		c3, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "gamma"})
		_ = cr.RecordPlay(ctx, c2.ID, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC))

		got, err := cr.ListByAccount(ctx, acc.ID)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("got %d, want 3", len(got))
		}
		// Played first; then alphabetical.
		want := []int64{c2.ID, c1.ID, c3.ID}
		for i, id := range want {
			if got[i].ID != id {
				t.Fatalf("position %d: got id %d, want %d (got=%+v)", i, got[i].ID, id, got)
			}
		}
	})

	t.Run(name+"/create_defaults_to_starter_room", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		got, err := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Pippin"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if got.CurrentRoomID != StarterRoomID {
			t.Fatalf("CurrentRoomID = %d, want %d", got.CurrentRoomID, StarterRoomID)
		}
		// Round-trip via FindByName.
		found, err := cr.FindByName(ctx, "Pippin")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if found.CurrentRoomID != StarterRoomID {
			t.Fatalf("found.CurrentRoomID = %d, want %d", found.CurrentRoomID, StarterRoomID)
		}
	})

	t.Run(name+"/record_room_persists", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Merry"})
		if err := cr.RecordRoom(ctx, c.ID, 42); err != nil {
			t.Fatalf("RecordRoom: %v", err)
		}
		found, err := cr.FindByName(ctx, "Merry")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if found.CurrentRoomID != 42 {
			t.Fatalf("CurrentRoomID = %d, want 42", found.CurrentRoomID)
		}
	})

	t.Run(name+"/list_by_account_isolates", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		a1, _ := ar.Create(ctx, Account{Username: "a1", PasswordHash: "h"})
		a2, _ := ar.Create(ctx, Account{Username: "a2", PasswordHash: "h"})
		_, _ = cr.Create(ctx, Character{AccountID: a1.ID, Name: "alpha"})
		_, _ = cr.Create(ctx, Character{AccountID: a2.ID, Name: "beta"})
		got, _ := cr.ListByAccount(ctx, a1.ID)
		if len(got) != 1 || got[0].Name != "alpha" {
			t.Fatalf("a1 got %+v", got)
		}
	})
}

func TestMemoryCharacterRepo(t *testing.T) {
	runCharacterRepoTests(t, "memory", func(t *testing.T) (CharacterRepo, AccountRepo) {
		return NewMemoryCharacterRepo(), NewMemoryAccountRepo()
	})
}

func TestSQLiteCharacterRepo(t *testing.T) {
	runCharacterRepoTests(t, "sqlite", func(t *testing.T) (CharacterRepo, AccountRepo) {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteCharacterRepo(conn), NewSQLiteAccountRepo(conn)
	})
}
