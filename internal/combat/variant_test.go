package combat

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/eventbus"
)

// TestVariantCadence_PowerSlowsActor verifies a Power swing stamps
// NextActAt 4.5s out rather than the Normal 3s. We bypass action
// resolution by writing PendingAttack with a missing target so the
// resolver short-circuits before touching the rng, then assert the
// schedule advance.
func TestVariantCadence_PowerSlowsActor(t *testing.T) {
	mgr, _, _ := seedManager(t, 1)
	now := time.Unix(0, 0).UTC()
	mgr.SetClock(func() time.Time { return now })

	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	f, err := mgr.Start(context.Background(), 1, parts)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	head := f.Order[0].Ref
	target := f.Order[1].Ref

	if err := mgr.EnqueueAction(1, head, Action{
		Kind:    ActionAttack,
		Variant: VariantPower,
		Target:  target,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	mgr.Tick(context.Background())

	got, _ := mgr.Get(1)
	var stamped time.Time
	for _, e := range got.Order {
		if e.Ref == head {
			stamped = e.NextActAt
			break
		}
	}
	if got, want := stamped, now.Add(4500*time.Millisecond); !got.Equal(want) {
		t.Errorf("power NextActAt = %v, want %v", got, want)
	}
}

// TestVariantCadence_QuickSpeedsActor mirrors the Power test for
// Quick — 1.8s instead of 3.0s.
func TestVariantCadence_QuickSpeedsActor(t *testing.T) {
	mgr, _, _ := seedManager(t, 1)
	now := time.Unix(0, 0).UTC()
	mgr.SetClock(func() time.Time { return now })

	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	f, err := mgr.Start(context.Background(), 1, parts)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	head := f.Order[0].Ref
	target := f.Order[1].Ref

	if err := mgr.EnqueueAction(1, head, Action{
		Kind:    ActionAttack,
		Variant: VariantQuick,
		Target:  target,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	mgr.Tick(context.Background())

	got, _ := mgr.Get(1)
	var stamped time.Time
	for _, e := range got.Order {
		if e.Ref == head {
			stamped = e.NextActAt
			break
		}
	}
	if got, want := stamped, now.Add(1800*time.Millisecond); !got.Equal(want) {
		t.Errorf("quick NextActAt = %v, want %v", got, want)
	}
}

// TestCombatHit_CarriesVariant asserts the published CombatHit and
// CombatMiss events stamp Variant from the queued action so cmd-layer
// echo subscribers can render flavored lines without consulting the
// queue.
func TestCombatHit_CarriesVariant(t *testing.T) {
	mgr, _, bus := seedManager(t, 7)
	now := time.Unix(0, 0).UTC()
	mgr.SetClock(func() time.Time { return now })

	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	f, err := mgr.Start(context.Background(), 1, parts)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	head := f.Order[0].Ref
	target := f.Order[1].Ref

	var (
		mu        sync.Mutex
		hitSeen   []AttackVariant
		missSeen  []AttackVariant
	)
	eventbus.Subscribe[CombatHit](bus, func(_ context.Context, ev CombatHit) {
		mu.Lock()
		hitSeen = append(hitSeen, ev.Variant)
		mu.Unlock()
	})
	eventbus.Subscribe[CombatMiss](bus, func(_ context.Context, ev CombatMiss) {
		mu.Lock()
		missSeen = append(missSeen, ev.Variant)
		mu.Unlock()
	})

	if err := mgr.EnqueueAction(1, head, Action{
		Kind:    ActionAttack,
		Variant: VariantQuick,
		Target:  target,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	mgr.Tick(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(hitSeen)+len(missSeen) == 0 {
		t.Fatal("expected one of CombatHit or CombatMiss to fire")
	}
	for _, v := range hitSeen {
		if v != VariantQuick {
			t.Errorf("CombatHit.Variant = %v, want VariantQuick", v)
		}
	}
	for _, v := range missSeen {
		if v != VariantQuick {
			t.Errorf("CombatMiss.Variant = %v, want VariantQuick", v)
		}
	}
}
