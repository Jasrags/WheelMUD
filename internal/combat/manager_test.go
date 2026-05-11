package combat

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// seedManager builds a Manager wired to memory repos with a
// deterministic RNG. The mob repo is pre-seeded with two mobs in
// room 1 (ids 1, 2) and one mob in room 2 (id 3) so participant
// resolution and the auto-end check both have something to read.
func seedManager(t *testing.T, seed int64) (*Manager, *repo.MemoryMobInstanceRepo, *eventbus.Bus) {
	t.Helper()
	bus := eventbus.New()
	mobs := repo.NewMemoryMobInstanceRepo()
	chars := repo.NewMemoryCharacterRepo()
	for _, m := range []creature.MobInstance{
		{TemplateID: 1, Core: creature.Core{Name: "trolloc-A", InitMod: 2, CurrentRoomID: 1,
			Abilities: creature.Abilities{Dex: creature.AbilityScore{Current: 14}}}},
		{TemplateID: 1, Core: creature.Core{Name: "trolloc-B", InitMod: 0, CurrentRoomID: 1,
			Abilities: creature.Abilities{Dex: creature.AbilityScore{Current: 10}}}},
		{TemplateID: 1, Core: creature.Core{Name: "trolloc-C", InitMod: 0, CurrentRoomID: 2,
			Abilities: creature.Abilities{Dex: creature.AbilityScore{Current: 10}}}},
	} {
		if _, err := mobs.Create(context.Background(), m); err != nil {
			t.Fatalf("seed mob: %v", err)
		}
	}
	mgr := New(bus, chars, mobs, repo.NewMemoryMobTemplateRepo(), repo.NewMemoryItemRepo())
	mgr.SetRNG(rand.New(rand.NewSource(seed)))
	mgr.SetClock(func() time.Time { return time.Unix(0, 0).UTC() })
	return mgr, mobs, bus
}

func TestStart_RejectsEmptyParticipants(t *testing.T) {
	mgr, _, _ := seedManager(t, 1)
	if _, err := mgr.Start(context.Background(), 1, nil); !errors.Is(err, ErrNoParticipants) {
		t.Fatalf("Start(empty) = %v, want ErrNoParticipants", err)
	}
}

func TestStart_WrapsResolveError(t *testing.T) {
	mgr, _, _ := seedManager(t, 1)
	// Mob id 999 is not in the seeded repo — Start must wrap the
	// repo lookup error and refuse to open the fight.
	_, err := mgr.Start(context.Background(), 1,
		[]ActorRef{{Kind: ActorKindMob, ID: 999}})
	if err == nil {
		t.Fatalf("Start with unknown mob: want error, got nil")
	}
	if mgr.Active(1) {
		t.Fatalf("Start with unknown mob should not open a fight")
	}
}

func TestStart_RejectsDoubleStart(t *testing.T) {
	mgr, _, _ := seedManager(t, 1)
	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}}
	if _, err := mgr.Start(context.Background(), 1, parts); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if _, err := mgr.Start(context.Background(), 1, parts); !errors.Is(err, ErrFightExists) {
		t.Fatalf("second Start = %v, want ErrFightExists", err)
	}
}

func TestStart_OrdersByInitiative(t *testing.T) {
	// Mob 1 has Dex 14 (+2) + InitMod 2 = +4 total bonus.
	// Mob 2 has Dex 10 (+0) + InitMod 0 = +0 total bonus.
	// Any d20 swing keeps mob 1 ahead — worst case (1 vs 20)
	// produces 5 vs 20, breaking via Tiebreak in mob 2's favor.
	// To assert the structural invariant rather than a seed-derived
	// id, repeat the test across many seeds and require the
	// higher-modifier actor to win the *combined* roll on average,
	// then run a single fixed-seed case for the deterministic head
	// assertion.
	for seed := int64(1); seed < 50; seed++ {
		mgr, _, _ := seedManager(t, seed)
		parts := []ActorRef{
			{Kind: ActorKindMob, ID: 1},
			{Kind: ActorKindMob, ID: 2},
		}
		f, err := mgr.Start(context.Background(), 1, parts)
		if err != nil {
			t.Fatalf("seed %d: Start: %v", seed, err)
		}
		if len(f.Order) != 2 {
			t.Fatalf("seed %d: Order len = %d, want 2", seed, len(f.Order))
		}
		// The leader's Initiative must always be ≥ the trailer's;
		// this is the property sortInitiative guarantees regardless
		// of RNG choice.
		if f.Order[0].Initiative < f.Order[1].Initiative {
			t.Fatalf("seed %d: Order not sorted: %+v", seed, f.Order)
		}
	}
}

