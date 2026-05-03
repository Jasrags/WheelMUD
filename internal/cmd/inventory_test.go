package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// invFixture stages two players in room 1 with empty inventories.
// The character repo is seeded so character IDs match what commPair
// stamps onto the sessions, so RecordInventory / RecordCoin write
// through cleanly.
type invFixture struct {
	items      *repo.MemoryItemRepo
	characters *repo.MemoryCharacterRepo
	sessions   *session.Registry
	alice      *telnet.Session
	bob        *telnet.Session
	aOut       *bufConn
	bOut       *bufConn
}

func newInvFixture(t *testing.T) *invFixture {
	t.Helper()
	items := repo.NewMemoryItemRepo()
	characters := repo.NewMemoryCharacterRepo()

	// Seed the character rows in the same order commPair binds the
	// session CharacterIDs (1 = Alice, 2 = Bob). Strength = 14 gives a
	// 175 lb heavy cap (10 → 100, ×~1.749 by step 4) — plenty of room
	// for normal items and a small bumper for the over-encumbrance test.
	mkChar := func(name string, accountID int64) {
		c := repo.Character{
			AccountID: accountID, Name: name, CurrentRoomID: 1,
			Core: creature.Core{Abilities: creature.Abilities{
				Str: creature.AbilityScore{Current: 14, Max: 14, Inherent: 14},
			}},
		}
		if _, err := characters.Create(context.Background(), c); err != nil {
			t.Fatalf("seed character %q: %v", name, err)
		}
	}
	mkChar("Alice", 100)
	mkChar("Bob", 200)

	sessions, alice, bob, aOut, bOut := commPair(t)
	return &invFixture{
		items: items, characters: characters, sessions: sessions,
		alice: alice, bob: bob, aOut: aOut, bOut: bOut,
	}
}

func TestInventory_EmptyShowsNothing(t *testing.T) {
	f := newInvFixture(t)
	runCmd(t, NewInventory(f.items, f.characters), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "(nothing)") {
		t.Fatalf("missing empty marker; got %q", out)
	}
	if !strings.Contains(out, "empty purse") {
		t.Fatalf("missing empty purse line; got %q", out)
	}
	if !strings.Contains(out, "light load") {
		t.Fatalf("missing load tag; got %q", out)
	}
}

func TestGet_PicksUpAndBroadcasts(t *testing.T) {
	f := newInvFixture(t)
	f.items.Insert(repo.Item{ExternalID: "rock", Name: "a smooth rock", RoomID: 1, Weight: 2})

	runCmd(t, NewGet(f.items, f.characters, f.sessions), f.alice, "rock")

	if !strings.Contains(f.aOut.String(), "You pick up a smooth rock") {
		t.Fatalf("alice: missing self echo; got %q", f.aOut.String())
	}
	if !strings.Contains(f.bOut.String(), "Alice picks up a smooth rock") {
		t.Fatalf("bob: missing room broadcast; got %q", f.bOut.String())
	}
	held, _ := f.items.ListInInventory(context.Background(), f.alice.CharacterID)
	if len(held) != 1 || held[0].Name != "a smooth rock" {
		t.Fatalf("inventory: got %+v", held)
	}
	floor, _ := f.items.ListInRoom(context.Background(), 1)
	if len(floor) != 0 {
		t.Fatalf("floor still holds item: %+v", floor)
	}
	// JSON ordering hint persisted.
	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if len(c.Inventory) != 1 || c.Inventory[0] != held[0].ID {
		t.Fatalf("character.Inventory not updated: %+v", c.Inventory)
	}
}

func TestGet_NoTakeRefuses(t *testing.T) {
	f := newInvFixture(t)
	f.items.Insert(repo.Item{ExternalID: "altar", Name: "a granite altar", RoomID: 1, Flags: repo.FlagNoTake})
	runCmd(t, NewGet(f.items, f.characters, f.sessions), f.alice, "altar")
	if !strings.Contains(f.aOut.String(), "can't take that") {
		t.Fatalf("got %q", f.aOut.String())
	}
}

