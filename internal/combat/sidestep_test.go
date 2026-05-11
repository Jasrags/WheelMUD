package combat

import (
	"context"
	"sync"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/eventbus"
)

// TestResolveSidestep_FlatFootsTarget verifies that a queued
// ActionSidestep stamps FlatFootedUntil for the named target (the
// attacker the defender wants to expose), and publishes a
// CombatStance{Kind:"sidestep", Target:<attacker>} event.
func TestResolveSidestep_FlatFootsTarget(t *testing.T) {
	mgr, _, bus := seedManager(t, 1)
	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	if _, err := mgr.Start(context.Background(), 1, parts); err != nil {
		t.Fatalf("start: %v", err)
	}
	fight, _ := mgr.Get(1)
	head := fight.Order[0].Ref
	var other ActorRef
	for _, e := range fight.Order {
		if e.Ref != head {
			other = e.Ref
			break
		}
	}

	var stance CombatStance
	var stanceMu sync.Mutex
	eventbus.Subscribe[CombatStance](bus, func(_ context.Context, ev CombatStance) {
		if ev.Kind != "sidestep" {
			return
		}
		stanceMu.Lock()
		stance = ev
		stanceMu.Unlock()
	})

	if err := mgr.EnqueueAction(1, head, Action{Kind: ActionSidestep, Target: other}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	mgr.Tick(context.Background())

	f, _ := mgr.Get(1)
	if got := f.FlatFootedUntil[other]; got != f.Round {
		t.Fatalf("FlatFootedUntil[target] = %d, want %d", got, f.Round)
	}
	stanceMu.Lock()
	defer stanceMu.Unlock()
	if stance.Kind != "sidestep" || stance.Actor != head || stance.Target != other {
		t.Fatalf("CombatStance = %+v, want kind=sidestep actor=%+v target=%+v",
			stance, head, other)
	}
}

// TestResolveSidestep_IgnoresUnknownTarget verifies the resolver
// silently no-ops when the named target isn't a fight participant —
// defense in depth for the verb-side validation.
func TestResolveSidestep_IgnoresUnknownTarget(t *testing.T) {
	mgr, _, _ := seedManager(t, 1)
	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	if _, err := mgr.Start(context.Background(), 1, parts); err != nil {
		t.Fatalf("start: %v", err)
	}
	fight, _ := mgr.Get(1)
	head := fight.Order[0].Ref
	stranger := ActorRef{Kind: ActorKindMob, ID: 999}

	if err := mgr.EnqueueAction(1, head, Action{Kind: ActionSidestep, Target: stranger}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	mgr.Tick(context.Background())

	if _, ok := fight.FlatFootedUntil[stranger]; ok {
		t.Fatal("sidestep should not flat-foot a non-participant")
	}
}
