package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

func TestRunner_FireDispatchesInOrder(t *testing.T) {
	var order []int64
	actions := NewActionRegistry()
	actions.Register("rec", func(_ context.Context, _ ActionDeps, _ OwnerRef, _ EventCtx, p json.RawMessage) error {
		var x struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal(p, &x)
		order = append(order, x.ID)
		return nil
	})

	reg := NewRegistry()
	reg.Replace([]repo.Trigger{
		{ID: 11, OwnerKind: OwnerRoom, OwnerID: 1, Event: EventOnEnter, Action: "rec", Payload: `{"id":11}`, Priority: 1},
		{ID: 22, OwnerKind: OwnerRoom, OwnerID: 1, Event: EventOnEnter, Action: "rec", Payload: `{"id":22}`, Priority: 5},
		{ID: 33, OwnerKind: OwnerRoom, OwnerID: 1, Event: EventOnEnter, Action: "rec", Payload: `{"id":33}`, Priority: 3},
	})

	r := NewRunner(reg, actions, ActionDeps{})
	r.FireForOwner(context.Background(), OwnerRef{Kind: OwnerRoom, ID: 1, RoomID: 1},
		EventCtx{Event: EventOnEnter, RoomID: 1})

	want := []int64{22, 33, 11}
	if len(order) != len(want) {
		t.Fatalf("len = %d want %d (order=%v)", len(order), len(want), order)
	}
	for i, v := range want {
		if order[i] != v {
			t.Fatalf("order[%d] = %d want %d", i, order[i], v)
		}
	}
}

func TestRunner_UnknownActionLoggedNotPropagated(t *testing.T) {
	var calls atomic.Int32
	actions := NewActionRegistry()
	actions.Register("rec", func(_ context.Context, _ ActionDeps, _ OwnerRef, _ EventCtx, _ json.RawMessage) error {
		calls.Add(1)
		return nil
	})

	reg := NewRegistry()
	reg.Replace([]repo.Trigger{
		{ID: 1, OwnerKind: OwnerRoom, OwnerID: 1, Event: EventOnEnter, Action: "missing"},
		{ID: 2, OwnerKind: OwnerRoom, OwnerID: 1, Event: EventOnEnter, Action: "rec"},
	})
	r := NewRunner(reg, actions, ActionDeps{})
	r.FireForOwner(context.Background(), OwnerRef{Kind: OwnerRoom, ID: 1, RoomID: 1},
		EventCtx{Event: EventOnEnter, RoomID: 1})

	if calls.Load() != 1 {
		t.Fatalf("rec calls = %d, want 1 (unknown action should not abort fan-out)", calls.Load())
	}
}

func TestRunner_HandlerErrorDoesNotAbort(t *testing.T) {
	var calls atomic.Int32
	actions := NewActionRegistry()
	actions.Register("boom", func(_ context.Context, _ ActionDeps, _ OwnerRef, _ EventCtx, _ json.RawMessage) error {
		calls.Add(1)
		return ErrUnknownAction
	})
	actions.Register("rec", func(_ context.Context, _ ActionDeps, _ OwnerRef, _ EventCtx, _ json.RawMessage) error {
		calls.Add(1)
		return nil
	})

	reg := NewRegistry()
	reg.Replace([]repo.Trigger{
		{ID: 1, OwnerKind: OwnerRoom, OwnerID: 1, Event: EventOnEnter, Action: "boom"},
		{ID: 2, OwnerKind: OwnerRoom, OwnerID: 1, Event: EventOnEnter, Action: "rec"},
	})
	r := NewRunner(reg, actions, ActionDeps{})
	r.FireForOwner(context.Background(), OwnerRef{Kind: OwnerRoom, ID: 1, RoomID: 1},
		EventCtx{Event: EventOnEnter, RoomID: 1})

	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2 (handler error should not abort)", calls.Load())
	}
}

