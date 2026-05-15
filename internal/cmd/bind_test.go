package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// bindFixture seeds Alice (matching commPair's CharacterID=1, currently
// in room 1) plus a memory RoomRepo with one bindable room (id 1) and
// one non-bindable room (id 2). Tests flip alice.CurrentRoomID between
// them to exercise the gate.
func bindFixture(t *testing.T) (*repo.MemoryCharacterRepo, *repo.MemoryRoomRepo) {
	t.Helper()
	chars := repo.NewMemoryCharacterRepo()
	if _, err := chars.Create(context.Background(), repo.Character{
		AccountID: 100,
		Name:      "Alice",
	}); err != nil {
		t.Fatalf("seed alice: %v", err)
	}
	rooms := repo.NewMemoryRoomRepo()
	// Room 1 is the starter (also the seeded BoundRoomID); plain.
	rooms.Insert(repo.Room{
		ID: 1, ExternalID: "town.square", Name: "Town Square",
	})
	// Room 2 is bindable; the persistence test binds here.
	rooms.Insert(repo.Room{
		ID: 2, ExternalID: "inn.fire", Name: "By the Fire",
		Flags: repo.RoomFlags{Bindable: true},
	})
	// Room 3 is non-bindable; the refusal test stands here.
	rooms.Insert(repo.Room{
		ID: 3, ExternalID: "wild.field", Name: "Empty Field",
	})
	return chars, rooms
}

func TestBind_BindableRoomPersists(t *testing.T) {
	chars, rooms := bindFixture(t)
	_, alice, _, aOut, _ := commPair(t)
	alice.CurrentRoomID = 2

	runCmd(t, NewBind(chars, rooms), alice, "")

	if !strings.Contains(aOut.String(), "bind your spirit") {
		t.Fatalf("missing success line: %q", aOut.String())
	}
	got, err := chars.GetByID(context.Background(), alice.CharacterID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.BoundRoomID != 2 {
		t.Fatalf("BoundRoomID = %d, want 2", got.BoundRoomID)
	}
}

func TestBind_NonBindableRoomRefuses(t *testing.T) {
	chars, rooms := bindFixture(t)
	_, alice, _, aOut, _ := commPair(t)
	alice.CurrentRoomID = 3

	before, _ := chars.GetByID(context.Background(), alice.CharacterID)

	runCmd(t, NewBind(chars, rooms), alice, "")

	if !strings.Contains(aOut.String(), "no connection") {
		t.Fatalf("missing refusal line: %q", aOut.String())
	}
	got, _ := chars.GetByID(context.Background(), alice.CharacterID)
	if got.BoundRoomID != before.BoundRoomID {
		t.Fatalf("BoundRoomID changed despite refusal: %d -> %d", before.BoundRoomID, got.BoundRoomID)
	}
}

func TestBind_AlreadyBoundShortCircuits(t *testing.T) {
	chars, rooms := bindFixture(t)
	_, alice, _, aOut, _ := commPair(t)
	alice.CurrentRoomID = 2

	c := NewBind(chars, rooms)
	runCmd(t, c, alice, "")
	aOut.Reset()
	runCmd(t, c, alice, "")

	if !strings.Contains(aOut.String(), "already bound") {
		t.Fatalf("missing already-bound line: %q", aOut.String())
	}
	got, _ := chars.GetByID(context.Background(), alice.CharacterID)
	if got.BoundRoomID != 2 {
		t.Fatalf("BoundRoomID drifted: %d", got.BoundRoomID)
	}
}

func TestBind_RoomLookupFailureRefuses(t *testing.T) {
	chars, rooms := bindFixture(t)
	_, alice, _, aOut, _ := commPair(t)
	alice.CurrentRoomID = 999 // not in repo

	runCmd(t, NewBind(chars, rooms), alice, "")

	if !strings.Contains(aOut.String(), "cannot bind") {
		t.Fatalf("missing failure line: %q", aOut.String())
	}
	got, _ := chars.GetByID(context.Background(), alice.CharacterID)
	if got.BoundRoomID == 999 {
		t.Fatalf("BoundRoomID written from missing room")
	}
}
