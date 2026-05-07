package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// attackPvPFixture stages a room (optionally nopvp), commPair-aligned
// Alice (ID=1) and Bob (ID=2) characters with the supplied opt-in
// state and level (single-class Armsman so characterLevel sums
// cleanly), and a Manager wired to the in-memory repos.
type attackPvPFixture struct {
	mgr        *combat.Manager
	rooms      *repo.MemoryRoomRepo
	mobs       *repo.MemoryMobInstanceRepo
	characters *repo.MemoryCharacterRepo
}

type pvpSide struct {
	name  string
	level int8
	pvpOn bool
}

func newAttackPvPFixture(t *testing.T, nopvp bool, alice, bob pvpSide) attackPvPFixture {
	t.Helper()
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{
		ID:    1,
		Name:  "Test Room",
		Flags: repo.RoomFlags{NoPVP: nopvp},
	})
	mobs := repo.NewMemoryMobInstanceRepo()
	chars := repo.NewMemoryCharacterRepo()
	for _, side := range []pvpSide{alice, bob} {
		ch := repo.Character{
			Name:          side.name,
			CurrentRoomID: 1,
			ClassLevels:   map[creature.Class]int8{creature.ClassArmsman: side.level},
			PvP:           side.pvpOn,
			Core: creature.Core{
				HPCurrent: 30, HPMax: 30, Defense: 12, BAB: 1,
			},
		}
		if _, err := chars.Create(context.Background(), ch); err != nil {
			t.Fatalf("seed %s: %v", side.name, err)
		}
	}
	mgr := combat.New(eventbus.New(), chars, mobs, repo.NewMemoryMobTemplateRepo(), repo.NewMemoryItemRepo())
	return attackPvPFixture{mgr: mgr, rooms: rooms, mobs: mobs, characters: chars}
}

func runAttackOnPlayer(t *testing.T, fx attackPvPFixture, target string) (aOut, bOut string, fightStarted bool) {
	t.Helper()
	sessions, alice, _, aOutBuf, bOutBuf := commPair(t)
	c := NewAttack(fx.mgr, fx.rooms, fx.mobs, fx.characters, sessions)
	runCmd(t, c, alice, target)
	return aOutBuf.String(), bOutBuf.String(), fx.mgr.Active(alice.CurrentRoomID)
}

func TestAttackPvP_BothOptedInLevelOK_StartsFight(t *testing.T) {
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 10, true},
		pvpSide{"Bob", 10, true})
	a, b, fight := runAttackOnPlayer(t, fx, "bob")
	if !fight {
		t.Fatalf("expected fight to start; alice=%q", a)
	}
	if !strings.Contains(a, "ready an attack against Bob") {
		t.Fatalf("missing self echo: %q", a)
	}
	if !strings.Contains(b, "Alice moves to attack Bob") {
		t.Fatalf("missing peer broadcast: %q", b)
	}
	got, ok := fx.mgr.PendingAction(1, ActorRefForCharacter(1))
	if !ok {
		t.Fatal("queued action missing")
	}
	if got.Target.Kind != combat.ActorKindCharacter || got.Target.ID != 2 {
		t.Fatalf("target = %+v, want char ID=2", got.Target)
	}
}

func TestAttackPvP_NoPVPRoom_Refuses(t *testing.T) {
	fx := newAttackPvPFixture(t, true,
		pvpSide{"Alice", 10, true},
		pvpSide{"Bob", 10, true})
	a, _, fight := runAttackOnPlayer(t, fx, "bob")
	if fight {
		t.Fatal("nopvp room must not start a fight")
	}
	if !strings.Contains(a, "sanctified") {
		t.Fatalf("missing nopvp refusal: %q", a)
	}
}

func TestAttackPvP_AttackerNewbie_Refuses(t *testing.T) {
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 5, true},
		pvpSide{"Bob", 10, true})
	a, _, fight := runAttackOnPlayer(t, fx, "bob")
	if fight {
		t.Fatal("newbie attacker must not start a fight")
	}
	if !strings.Contains(a, "too green for the killing fields") {
		t.Fatalf("missing attacker-newbie refusal: %q", a)
	}
}

func TestAttackPvP_TargetNewbie_Refuses(t *testing.T) {
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 10, true},
		pvpSide{"Bob", 5, true})
	a, _, fight := runAttackOnPlayer(t, fx, "bob")
	if fight {
		t.Fatal("newbie target must not start a fight")
	}
	if !strings.Contains(a, "Bob is too green") {
		t.Fatalf("missing target-newbie refusal: %q", a)
	}
}

func TestAttackPvP_AttackerOptedOut_Refuses(t *testing.T) {
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 10, false},
		pvpSide{"Bob", 10, true})
	a, _, fight := runAttackOnPlayer(t, fx, "bob")
	if fight {
		t.Fatal("attacker not opted in must not start a fight")
	}
	if !strings.Contains(a, "haven't enabled PvP") {
		t.Fatalf("missing attacker-opt refusal: %q", a)
	}
}

func TestAttackPvP_TargetOptedOut_Refuses(t *testing.T) {
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 10, true},
		pvpSide{"Bob", 10, false})
	a, _, fight := runAttackOnPlayer(t, fx, "bob")
	if fight {
		t.Fatal("target not opted in must not start a fight")
	}
	if !strings.Contains(a, "Bob hasn't enabled PvP") {
		t.Fatalf("missing target-opt refusal: %q", a)
	}
}

func TestAttackPvP_RoomLookupFailureFailsClosed(t *testing.T) {
	// Drop the room row entirely so rooms.FindByID errors on the
	// player-target branch. The PvP guard must refuse (fail closed)
	// rather than silently bypass NoPVP enforcement.
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 10, true},
		pvpSide{"Bob", 10, true})
	fx.rooms = repo.NewMemoryRoomRepo() // empty
	a, _, fight := runAttackOnPlayer(t, fx, "bob")
	if fight {
		t.Fatal("missing room must not start a PvP fight")
	}
	if !strings.Contains(a, "sanctified") {
		t.Fatalf("expected fail-closed sanctified refusal: %q", a)
	}
}

func TestCharacterLevel_SumsClassLevels(t *testing.T) {
	ch := repo.Character{
		ClassLevels: map[creature.Class]int8{
			creature.ClassArmsman:  3,
			creature.ClassWoodsman: 2,
		},
	}
	if got := characterLevel(ch); got != 5 {
		t.Fatalf("characterLevel = %d, want 5", got)
	}
	if got := characterLevel(repo.Character{}); got != 0 {
		t.Fatalf("empty characterLevel = %d, want 0", got)
	}
}