func TestRunner_FaultBudgetAutoDisables(t *testing.T) {
	// A handler that consistently faults must trip the per-trigger
	// fault budget exactly at FaultThreshold. The 6th invocation
	// should be a no-op because the trigger flipped Disabled on
	// fire #5.
	var calls atomic.Int32
	actions := NewActionRegistry()
	actions.Register("flaky", func(_ context.Context, _ ActionDeps, _ OwnerRef, _ EventCtx, _ json.RawMessage) error {
		calls.Add(1)
		return fmt.Errorf("%w: synthetic", ErrActionFaulted)
	})

	repoBacking := repo.NewMemoryTriggerRepo()
	created, err := repoBacking.Create(context.Background(), repo.Trigger{
		OwnerKind: repo.TriggerOwnerRoom, OwnerID: 1,
		Event: repo.TriggerEventOnEnter, Action: "flaky", Payload: "{}",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	reg := NewRegistry()
	reg.Replace([]repo.Trigger{created})
	r := NewRunner(reg, actions, ActionDeps{Triggers: repoBacking})

	owner := OwnerRef{Kind: OwnerRoom, ID: 1, RoomID: 1}
	ev := EventCtx{Event: EventOnEnter, RoomID: 1}

	for i := 0; i < FaultThreshold+1; i++ {
		r.FireForOwner(context.Background(), owner, ev)
	}

	if calls.Load() != int32(FaultThreshold) {
		t.Fatalf("handler called %d times, want %d (post-disable fires must be no-ops)",
			calls.Load(), FaultThreshold)
	}
	rows, _ := repoBacking.ListByOwner(context.Background(), repo.TriggerOwnerRoom, 1)
	if rows[0].ConsecutiveFaults != FaultThreshold {
		t.Fatalf("consecutive_faults = %d, want %d", rows[0].ConsecutiveFaults, FaultThreshold)
	}
	if !rows[0].Disabled {
		t.Fatalf("expected Disabled=true after threshold")
	}
}

func TestRunner_SuccessResetsFaultCounter(t *testing.T) {
	// A successful invocation after a partial fault streak should
	// reset consecutive_faults back to zero.
	calls := 0
	wantFault := true
	actions := NewActionRegistry()
	actions.Register("flap", func(_ context.Context, _ ActionDeps, _ OwnerRef, _ EventCtx, _ json.RawMessage) error {
		calls++
		if wantFault {
			return fmt.Errorf("%w: synthetic", ErrActionFaulted)
		}
		return nil
	})

	repoBacking := repo.NewMemoryTriggerRepo()
	created, err := repoBacking.Create(context.Background(), repo.Trigger{
		OwnerKind: repo.TriggerOwnerRoom, OwnerID: 2,
		Event: repo.TriggerEventOnEnter, Action: "flap", Payload: "{}",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	reg := NewRegistry()
	reg.Replace([]repo.Trigger{created})
	r := NewRunner(reg, actions, ActionDeps{Triggers: repoBacking})

	owner := OwnerRef{Kind: OwnerRoom, ID: 2, RoomID: 2}
	ev := EventCtx{Event: EventOnEnter, RoomID: 2}

	// Three faults — counter at 3, not yet disabled.
	for i := 0; i < 3; i++ {
		r.FireForOwner(context.Background(), owner, ev)
	}
	rows, _ := repoBacking.ListByOwner(context.Background(), repo.TriggerOwnerRoom, 2)
	if rows[0].ConsecutiveFaults != 3 {
		t.Fatalf("after 3 faults: %d", rows[0].ConsecutiveFaults)
	}

	// One success — counter resets.
	wantFault = false
	r.FireForOwner(context.Background(), owner, ev)
	rows, _ = repoBacking.ListByOwner(context.Background(), repo.TriggerOwnerRoom, 2)
	if rows[0].ConsecutiveFaults != 0 {
		t.Fatalf("success did not reset counter: %d", rows[0].ConsecutiveFaults)
	}
}

func TestNoopAction_AcceptsEmptyPayload(t *testing.T) {
	if err := NoopAction(context.Background(), ActionDeps{}, OwnerRef{}, EventCtx{}, nil); err != nil {
		t.Fatalf("noop empty payload: %v", err)
	}
	if err := NoopAction(context.Background(), ActionDeps{}, OwnerRef{}, EventCtx{}, []byte(`{"message":"ok"}`)); err != nil {
		t.Fatalf("noop with message: %v", err)
	}
}
