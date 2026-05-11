package combat

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// throwFixture seeds a Manager with one character + one mob in room 1
// and a thrown-range weapon item in the character's wield slot. The
// character has high BAB and Str so any non-1 roll lands against the
// mob's Defense=0 fixture, keeping the test stable across RNG drift.
func throwFixture(t *testing.T) (*Manager, *repo.MemoryCharacterRepo, *repo.MemoryItemRepo,
	int64, ActorRef, ActorRef) {
	t.Helper()
	ctx := context.Background()
	bus := eventbus.New()
	chars := repo.NewMemoryCharacterRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	templates := repo.NewMemoryMobTemplateRepo()
	items := repo.NewMemoryItemRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "owner", PasswordHash: "h"})

	knife, err := items.Create(ctx, repo.Item{
		ExternalID: "test-knife",
		Name:       "a throwing knife",
		Type:       repo.ItemTypeWeapon,
		Weight:     0.5,
		Stats: &repo.WeaponStats{
			Proficiency: "simple", Size: "small", Range: "thrown",
			Damage: "1d4", CritMult: 2,
		},
	})
	if err != nil {
		t.Fatalf("seed knife: %v", err)
	}

	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Alice", CurrentRoomID: 1,
		Core: creature.Core{
			HPCurrent: 50, HPMax: 50, Defense: 10, BAB: 50,
			Abilities: creature.Abilities{
				Str: creature.AbilityScore{Current: 18},
			},
			CurrentRoomID: 1,
		},
	})
	if err != nil {
		t.Fatalf("seed char: %v", err)
	}
	// Wield the knife.
	ch.Equipment.Set(creature.SlotPrimaryWield, knife.ID)
	if err := chars.RecordEquipment(ctx, ch.ID, ch.Equipment); err != nil {
		t.Fatalf("wield: %v", err)
	}
	if err := items.SetOwner(ctx, knife.ID, ch.ID); err != nil {
		t.Fatalf("owner: %v", err)
	}

	tmpl, _ := templates.Create(ctx, creature.MobTemplate{
		ExternalID: "trolloc-grunt", ChallengeCode: 'A',
	})
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core: creature.Core{
			Name: "trolloc", HPCurrent: 100, HPMax: 100,
			Defense: 0, BAB: 0, CurrentRoomID: 1,
		},
	})
	if err := mobs.UpdateRoom(ctx, mob.ID, 1); err != nil {
		t.Fatalf("place mob: %v", err)
	}

	mgr := New(bus, chars, mobs, templates, items)
	mgr.SetClock(func() time.Time { return time.Unix(0, 0).UTC() })

	parts := []ActorRef{
		{Kind: ActorKindCharacter, ID: ch.ID},
		{Kind: ActorKindMob, ID: mob.ID},
	}
	if _, err := mgr.Start(ctx, 1, parts); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// We want the character to act first regardless of dice. Pin the
	// order head to the character ref by directly mutating the slice.
	mgr.mu.Lock()
	if fight, ok := mgr.fights[1]; ok && len(fight.Order) == 2 {
		if fight.Order[0].Ref != parts[0] {
			fight.Order[0], fight.Order[1] = fight.Order[1], fight.Order[0]
		}
		// Make sure NextActAt allows the head to act on next Tick.
		fight.Order[0].NextActAt = time.Unix(0, 0).UTC()
	}
	mgr.mu.Unlock()

	return mgr, chars, items, knife.ID, parts[0], parts[1]
}

// TestResolveThrow_ClearsWieldAndDropsItem asserts the thrown weapon
// is cleared from SlotPrimaryWield and either lands in the room
// (miss / hit-but-alive) or in a corpse (kill).
func TestResolveThrow_ClearsWieldAndDropsItem(t *testing.T) {
	ctx := context.Background()
	mgr, chars, items, knifeID, attacker, defender := throwFixture(t)

	var throwEv CombatThrow
	var thrMu sync.Mutex
	gotThrow := false
	eventbus.Subscribe[CombatThrow](mgr.bus, func(_ context.Context, ev CombatThrow) {
		thrMu.Lock()
		throwEv = ev
		gotThrow = true
		thrMu.Unlock()
	})

	if err := mgr.EnqueueAction(1, attacker, Action{
		Kind: ActionThrow, Target: defender, WeaponID: knifeID,
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	mgr.Tick(ctx)

	thrMu.Lock()
	defer thrMu.Unlock()
	if !gotThrow {
		t.Fatal("CombatThrow not published")
	}
	if throwEv.ItemID != knifeID {
		t.Fatalf("CombatThrow.ItemID = %d, want %d", throwEv.ItemID, knifeID)
	}

	// Equipment slot must be empty.
	ch, _ := chars.GetByID(ctx, attacker.ID)
	if id := ch.Equipment.Get(creature.SlotPrimaryWield); id != 0 {
		t.Fatalf("SlotPrimaryWield = %d after throw, want 0", id)
	}

	// Item must have left the character's inventory. With Defense=0
	// the throw lands and either (a) the mob still lives so the
	// item is on the room floor, or (b) the mob died and the item
	// parented into the corpse.
	knife, err := items.GetByID(ctx, knifeID)
	if err != nil {
		t.Fatalf("knife lookup: %v", err)
	}
	if knife.OwnerCharacterID != 0 {
		t.Fatalf("knife still owned by char (%d) after throw",
			knife.OwnerCharacterID)
	}
	if knife.RoomID == 0 && knife.ParentItemID == 0 {
		t.Fatal("knife landed in the void — must be in room or parent container")
	}
}

// TestResolveThrow_RefusesNoWeapon asserts the resolver short-circuits
// silently when WeaponID is 0 (no weapon equipped). The action is
// popped but no CombatThrow / CombatHit fire.
func TestResolveThrow_RefusesNoWeapon(t *testing.T) {
	mgr, _, _, _, attacker, defender := throwFixture(t)

	thrown := false
	eventbus.Subscribe[CombatThrow](mgr.bus, func(_ context.Context, _ CombatThrow) {
		thrown = true
	})

	if err := mgr.EnqueueAction(1, attacker, Action{
		Kind: ActionThrow, Target: defender, WeaponID: 0,
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	mgr.Tick(context.Background())

	if thrown {
		t.Fatal("CombatThrow fired with WeaponID=0")
	}
}
