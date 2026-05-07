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
	mgr := New(bus, chars, mobs)
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
	// Seed 42 produces a stable d20 sequence; the assertion is on
	// the relative ordering, which a higher Dex / InitMod must
	// dominate over RNG noise.
	mgr, _, _ := seedManager(t, 42)
	parts := []ActorRef{
		{Kind: ActorKindMob, ID: 1}, // Dex 14 (+2), InitMod 2 → +4 total
		{Kind: ActorKindMob, ID: 2}, // Dex 10 (+0), InitMod 0 → +0 total
	}
	f, err := mgr.Start(context.Background(), 1, parts)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(f.Order) != 2 {
		t.Fatalf("Order len = %d, want 2", len(f.Order))
	}
	// Mob 1 has +4 vs mob 2's +0; the gap is 4 so even on a worst-
	// case d20 swing (mob 1 rolls 1 vs mob 2 rolls 20) they tie at
	// 5, broken by Tiebreak. With seed=42 mob 1 rolls higher; assert
	// it's at the head.
	if f.Order[0].Ref.ID != 1 {
		t.Fatalf("expected mob 1 first; got order=%+v", f.Order)
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

func TestTick_AdvancesRoundAndRotatesActor(t *testing.T) {
	mgr, _, bus := seedManager(t, 42)
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

	mgr.Tick(context.Background())
	mgr.Tick(context.Background())
	mgr.Tick(context.Background())

	roundsMu.Lock()
	defer roundsMu.Unlock()
	if len(rounds) != 3 {
		t.Fatalf("got %d RoundStarted events, want 3", len(rounds))
	}
	// Round 1 keeps ActiveIdx=0 (no rotation on the opening round);
	// round 2 advances to second; round 3 wraps back to first.
	wantRoundActor := []ActorRef{first, second, first}
	wantRoundN := []int{1, 2, 3}
	for i, ev := range rounds {
		if ev.Round != wantRoundN[i] {
			t.Errorf("round[%d].Round = %d, want %d", i, ev.Round, wantRoundN[i])
		}
		if ev.Active != wantRoundActor[i] {
			t.Errorf("round[%d].Active = %+v, want %+v", i, ev.Active, wantRoundActor[i])
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
