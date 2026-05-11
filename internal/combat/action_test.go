package combat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/eventbus"
)

func TestEnqueueAction_RoundTrip(t *testing.T) {
	mgr, _, _ := seedManager(t, 1)
	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	if _, err := mgr.Start(context.Background(), 1, parts); err != nil {
		t.Fatalf("start: %v", err)
	}
	actor := parts[0]
	target := parts[1]
	a := Action{Kind: ActionAttack, Target: target, WeaponID: 99}
	if err := mgr.EnqueueAction(1, actor, a); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	got, ok := mgr.PendingAction(1, actor)
	if !ok {
		t.Fatal("no pending action")
	}
	if got != a {
		t.Fatalf("got %+v, want %+v", got, a)
	}
}

func TestEnqueueAction_Overwrites(t *testing.T) {
	mgr, _, _ := seedManager(t, 1)
	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	mgr.Start(context.Background(), 1, parts)
	actor := parts[0]
	first := Action{Kind: ActionAttack, Target: parts[1], WeaponID: 1}
	second := Action{Kind: ActionAttack, Target: parts[1], WeaponID: 2}
	_ = mgr.EnqueueAction(1, actor, first)
	_ = mgr.EnqueueAction(1, actor, second)
	got, _ := mgr.PendingAction(1, actor)
	if got != second {
		t.Fatalf("expected overwrite, got %+v", got)
	}
}

func TestEnqueueAction_NoFight(t *testing.T) {
	mgr, _, _ := seedManager(t, 1)
	if err := mgr.EnqueueAction(99, ActorRef{Kind: ActorKindMob, ID: 1},
		Action{Kind: ActionAttack}); !errors.Is(err, ErrFightNotFound) {
		t.Fatalf("got %v, want ErrFightNotFound", err)
	}
}

// TestDefaultActionCost_Variants pins the slice-61 cost table:
// Attack scales by variant factor (1.0 / 1.5 / 0.6); Parry/Flee
// ignore Variant; the zero ActionNone falls through to 1s. Times
// are pinned to the millisecond so a future tuning pass surfaces
// here.
func TestDefaultActionCost_Variants(t *testing.T) {
	cases := []struct {
		kind    ActionKind
		variant AttackVariant
		want    time.Duration
	}{
		{ActionAttack, VariantNormal, 3 * time.Second},
		{ActionAttack, VariantPower, 4500 * time.Millisecond},
		{ActionAttack, VariantQuick, 1800 * time.Millisecond},
		{ActionParry, VariantNormal, 2 * time.Second},
		{ActionParry, VariantPower, 2 * time.Second},  // variant ignored
		{ActionParry, VariantQuick, 2 * time.Second},  // variant ignored
		{ActionFlee, VariantNormal, 2 * time.Second},
		{ActionFlee, VariantPower, 2 * time.Second},   // variant ignored
		{ActionDodge, VariantNormal, 1 * time.Second},
		{ActionThrow, VariantNormal, 2 * time.Second},
		{ActionSidestep, VariantNormal, 500 * time.Millisecond},
		{ActionNone, VariantNormal, 1 * time.Second},
	}
	for _, tc := range cases {
		got := DefaultActionCost(tc.kind, tc.variant)
		if got != tc.want {
			t.Errorf("DefaultActionCost(kind=%v variant=%v) = %v, want %v",
				tc.kind, tc.variant, got, tc.want)
		}
	}
}

// TestEnqueueAction_VariantRoundTrip pins that the new Action.Variant
// field round-trips through EnqueueAction + PendingAction.
func TestEnqueueAction_VariantRoundTrip(t *testing.T) {
	mgr, _, _ := seedManager(t, 1)
	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	if _, err := mgr.Start(context.Background(), 1, parts); err != nil {
		t.Fatalf("start: %v", err)
	}
	for _, v := range []AttackVariant{VariantNormal, VariantPower, VariantQuick} {
		a := Action{Kind: ActionAttack, Variant: v, Target: parts[1]}
		if err := mgr.EnqueueAction(1, parts[0], a); err != nil {
			t.Fatalf("enqueue variant=%v: %v", v, err)
		}
		got, ok := mgr.PendingAction(1, parts[0])
		if !ok {
			t.Fatalf("variant=%v: no pending action", v)
		}
		if got.Variant != v {
			t.Errorf("variant=%v: got %v", v, got.Variant)
		}
	}
}

func TestPopAction_EmptyMap(t *testing.T) {
	f := &Fight{}
	if _, ok := f.popAction(ActorRef{Kind: ActorKindMob, ID: 1}); ok {
		t.Fatal("popAction on nil map should return ok=false")
	}
}

func TestTick_ResolvesQueuedAttack(t *testing.T) {
	mgr, mobs, bus := seedManager(t, 1)
	// Mob 1 attacks Mob 2. Mob 2 has Defense=0 so almost any roll
	// hits. Confirm: an action queued, an ActionResolved fires, and
	// HP drops on the target.
	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	if _, err := mgr.Start(context.Background(), 1, parts); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Mob 1 might not be the head of order — pre-Tick, Round=0 and the
	// first Tick advances to Round=1 with the head's turn. Queue an
	// attack from whichever ref is at index 0 so the very first Tick
	// resolves it.
	fight, _ := mgr.Get(1)
	head := fight.Order[0].Ref
	var defender ActorRef
	for _, e := range fight.Order {
		if e.Ref != head {
			defender = e.Ref
			break
		}
	}
	if err := mgr.EnqueueAction(1, head, Action{Kind: ActionAttack, Target: defender}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	var resolved bool
	eventbus.Subscribe[ActionResolved](bus, func(_ context.Context, ev ActionResolved) {
		if ev.Actor == head {
			resolved = true
		}
	})
	_ = resolved // silence linters; eventbus delivery is async and not asserted here

	preMob, _ := mobs.GetByID(context.Background(), defender.ID)
	preHP := preMob.Core.HPCurrent

	mgr.Tick(context.Background())

	postMob, _ := mobs.GetByID(context.Background(), defender.ID)
	// Mob 2 has Defense=0, attacker has 0 BAB but nat-1 still misses
	// — so HP either stayed put (nat-1) or dropped. Allow both, but
	// require the action was popped from the queue.
	if _, ok := mgr.PendingAction(1, head); ok {
		t.Fatal("queued action should have been popped after Tick")
	}
	if postMob.Core.HPCurrent > preHP {
		t.Fatalf("HP rose from %d to %d", preHP, postMob.Core.HPCurrent)
	}
}