func TestGet_OverloadedRefuses(t *testing.T) {
	f := newInvFixture(t)
	// Str 14 → heavy cap ~175 lb. A 200 lb anvil overloads on its own.
	f.items.Insert(repo.Item{ExternalID: "anvil", Name: "a heavy anvil", RoomID: 1, Weight: 500})
	runCmd(t, NewGet(f.items, f.characters, f.sessions), f.alice, "anvil")
	if !strings.Contains(f.aOut.String(), "too heavy") {
		t.Fatalf("got %q", f.aOut.String())
	}
}

func TestGet_MissingItem(t *testing.T) {
	f := newInvFixture(t)
	runCmd(t, NewGet(f.items, f.characters, f.sessions), f.alice, "nothing")
	if !strings.Contains(f.aOut.String(), "don't see that here") {
		t.Fatalf("got %q", f.aOut.String())
	}
}

func TestDrop_PutsBackOnFloor(t *testing.T) {
	f := newInvFixture(t)
	it := f.items.Insert(repo.Item{ExternalID: "rock", Name: "a smooth rock", OwnerCharacterID: f.alice.CharacterID, Weight: 2})
	// Mirror what `get` would have done to the JSON ordering.
	_ = f.characters.RecordInventory(context.Background(), f.alice.CharacterID, []int64{it.ID})

	runCmd(t, NewDrop(f.items, f.characters, f.sessions), f.alice, "rock")

	if !strings.Contains(f.aOut.String(), "You drop a smooth rock") {
		t.Fatalf("alice echo: %q", f.aOut.String())
	}
	if !strings.Contains(f.bOut.String(), "Alice drops") {
		t.Fatalf("bob broadcast: %q", f.bOut.String())
	}
	floor, _ := f.items.ListInRoom(context.Background(), 1)
	if len(floor) != 1 {
		t.Fatalf("floor: %+v", floor)
	}
	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if len(c.Inventory) != 0 {
		t.Fatalf("character.Inventory not cleared: %+v", c.Inventory)
	}
}

func TestDrop_NoDropRefuses(t *testing.T) {
	f := newInvFixture(t)
	f.items.Insert(repo.Item{ExternalID: "ring", Name: "a cursed ring", OwnerCharacterID: f.alice.CharacterID, Flags: repo.FlagNoDrop})
	runCmd(t, NewDrop(f.items, f.characters, f.sessions), f.alice, "ring")
	if !strings.Contains(f.aOut.String(), "stuck to your hand") {
		t.Fatalf("got %q", f.aOut.String())
	}
}

func TestGive_ItemTransfers(t *testing.T) {
	f := newInvFixture(t)
	it := f.items.Insert(repo.Item{ExternalID: "letter", Name: "a sealed letter", OwnerCharacterID: f.alice.CharacterID})

	runCmd(t, NewGive(f.items, f.characters, f.sessions), f.alice, "letter Bob")

	if !strings.Contains(f.aOut.String(), "You give a sealed letter to Bob") {
		t.Fatalf("alice: %q", f.aOut.String())
	}
	if !strings.Contains(f.bOut.String(), "Alice gives you a sealed letter") {
		t.Fatalf("bob: %q", f.bOut.String())
	}
	got, _ := f.items.GetByID(context.Background(), it.ID)
	if got.OwnerCharacterID != f.bob.CharacterID {
		t.Fatalf("owner not transferred: %+v", got)
	}
}

func TestGive_TargetNotInRoom(t *testing.T) {
	f := newInvFixture(t)
	f.bob.CurrentRoomID = 99
	f.items.Insert(repo.Item{ExternalID: "letter", Name: "a sealed letter", OwnerCharacterID: f.alice.CharacterID})

	runCmd(t, NewGive(f.items, f.characters, f.sessions), f.alice, "letter Bob")
	if !strings.Contains(f.aOut.String(), "aren't here") {
		t.Fatalf("got %q", f.aOut.String())
	}
}