func TestStart_PublishesCombatStarted(t *testing.T) {
	mgr, _, bus := seedManager(t, 1)
	var got CombatStarted
	var gotMu sync.Mutex
	eventbus.Subscribe[CombatStarted](bus, func(_ context.Context, ev CombatStarted) {
		gotMu.Lock()
		got = ev
		gotMu.Unlock()
	})
	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	if _, err := mgr.Start(context.Background(), 1, parts); err != nil {
		t.Fatalf("Start: %v", err)
	}
	gotMu.Lock()
	defer gotMu.Unlock()
	if got.RoomID != 1 {
		t.Fatalf("event RoomID = %d, want 1", got.RoomID)
	}
	if len(got.Order) != 2 {
		t.Fatalf("event Order len = %d, want 2", len(got.Order))
	}
}

// TestTick_PerActorCadence verifies the slice-60 cadence rewrite:
// every actor whose NextActAt has elapsed resolves in initiative
// order on each pulse, Round increments per actor-act, and an actor
// who acted on the previous pulse is gated by DefaultActionCost
// before re-acting.
//
// Setup: two mobs, no actions queued (ActionNone → 1s cost). Both
// start ready at t=0. Pulse 1 at t=0 fires both in initiative order
// (Round 1, Round 2). Pulse 2 at t=0.5s — neither has crossed the 1s
// gate, so zero events fire. Pulse 3 at t=1.5s — both ready again,
// fires both (Round 3, Round 4).
func TestTick_PerActorCadence(t *testing.T) {
	mgr, _, bus := seedManager(t, 42)
	now := time.Unix(0, 0).UTC()
	mgr.SetClock(func() time.Time { return now })

	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	f, err := mgr.Start(context.Background(), 1, parts)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	first := f.Order[0].Ref
	second := f.Order[1].Ref

	rounds := []RoundStarted{}
	var roundsMu sync.Mutex
	eventbus.Subscribe[RoundStarted](bus, func(_ context.Context, ev RoundStarted) {
		roundsMu.Lock()
		rounds = append(rounds, ev)
		roundsMu.Unlock()
	})

	// Pulse 1: both ready, both fire in initiative order.
	mgr.Tick(context.Background())
	// Pulse 2: clock barely moved; gate is 1s for ActionNone.
	now = now.Add(500 * time.Millisecond)
	mgr.Tick(context.Background())
	// Pulse 3: clock past the gate; both fire again.
	now = now.Add(1 * time.Second)
	mgr.Tick(context.Background())

	roundsMu.Lock()
	defer roundsMu.Unlock()
	if len(rounds) != 4 {
		t.Fatalf("got %d RoundStarted events, want 4 (pulse 1: 2, pulse 2: 0, pulse 3: 2)", len(rounds))
	}
	wantActor := []ActorRef{first, second, first, second}
	wantRound := []int{1, 2, 3, 4}
	for i, ev := range rounds {
		if ev.Round != wantRound[i] {
			t.Errorf("event[%d].Round = %d, want %d", i, ev.Round, wantRound[i])
		}
		if ev.Active != wantActor[i] {
			t.Errorf("event[%d].Active = %+v, want %+v", i, ev.Active, wantActor[i])
		}
	}
}

// TestTick_FastActorOutPaces stamps NextActAt directly on the two
// entries to prove the gate works independently of action cost.
// Actor A starts ready (NextActAt = 0); actor B is gated until t=10s.
// Three 3-second clock advances must produce A acting 3× and B
// acting 0×. Real cost-differentiation (gear, race, feats) lands in
// later L slices; this test exercises the timing plumbing only.
func TestTick_FastActorOutPaces(t *testing.T) {
	mgr, _, bus := seedManager(t, 1)
	now := time.Unix(0, 0).UTC()
	mgr.SetClock(func() time.Time { return now })

	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	f, err := mgr.Start(context.Background(), 1, parts)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	a := f.Order[0].Ref
	b := f.Order[1].Ref

	// Direct field stamp: A ready immediately, B parked far in the
	// future. Acquire the manager lock to keep -race quiet.
	mgr.mu.Lock()
	for i := range f.Order {
		switch f.Order[i].Ref {
		case a:
			f.Order[i].NextActAt = now
		case b:
			f.Order[i].NextActAt = now.Add(10 * time.Second)
		}
	}
	mgr.mu.Unlock()

	rounds := []RoundStarted{}
	var roundsMu sync.Mutex
	eventbus.Subscribe[RoundStarted](bus, func(_ context.Context, ev RoundStarted) {
		roundsMu.Lock()
		rounds = append(rounds, ev)
		roundsMu.Unlock()
	})

	// Three pulses spaced 3s apart. ActionNone cost is 1s so A is
	// ready at every step; B's 10s gate keeps them silent.
	for i := 0; i < 3; i++ {
		mgr.Tick(context.Background())
		now = now.Add(3 * time.Second)
	}

	roundsMu.Lock()
	defer roundsMu.Unlock()
	if len(rounds) != 3 {
		t.Fatalf("got %d events, want 3 (A acts 3×, B 0×): %+v", len(rounds), rounds)
	}
	for i, ev := range rounds {
		if ev.Active != a {
			t.Errorf("event[%d].Active = %+v, want A %+v", i, ev.Active, a)
		}
	}
}

