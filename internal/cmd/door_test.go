package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// doorFixture stages a two-room world with a single closed exit
// from room 1 -> 2 plus a reverse exit from 2 -> 1, two sessions
// (alice in room 1, bob in room 2), and an item repo for key tests.
type doorFixture struct {
	exits     *repo.MemoryExitRepo
	items     *repo.MemoryItemRepo
	sessions  *session.Registry
	alice     *telnet.Session
	bob       *telnet.Session
	aOut      *bufConn
	bOut      *bufConn
	northExit repo.Exit
	southExit repo.Exit
}

func newDoorFixture(t *testing.T) *doorFixture {
	t.Helper()
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	sessions := session.NewRegistry()

	north := exits.Insert(repo.Exit{
		FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth,
		Flags: repo.ExitFlags{Closed: true, Pickable: true},
	})
	south := exits.Insert(repo.Exit{
		FromRoomID: 2, ToRoomID: 1, Direction: repo.DirSouth,
		Flags: repo.ExitFlags{Closed: true, Pickable: true},
	})

	a, aOut := bufSession(t)
	a.AccountID = 100
	a.AuthLevel = telnet.AuthPlayer
	a.CharacterID = 1
	a.CharacterName = "Alice"
	a.CurrentRoomID = 1
	sessions.Bind(a.AccountID, a)

	b, bOut := bufSession(t)
	b.AccountID = 200
	b.AuthLevel = telnet.AuthPlayer
	b.CharacterID = 2
	b.CharacterName = "Bob"
	b.CurrentRoomID = 2
	sessions.Bind(b.AccountID, b)

	return &doorFixture{
		exits: exits, items: items, sessions: sessions,
		alice: a, bob: b, aOut: aOut, bOut: bOut,
		northExit: north, southExit: south,
	}
}

// freshExit reloads the canonical state from the repo so assertions
// don't read a stale snapshot taken before the verb mutated.
func (f *doorFixture) freshNorth(t *testing.T) repo.Exit {
	t.Helper()
	e, err := f.exits.FindByDirection(context.Background(), 1, repo.DirNorth)
	if err != nil {
		t.Fatalf("freshNorth: %v", err)
	}
	return e
}

func TestOpen_OpensClosedDoorAndBroadcastsBothSides(t *testing.T) {
	f := newDoorFixture(t)
	open := NewOpen(f.exits, f.sessions)

	runCmd(t, open, f.alice, "north")

	if e := f.freshNorth(t); e.Flags.Closed {
		t.Fatalf("door still closed: %+v", e.Flags)
	}
	if !strings.Contains(f.aOut.String(), "You open the north door") {
		t.Fatalf("alice: missing self echo; got %q", f.aOut.String())
	}
	if !strings.Contains(f.bOut.String(), "south door swings open") {
		t.Fatalf("bob: missing far-side broadcast; got %q", f.bOut.String())
	}
}

func TestOpen_RefusesLocked(t *testing.T) {
	f := newDoorFixture(t)
	if err := f.exits.UpdateFlags(context.Background(), f.northExit.ID, true, true); err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	open := NewOpen(f.exits, f.sessions)

	runCmd(t, open, f.alice, "north")

	if !strings.Contains(f.aOut.String(), "It is locked") {
		t.Fatalf("missing lock refusal; got %q", f.aOut.String())
	}
	if e := f.freshNorth(t); !e.Flags.Closed || !e.Flags.Locked {
		t.Fatalf("flags mutated: %+v", e.Flags)
	}
}

func TestOpen_AlreadyOpen(t *testing.T) {
	f := newDoorFixture(t)
	if err := f.exits.UpdateFlags(context.Background(), f.northExit.ID, false, false); err != nil {
		t.Fatalf("seed open: %v", err)
	}
	open := NewOpen(f.exits, f.sessions)

	runCmd(t, open, f.alice, "north")

	if !strings.Contains(f.aOut.String(), "already open") {
		t.Fatalf("missing already-open refusal; got %q", f.aOut.String())
	}
}

func TestClose_ClosesAndBroadcasts(t *testing.T) {
	f := newDoorFixture(t)
	if err := f.exits.UpdateFlags(context.Background(), f.northExit.ID, false, false); err != nil {
		t.Fatalf("seed open: %v", err)
	}
	if err := f.exits.UpdateFlags(context.Background(), f.southExit.ID, false, false); err != nil {
		t.Fatalf("seed open south: %v", err)
	}
	close := NewClose(f.exits, f.sessions)

	runCmd(t, close, f.alice, "n")

	if e := f.freshNorth(t); !e.Flags.Closed {
		t.Fatalf("door not closed: %+v", e.Flags)
	}
	if !strings.Contains(f.aOut.String(), "You close the north door") {
		t.Fatalf("alice: missing self echo; got %q", f.aOut.String())
	}
	if !strings.Contains(f.bOut.String(), "south door swings shut") {
		t.Fatalf("bob: missing far-side broadcast; got %q", f.bOut.String())
	}
}