func TestGive_CoinTransfersBothPurses(t *testing.T) {
	f := newInvFixture(t)
	// Seed Alice with 1gc 5sp = 1050 cp.
	if err := f.characters.RecordCoin(context.Background(), f.alice.CharacterID, currency.MustNew(1, 0, 5, 0), 0); err != nil {
		t.Fatalf("seed coin: %v", err)
	}

	runCmd(t, NewGive(f.items, f.characters, f.sessions), f.alice, "5sp Bob")

	if !strings.Contains(f.aOut.String(), "You hand Bob") {
		t.Fatalf("alice: %q", f.aOut.String())
	}
	if !strings.Contains(f.bOut.String(), "Alice hands you") {
		t.Fatalf("bob: %q", f.bOut.String())
	}
	a, _ := f.characters.FindByName(context.Background(), "Alice")
	b, _ := f.characters.FindByName(context.Background(), "Bob")
	if a.Coin != currency.Amount(1000) {
		t.Fatalf("alice coin = %d, want 1000", a.Coin)
	}
	if b.Coin != currency.Amount(50) {
		t.Fatalf("bob coin = %d, want 50", b.Coin)
	}
}

func TestGive_CoinInsufficient(t *testing.T) {
	f := newInvFixture(t)
	runCmd(t, NewGive(f.items, f.characters, f.sessions), f.alice, "10gc Bob")
	if !strings.Contains(f.aOut.String(), "don't have that much") {
		t.Fatalf("got %q", f.aOut.String())
	}
}

// TestGet_RaceLoserGetsRefusal proves the CAS swap retired the
// silent-overwrite path: when two players see the same floor item,
// the second `get` resolves to a clear refusal instead of stealing
// the item from the winner.
func TestGet_RaceLoserGetsRefusal(t *testing.T) {
	f := newInvFixture(t)
	f.items.Insert(repo.Item{ExternalID: "rock", Name: "a rock", RoomID: 1, Weight: 1})

	get := NewGet(f.items, f.characters, f.sessions)
	runCmd(t, get, f.alice, "rock") // alice wins
	runCmd(t, get, f.bob, "rock")   // bob's snapshot still showed it

	if !strings.Contains(f.bOut.String(), "Someone else got there first") &&
		!strings.Contains(f.bOut.String(), "don't see that here") {
		t.Fatalf("bob: missing race-lost message; got %q", f.bOut.String())
	}
	held, _ := f.items.ListInInventory(context.Background(), f.alice.CharacterID)
	if len(held) != 1 {
		t.Fatalf("alice should have the item; got %+v", held)
	}
	bobHeld, _ := f.items.ListInInventory(context.Background(), f.bob.CharacterID)
	if len(bobHeld) != 0 {
		t.Fatalf("bob should not have the item; got %+v", bobHeld)
	}
}

func TestKeyword_OrdinalMatch(t *testing.T) {
	// Token-prefix matching: each name must contain a whitespace-
	// separated token that begins with the keyword. All three items
	// share the "sword" token so the ordinal can pick between them.
	items := []repo.Item{
		{ID: 1, Name: "a long sword"},
		{ID: 2, Name: "a short sword"},
		{ID: 3, Name: "a rusty sword"},
	}
	tests := []struct {
		target string
		want   int64
	}{
		{"sword", 1},
		{"1.sword", 1},
		{"2.sword", 2},
		{"3.sword", 3},
		{"long", 1},
		{"rust", 3},
	}
	for _, tt := range tests {
		got, ok := MatchItem(tt.target, items)
		if !ok || got.ID != tt.want {
			t.Errorf("MatchItem(%q): got id=%d ok=%v, want id=%d", tt.target, got.ID, ok, tt.want)
		}
	}
	if _, ok := MatchItem("4.sword", items); ok {
		t.Errorf("MatchItem(4.sword) should miss (only 3 swords)")
	}
	if _, ok := MatchItem("", items); ok {
		t.Errorf("MatchItem(empty) should miss")
	}
}