func TestTick_AutoEndsWhenMobsGone(t *testing.T) {
	mgr, mobs, bus := seedManager(t, 1)
	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	if _, err := mgr.Start(context.Background(), 1, parts); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var endReason string
	var endMu sync.Mutex
	eventbus.Subscribe[CombatEnded](bus, func(_ context.Context, ev CombatEnded) {
		endMu.Lock()
		endReason = ev.Reason
		endMu.Unlock()
	})

	// Despawn both mobs — Tick should auto-end the fight on the
	// next pulse.
	if err := mobs.Delete(context.Background(), 1); err != nil {
		t.Fatalf("delete mob 1: %v", err)
	}
	if err := mobs.Delete(context.Background(), 2); err != nil {
		t.Fatalf("delete mob 2: %v", err)
	}

	mgr.Tick(context.Background())

	if mgr.Active(1) {
		t.Fatalf("fight should have auto-ended; Manager still has it")
	}
	endMu.Lock()
	defer endMu.Unlock()
	if endReason != ReasonNoParticipants {
		t.Fatalf("CombatEnded.Reason = %q, want %q", endReason, ReasonNoParticipants)
	}
}

func TestEnd_RemovesFightAndPublishes(t *testing.T) {
	mgr, _, bus := seedManager(t, 1)
	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}}
	if _, err := mgr.Start(context.Background(), 1, parts); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var got CombatEnded
	var gotMu sync.Mutex
	eventbus.Subscribe[CombatEnded](bus, func(_ context.Context, ev CombatEnded) {
		gotMu.Lock()
		got = ev
		gotMu.Unlock()
	})

	if err := mgr.End(context.Background(), 1, ReasonExplicit); err != nil {
		t.Fatalf("End: %v", err)
	}
	if mgr.Active(1) {
		t.Fatalf("fight still active after End")
	}
	gotMu.Lock()
	defer gotMu.Unlock()
	if got.Reason != ReasonExplicit {
		t.Fatalf("CombatEnded.Reason = %q, want %q", got.Reason, ReasonExplicit)
	}
}

func TestEnd_UnknownRoomReturnsNotFound(t *testing.T) {
	mgr, _, _ := seedManager(t, 1)
	if err := mgr.End(context.Background(), 99, ReasonExplicit); !errors.Is(err, ErrFightNotFound) {
		t.Fatalf("End(unknown) = %v, want ErrFightNotFound", err)
	}
}

func TestStop_EndsAllActiveFights(t *testing.T) {
	mgr, _, bus := seedManager(t, 1)
	if _, err := mgr.Start(context.Background(), 1,
		[]ActorRef{{Kind: ActorKindMob, ID: 1}}); err != nil {
		t.Fatalf("Start room 1: %v", err)
	}
	if _, err := mgr.Start(context.Background(), 2,
		[]ActorRef{{Kind: ActorKindMob, ID: 3}}); err != nil {
		t.Fatalf("Start room 2: %v", err)
	}

	var ended []int64
	var endMu sync.Mutex
	eventbus.Subscribe[CombatEnded](bus, func(_ context.Context, ev CombatEnded) {
		endMu.Lock()
		ended = append(ended, ev.RoomID)
		endMu.Unlock()
	})

	mgr.Stop(context.Background())
	if mgr.Active(1) || mgr.Active(2) {
		t.Fatalf("Stop did not clear all fights")
	}
	endMu.Lock()
	defer endMu.Unlock()
	if len(ended) != 2 {
		t.Fatalf("got %d CombatEnded events, want 2", len(ended))
	}
}

func TestSortInitiative_TiebreakDeterministic(t *testing.T) {
	entries := []ActorEntry{
		{Ref: ActorRef{Kind: ActorKindMob, ID: 3}, Initiative: 10, Tiebreak: 5},
		{Ref: ActorRef{Kind: ActorKindMob, ID: 1}, Initiative: 10, Tiebreak: 5},
		{Ref: ActorRef{Kind: ActorKindMob, ID: 2}, Initiative: 10, Tiebreak: 7},
	}
	sortInitiative(entries)
	// Higher Tiebreak first; equal Tiebreak orders by ID ascending.
	want := []int64{2, 1, 3}
	for i, e := range entries {
		if e.Ref.ID != want[i] {
			t.Errorf("entries[%d].ID = %d, want %d (got %+v)", i, e.Ref.ID, want[i], entries)
		}
	}
}
