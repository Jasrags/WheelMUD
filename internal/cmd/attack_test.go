package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
)

// attackFixture wires the repos + manager + a room with one mob and
// an Alice character session, ready for an `attack` invocation.
type attackFixture struct {
	mgr        *combat.Manager
	rooms      *repo.MemoryRoomRepo
	mobs       *repo.MemoryMobInstanceRepo
	characters *repo.MemoryCharacterRepo
	sessions   *session.Registry
	mob        creature.MobInstance
}

func newAttackFixture(t *testing.T, peaceful bool) attackFixture {
	t.Helper()
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{
		ID:    1,
		Name:  "Test Room",
		Flags: repo.RoomFlags{Peaceful: peaceful},
	})
	mobs := repo.NewMemoryMobInstanceRepo()
	mob, err := mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: 1,
		Core: creature.Core{
			Name:          "trolloc",
			HPCurrent:     30,
			HPMax:         30,
			Defense:       12,
			CurrentRoomID: 1,
		},
	})
	if err != nil {
		t.Fatalf("seed mob: %v", err)
	}
	if err := mobs.UpdateRoom(context.Background(), mob.ID, 1); err != nil {
		t.Fatalf("set mob room: %v", err)
	}
	chars := repo.NewMemoryCharacterRepo()
	accID := int64(7)
	if _, err := chars.Create(context.Background(), repo.Character{
		AccountID:     accID,
		Name:          "Alice",
		CurrentRoomID: 1,
		Core: creature.Core{
			HPCurrent: 20, HPMax: 20, Defense: 12, BAB: 1,
			Abilities: creature.Abilities{
				Str: creature.AbilityScore{Current: 14},
				Dex: creature.AbilityScore{Current: 12},
			},
		},
	}); err != nil {
		t.Fatalf("seed char: %v", err)
	}
	mgr := combat.New(eventbus.New(), chars, mobs, repo.NewMemoryMobTemplateRepo(), repo.NewMemoryItemRepo())
	return attackFixture{
		mgr: mgr, rooms: rooms, mobs: mobs, characters: chars,
		sessions: session.NewRegistry(), mob: mob,
	}
}

func TestAttack_NoArgsRefuses(t *testing.T) {
	fx := newAttackFixture(t, false)
	_, alice, _, aOut, _ := commPair(t)
	c := NewAttack(fx.mgr, fx.rooms, fx.mobs, fx.characters, fx.sessions)
	runCmd(t, c, alice, "")
	if !strings.Contains(aOut.String(), "Attack what?") {
		t.Fatalf("missing usage refusal: %q", aOut.String())
	}
	if fx.mgr.Active(alice.CurrentRoomID) {
		t.Fatal("no fight should start on empty arg")
	}
}

func TestAttack_TargetNotFound(t *testing.T) {
	fx := newAttackFixture(t, false)
	_, alice, _, aOut, _ := commPair(t)
	c := NewAttack(fx.mgr, fx.rooms, fx.mobs, fx.characters, fx.sessions)
	runCmd(t, c, alice, "ghost")
	if !strings.Contains(aOut.String(), "don't see them here") {
		t.Fatalf("missing not-found refusal: %q", aOut.String())
	}
	if fx.mgr.Active(alice.CurrentRoomID) {
		t.Fatal("no fight should start on bad target")
	}
}

func TestAttack_PeacefulRoomRefuses(t *testing.T) {
	fx := newAttackFixture(t, true)
	_, alice, _, aOut, _ := commPair(t)
	c := NewAttack(fx.mgr, fx.rooms, fx.mobs, fx.characters, fx.sessions)
	runCmd(t, c, alice, "trolloc")
	if !strings.Contains(aOut.String(), "profound peace") {
		t.Fatalf("missing peace refusal: %q", aOut.String())
	}
	if fx.mgr.Active(alice.CurrentRoomID) {
		t.Fatal("peaceful room must not start a fight")
	}
}

func TestAttack_StartsFightAndQueuesAction(t *testing.T) {
	fx := newAttackFixture(t, false)
	// Use commPair's session registry so Bob actually receives the
	// peer broadcast. CharacterID=1 for Alice from commPair lines up
	// with the first character seeded in newAttackFixture (memory
	// repo IDs from 1).
	sessions, alice, _, aOut, bOut := commPair(t)
	c := NewAttack(fx.mgr, fx.rooms, fx.mobs, fx.characters, sessions)
	runCmd(t, c, alice, "trolloc")

	if !strings.Contains(aOut.String(), "ready an attack") {
		t.Fatalf("missing self-echo: %q", aOut.String())
	}
	if !strings.Contains(bOut.String(), "Alice moves to attack") {
		t.Fatalf("missing peer broadcast: %q", bOut.String())
	}
	if !fx.mgr.Active(alice.CurrentRoomID) {
		t.Fatal("fight should have started")
	}
	actor := ActorRefForCharacter(alice.CharacterID)
	got, ok := fx.mgr.PendingAction(alice.CurrentRoomID, actor)
	if !ok {
		t.Fatal("no queued action")
	}
	if got.Kind != combat.ActionAttack {
		t.Fatalf("kind = %v, want ActionAttack", got.Kind)
	}
	if got.Target.Kind != combat.ActorKindMob || got.Target.ID != fx.mob.ID {
		t.Fatalf("target = %+v, want mob %d", got.Target, fx.mob.ID)
	}
}

func TestAttack_ReQueuesWithoutRestartingFight(t *testing.T) {
	fx := newAttackFixture(t, false)
	_, alice, _, _, _ := commPair(t)
	// Spawn a second mob so the player has something to switch to.
	mob2, err := fx.mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: 1,
		Core:       creature.Core{Name: "myrddraal", CurrentRoomID: 1, HPCurrent: 50, HPMax: 50},
	})
	if err != nil {
		t.Fatalf("seed mob2: %v", err)
	}
	if err := fx.mobs.UpdateRoom(context.Background(), mob2.ID, 1); err != nil {
		t.Fatalf("place mob2: %v", err)
	}
	c := NewAttack(fx.mgr, fx.rooms, fx.mobs, fx.characters, fx.sessions)

	runCmd(t, c, alice, "trolloc")
	fight, ok := fx.mgr.Get(alice.CurrentRoomID)
	if !ok {
		t.Fatal("first attack must have started a fight")
	}
	startedAt := fight.StartedAt

	runCmd(t, c, alice, "myrddraal")
	again, _ := fx.mgr.Get(alice.CurrentRoomID)
	if !again.StartedAt.Equal(startedAt) {
		t.Fatalf("StartedAt changed: re-issuing attack restarted the fight")
	}
	got, _ := fx.mgr.PendingAction(alice.CurrentRoomID,
		ActorRefForCharacter(alice.CharacterID))
	if got.Target.ID != mob2.ID {
		t.Fatalf("target not switched: got %+v want mob %d", got.Target, mob2.ID)
	}
}