func TestLock_RequiresKeyInRoom(t *testing.T) {
	f := newDoorFixture(t)
	// Set a key requirement that nothing in the room satisfies.
	f.northExit.KeyExternalID = "iron.key"
	f.exits = repo.NewMemoryExitRepo()
	f.northExit = f.exits.Insert(f.northExit)
	lock := NewLock(f.exits, f.items, f.sessions)

	runCmd(t, lock, f.alice, "north")

	if !strings.Contains(f.aOut.String(), "don't have the key") {
		t.Fatalf("missing key refusal; got %q", f.aOut.String())
	}
	if e := f.freshNorth(t); e.Flags.Locked {
		t.Fatalf("door locked despite missing key: %+v", e.Flags)
	}
}

func TestLock_KeyInInventorySucceeds(t *testing.T) {
	f := newDoorFixture(t)
	exits := repo.NewMemoryExitRepo()
	exits.Insert(repo.Exit{
		FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth,
		Flags:         repo.ExitFlags{Closed: true, Pickable: true},
		KeyExternalID: "iron.key",
	})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 1, Direction: repo.DirSouth, Flags: repo.ExitFlags{Closed: true}})
	f.exits = exits
	// Key in Alice's inventory — §14 retired the room-floor placeholder.
	f.items.Insert(repo.Item{
		ExternalID: "iron.key", Name: "an iron key",
		OwnerCharacterID: f.alice.CharacterID,
		Type:             repo.ItemTypeKey, Stats: &repo.KeyStats{KeyID: "iron.key"},
	})

	lock := NewLock(f.exits, f.items, f.sessions)

	runCmd(t, lock, f.alice, "north")

	if !strings.Contains(f.aOut.String(), "You lock the north door") {
		t.Fatalf("missing self echo; got %q", f.aOut.String())
	}
	got, err := f.exits.FindByDirection(context.Background(), 1, repo.DirNorth)
	if err != nil {
		t.Fatalf("FindByDirection: %v", err)
	}
	if !got.Flags.Locked || !got.Flags.Closed {
		t.Fatalf("door not locked: %+v", got.Flags)
	}
}

// TestLock_KeyOnRoomFloorRefuses confirms the §14 swap actually
// retired the room-floor placeholder — a key sitting on the floor
// is no longer enough.
func TestLock_KeyOnRoomFloorRefuses(t *testing.T) {
	f := newDoorFixture(t)
	exits := repo.NewMemoryExitRepo()
	exits.Insert(repo.Exit{
		FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth,
		Flags:         repo.ExitFlags{Closed: true, Pickable: true},
		KeyExternalID: "iron.key",
	})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 1, Direction: repo.DirSouth, Flags: repo.ExitFlags{Closed: true}})
	f.exits = exits
	f.items.Insert(repo.Item{
		ExternalID: "iron.key", Name: "an iron key", RoomID: 1,
		Type: repo.ItemTypeKey, Stats: &repo.KeyStats{KeyID: "iron.key"},
	})

	lock := NewLock(f.exits, f.items, f.sessions)
	runCmd(t, lock, f.alice, "north")

	if !strings.Contains(f.aOut.String(), "don't have the key") {
		t.Fatalf("expected key refusal; got %q", f.aOut.String())
	}
}

func TestLock_AdminBypassesKey(t *testing.T) {
	f := newDoorFixture(t)
	f.alice.AuthLevel = telnet.AuthAdmin
	lock := NewLock(f.exits, f.items, f.sessions)

	runCmd(t, lock, f.alice, "north")

	if e := f.freshNorth(t); !e.Flags.Locked {
		t.Fatalf("admin lock didn't take: %+v", e.Flags)
	}
}

func TestLock_RefusesOpenDoor(t *testing.T) {
	f := newDoorFixture(t)
	if err := f.exits.UpdateFlags(context.Background(), f.northExit.ID, false, false); err != nil {
		t.Fatalf("seed open: %v", err)
	}
	f.alice.AuthLevel = telnet.AuthAdmin
	lock := NewLock(f.exits, f.items, f.sessions)

	runCmd(t, lock, f.alice, "north")

	if !strings.Contains(f.aOut.String(), "must close it first") {
		t.Fatalf("missing close-first refusal; got %q", f.aOut.String())
	}
}

func TestUnlock_LeavesDoorClosed(t *testing.T) {
	f := newDoorFixture(t)
	if err := f.exits.UpdateFlags(context.Background(), f.northExit.ID, true, true); err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	f.alice.AuthLevel = telnet.AuthAdmin
	unlock := NewUnlock(f.exits, f.items, f.sessions)

	runCmd(t, unlock, f.alice, "north")

	got := f.freshNorth(t)
	if got.Flags.Locked {
		t.Errorf("still locked: %+v", got.Flags)
	}
	if !got.Flags.Closed {
		t.Errorf("door opened on unlock; want still closed: %+v", got.Flags)
	}
}

func TestUnlock_NotLocked(t *testing.T) {
	f := newDoorFixture(t)
	unlock := NewUnlock(f.exits, f.items, f.sessions)

	runCmd(t, unlock, f.alice, "north")

	if !strings.Contains(f.aOut.String(), "not locked") {
		t.Fatalf("missing not-locked refusal; got %q", f.aOut.String())
	}
}

