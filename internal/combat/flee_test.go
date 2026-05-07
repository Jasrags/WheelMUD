package combat

import (
	"context"
	"sync"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/eventbus"
)

// stubFleeMover records calls and returns a fixed FleeResult.
type stubFleeMover struct {
	mu   sync.Mutex
	res  FleeResult
	last ActorRef
}

func (s *stubFleeMover) AttemptFlee(_ context.Context, _ int64, actor ActorRef) FleeResult {
	s.mu.Lock()
	s.last = actor
	r := s.res
	s.mu.Unlock()
	return r
}

func TestResolveFlee_SuccessMarksFled(t *testing.T) {
	mgr, _, bus := seedManager(t, 1)
	mover := &stubFleeMover{res: FleeResult{
		Success:   true,
		Direction: "n",
		ToRoomID:  2,
		Reason:    "moved",
	}}
	mgr.SetFleeMover(mover)

	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	if _, err := mgr.Start(context.Background(), 1, parts); err != nil {
		t.Fatalf("start: %v", err)
	}
	fight, _ := mgr.Get(1)
	head := fight.Order[0].Ref

	var fleeEv CombatFlee
	var fleeMu sync.Mutex
	eventbus.Subscribe[CombatFlee](bus, func(_ context.Context, ev CombatFlee) {
		fleeMu.Lock()
		fleeEv = ev
		fleeMu.Unlock()
	})

	if err := mgr.EnqueueAction(1, head, Action{Kind: ActionFlee}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	mgr.Tick(context.Background())

	fleeMu.Lock()
	if !fleeEv.Success || fleeEv.Direction != "n" || fleeEv.ToRoomID != 2 {
		t.Fatalf("CombatFlee = %+v, want success/n/2", fleeEv)
	}
	fleeMu.Unlock()

	f, _ := mgr.Get(1)
	if f == nil {
		t.Fatal("fight ended before pruneDead could observe Fled")
	}
	if _, ok := f.Fled[head]; !ok {
		t.Fatal("actor not added to f.Fled after successful flee")
	}
}

func TestResolveFlee_FailureKeepsActorInOrder(t *testing.T) {
	mgr, _, bus := seedManager(t, 1)
	mover := &stubFleeMover{res: FleeResult{Reason: "rolled_failure"}}
	mgr.SetFleeMover(mover)

	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	if _, err := mgr.Start(context.Background(), 1, parts); err != nil {
		t.Fatalf("start: %v", err)
	}
	fight, _ := mgr.Get(1)
	head := fight.Order[0].Ref

	var fleeEv CombatFlee
	var fleeMu sync.Mutex
	eventbus.Subscribe[CombatFlee](bus, func(_ context.Context, ev CombatFlee) {
		fleeMu.Lock()
		fleeEv = ev
		fleeMu.Unlock()
	})

	if err := mgr.EnqueueAction(1, head, Action{Kind: ActionFlee}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	mgr.Tick(context.Background())

	fleeMu.Lock()
	defer fleeMu.Unlock()
	if fleeEv.Success {
		t.Fatalf("expected failure: %+v", fleeEv)
	}
	if fleeEv.Reason != "rolled_failure" {
		t.Fatalf("Reason = %q, want rolled_failure", fleeEv.Reason)
	}
	f, _ := mgr.Get(1)
	if f == nil {
		t.Fatal("fight ended unexpectedly on failed flee")
	}
	if _, ok := f.Fled[head]; ok {
		t.Fatal("actor must not be in Fled on failure")
	}
}

func TestResolveFlee_NoMoverPublishesNoMover(t *testing.T) {
	mgr, _, bus := seedManager(t, 1)
	parts := []ActorRef{{Kind: ActorKindMob, ID: 1}, {Kind: ActorKindMob, ID: 2}}
	if _, err := mgr.Start(context.Background(), 1, parts); err != nil {
		t.Fatalf("start: %v", err)
	}
	fight, _ := mgr.Get(1)
	head := fight.Order[0].Ref

	var fleeEv CombatFlee
	var fleeMu sync.Mutex
	eventbus.Subscribe[CombatFlee](bus, func(_ context.Context, ev CombatFlee) {
		fleeMu.Lock()
		fleeEv = ev
		fleeMu.Unlock()
	})

	_ = mgr.EnqueueAction(1, head, Action{Kind: ActionFlee})
	mgr.Tick(context.Background())

	fleeMu.Lock()
	defer fleeMu.Unlock()
	if fleeEv.Success {
		t.Fatalf("no-mover flee should fail: %+v", fleeEv)
	}
	if fleeEv.Reason != "no_mover" {
		t.Fatalf("Reason = %q, want no_mover", fleeEv.Reason)
	}
}
