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

// iterativeFixture seeds a character/mob pair in room 1, pins the
// character at the head of the order, and parks the clock at t=0.
// bab/sta let the caller dial in the iterative tier and stamina
// budget under test.
func iterativeFixture(t *testing.T, bab int16, sta int32) (
	*Manager, *repo.MemoryCharacterRepo, *repo.MemoryMobInstanceRepo,
	*eventbus.Bus, ActorRef, ActorRef,
) {
	t.Helper()
	ctx := context.Background()
	bus := eventbus.New()
	chars := repo.NewMemoryCharacterRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	templates := repo.NewMemoryMobTemplateRepo()
	items := repo.NewMemoryItemRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "owner", PasswordHash: "h"})

	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Bashere", CurrentRoomID: 1,
		Core: creature.Core{
			HPCurrent: 50, HPMax: 50, Defense: 0, BAB: bab,
			Abilities:      creature.Abilities{Str: creature.AbilityScore{Current: 18}},
			StaminaCurrent: sta, StaminaMax: 100,
			CurrentRoomID: 1,
		},
	})
	if err != nil {
		t.Fatalf("seed char: %v", err)
	}

	tmpl, _ := templates.Create(ctx, creature.MobTemplate{
		ExternalID: "training-dummy", ChallengeCode: 'A',
	})
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core: creature.Core{
			Name: "training dummy", HPCurrent: 9999, HPMax: 9999,
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
	// Pin character at the head so we can predict which Ref swings.
	mgr.mu.Lock()
	if fight, ok := mgr.fights[1]; ok && len(fight.Order) == 2 {
		if fight.Order[0].Ref != parts[0] {
			fight.Order[0], fight.Order[1] = fight.Order[1], fight.Order[0]
		}
		fight.Order[0].NextActAt = time.Unix(0, 0).UTC()
	}
	mgr.mu.Unlock()

	return mgr, chars, mobs, bus, parts[0], parts[1]
}

// TestStart_StampsIterativeBonuses verifies the Phase L #66 wiring at
// fight open: ActorEntry.IterativeBonuses is derived from creature.BAB
// via IterativeBonusesFor.
func TestStart_StampsIterativeBonuses(t *testing.T) {
	mgr, _, _, _, attacker, _ := iterativeFixture(t, 11, 100)

	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	f := mgr.fights[1]
	var got ActorEntry
	for _, e := range f.Order {
		if e.Ref == attacker {
			got = e
			break
		}
	}
	want := []int16{0, -5, -10}
	if got.PendingSwings != len(want) {
		t.Errorf("PendingSwings = %d, want %d", got.PendingSwings, len(want))
	}
	if len(got.Swings) != len(want) {
		t.Fatalf("Swings = %v, want %v", got.Swings, want)
	}
	for i, sp := range got.Swings {
		if sp.Bonus != want[i] {
			t.Errorf("Swings[%d].Bonus = %d, want %d", i, sp.Bonus, want[i])
		}
		if sp.Slot != creature.SlotPrimaryWield {
			t.Errorf("Swings[%d].Slot = %v, want SlotPrimaryWield", i, sp.Slot)
		}
	}
}

// TestTick_IterativeChainAllThreeSwings: a BAB-11 attacker with full
// stamina queues a single Attack. One Tick must produce three
// resolutions (ActionResolved events) attributed to that attacker in
// the same pulse, and NextActAt must accumulate to 3× the per-swing
// cost.
func TestTick_IterativeChainAllThreeSwings(t *testing.T) {
	ctx := context.Background()
	mgr, _, _, bus, attacker, defender := iterativeFixture(t, 11, 100)

	var (
		mu      sync.Mutex
		resolves []ActionResolved
	)
	eventbus.Subscribe[ActionResolved](bus, func(_ context.Context, ev ActionResolved) {
		mu.Lock()
		resolves = append(resolves, ev)
		mu.Unlock()
	})

	if err := mgr.EnqueueAction(1, attacker, Action{Kind: ActionAttack, Target: defender}); err != nil {
		t.Fatalf("EnqueueAction: %v", err)
	}

	mgr.Tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	// Count resolutions attributed to the attacker on this pulse.
	atkSwings := 0
	for _, ev := range resolves {
		if ev.Actor == attacker && ev.Kind == ActionAttack {
			atkSwings++
		}
	}
	if atkSwings != 3 {
		t.Fatalf("attacker resolutions on one pulse = %d, want 3 (BAB 11 iteratives); events: %+v", atkSwings, resolves)
	}

	// NextActAt should be 3× the actor's per-swing cost.
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	f := mgr.fights[1]
	var entry ActorEntry
	for _, e := range f.Order {
		if e.Ref == attacker {
			entry = e
			break
		}
	}
	base := mgr.actorActionCost(ctx, attacker, Action{Kind: ActionAttack, Target: defender}, creature.SlotPrimaryWield)
	want := 3 * base
	got := entry.NextActAt.Sub(time.Unix(0, 0).UTC())
	if got != want {
		t.Errorf("NextActAt offset = %v, want %v (3 × %v)", got, want, base)
	}
}

// TestTick_IterativeStaminaTruncates: BAB-11 attacker with enough
// stamina for only the first swing (cost 5 SP each) — the loop must
// resolve exactly one Attack and stop.
//
// The first swing always fires (matching the EnqueueAction gate
// pattern — staminaGate already approved it). The follow-up swing's
// hasStaminaFor pre-check sees 0 SP and breaks the chain.
func TestTick_IterativeStaminaTruncates(t *testing.T) {
	ctx := context.Background()
	mgr, _, _, bus, attacker, defender := iterativeFixture(t, 11, 5)

	var (
		mu       sync.Mutex
		resolves []ActionResolved
	)
	eventbus.Subscribe[ActionResolved](bus, func(_ context.Context, ev ActionResolved) {
		mu.Lock()
		resolves = append(resolves, ev)
		mu.Unlock()
	})

	if err := mgr.EnqueueAction(1, attacker, Action{Kind: ActionAttack, Target: defender}); err != nil {
		t.Fatalf("EnqueueAction: %v", err)
	}
	mgr.Tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	atkSwings := 0
	for _, ev := range resolves {
		if ev.Actor == attacker && ev.Kind == ActionAttack {
			atkSwings++
		}
	}
	if atkSwings != 1 {
		t.Fatalf("attacker resolutions on one pulse = %d, want 1 (stamina dry after first swing); events: %+v", atkSwings, resolves)
	}
}

// TestTick_NonAttackKeepsSingleResolution: queue a Parry from a
// BAB-16 character — the Parry branch is single-resolution even on a
// 4-swing iterative tier.
func TestTick_NonAttackKeepsSingleResolution(t *testing.T) {
	ctx := context.Background()
	mgr, _, _, bus, attacker, _ := iterativeFixture(t, 16, 100)

	var (
		mu       sync.Mutex
		resolves []ActionResolved
	)
	eventbus.Subscribe[ActionResolved](bus, func(_ context.Context, ev ActionResolved) {
		mu.Lock()
		resolves = append(resolves, ev)
		mu.Unlock()
	})

	if err := mgr.EnqueueAction(1, attacker, Action{Kind: ActionParry}); err != nil {
		t.Fatalf("EnqueueAction: %v", err)
	}
	mgr.Tick(ctx)

	mu.Lock()
	defer mu.Unlock()
	parries := 0
	for _, ev := range resolves {
		if ev.Actor == attacker && ev.Kind == ActionParry {
			parries++
		}
	}
	if parries != 1 {
		t.Fatalf("attacker parry resolutions = %d, want 1 (non-attack stays single-swing)", parries)
	}
}