func TestPick_PlayerLacksSkill(t *testing.T) {
	f := newDoorFixture(t)
	if err := f.exits.UpdateFlags(context.Background(), f.northExit.ID, true, true); err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	pick := NewPick(f.exits, f.sessions)

	runCmd(t, pick, f.alice, "north")

	if !strings.Contains(f.aOut.String(), "lack the skill") {
		t.Fatalf("missing skill refusal; got %q", f.aOut.String())
	}
	if e := f.freshNorth(t); !e.Flags.Locked {
		t.Fatalf("door unlocked despite skill block: %+v", e.Flags)
	}
}

func TestPick_AdminSucceeds(t *testing.T) {
	f := newDoorFixture(t)
	if err := f.exits.UpdateFlags(context.Background(), f.northExit.ID, true, true); err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	f.alice.AuthLevel = telnet.AuthAdmin
	pick := NewPick(f.exits, f.sessions)

	runCmd(t, pick, f.alice, "north")

	got := f.freshNorth(t)
	if got.Flags.Locked {
		t.Fatalf("admin pick failed: %+v", got.Flags)
	}
}

func TestPick_NotPickable(t *testing.T) {
	f := newDoorFixture(t)
	exits := repo.NewMemoryExitRepo()
	exits.Insert(repo.Exit{
		FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth,
		Flags: repo.ExitFlags{Closed: true, Locked: true, Pickable: false},
	})
	f.exits = exits
	f.alice.AuthLevel = telnet.AuthAdmin
	pick := NewPick(f.exits, f.sessions)

	runCmd(t, pick, f.alice, "north")

	if !strings.Contains(f.aOut.String(), "cannot be picked") {
		t.Fatalf("missing pickable=false refusal; got %q", f.aOut.String())
	}
}

func TestDoor_UnknownDirection(t *testing.T) {
	f := newDoorFixture(t)
	open := NewOpen(f.exits, f.sessions)

	runCmd(t, open, f.alice, "sideways")

	if !strings.Contains(f.aOut.String(), "isn't a direction") {
		t.Fatalf("missing direction refusal; got %q", f.aOut.String())
	}
}

func TestDoor_NoExit(t *testing.T) {
	f := newDoorFixture(t)
	open := NewOpen(f.exits, f.sessions)

	runCmd(t, open, f.alice, "south")

	if !strings.Contains(f.aOut.String(), "no door that way") {
		t.Fatalf("missing no-door refusal; got %q", f.aOut.String())
	}
}

func TestClose_RefusesNoPass(t *testing.T) {
	f := newDoorFixture(t)
	exits := repo.NewMemoryExitRepo()
	exits.Insert(repo.Exit{
		FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth,
		Flags: repo.ExitFlags{NoPass: true},
	})
	f.exits = exits
	close := NewClose(f.exits, f.sessions)

	runCmd(t, close, f.alice, "north")

	if !strings.Contains(f.aOut.String(), "no door there to close") {
		t.Fatalf("missing nopass refusal; got %q", f.aOut.String())
	}
}

func TestDoor_SafeActorStripsCfmtBytes(t *testing.T) {
	// character_create restricts names to [A-Za-z0-9_-], but the door
	// verbs interpolate CharacterName into a cfmt {{...}} payload, so
	// they defang `{`, `}`, `:` defensively. The actor's name only
	// appears in the near-room broadcast, so the assertion needs a
	// witness in alice's own room rather than relying on the far echo.
	f := newDoorFixture(t)
	f.alice.CharacterName = "Eve}}::red {{boom}}::"
	witness, wOut := bufSession(t)
	witness.AccountID = 300
	witness.AuthLevel = telnet.AuthPlayer
	witness.CharacterID = 3
	witness.CharacterName = "Carol"
	witness.CurrentRoomID = 1
	f.sessions.Bind(witness.AccountID, witness)

	open := NewOpen(f.exits, f.sessions)
	runCmd(t, open, f.alice, "north")

	out := wOut.String()
	if strings.Contains(out, "}}::red") || strings.Contains(out, "{{boom") {
		t.Fatalf("unsanitized cfmt reached witness: %q", out)
	}
	// `{`, `}`, `:` stripped → "Eve}}::red {{boom}}::" → "Evered boom".
	if !strings.Contains(out, "Evered boom opens the north door") {
		t.Fatalf("safeActor produced unexpected name; got %q", out)
	}
}

func TestDoor_HiddenIsNoDoor(t *testing.T) {
	f := newDoorFixture(t)
	exits := repo.NewMemoryExitRepo()
	exits.Insert(repo.Exit{
		FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth,
		Flags: repo.ExitFlags{Closed: true, Hidden: true},
	})
	f.exits = exits
	open := NewOpen(f.exits, f.sessions)

	runCmd(t, open, f.alice, "north")

	if !strings.Contains(f.aOut.String(), "no door that way") {
		t.Fatalf("hidden exit revealed itself; got %q", f.aOut.String())
	}
}
