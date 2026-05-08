package trigger

import (
	"context"
	"encoding/json"
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

func TestNoopAction_AcceptsEmptyPayload(t *testing.T) {
	if err := NoopAction(context.Background(), ActionDeps{}, OwnerRef{}, EventCtx{}, nil); err != nil {
		t.Fatalf("noop empty payload: %v", err)
	}
	if err := NoopAction(context.Background(), ActionDeps{}, OwnerRef{}, EventCtx{}, []byte(`{"message":"ok"}`)); err != nil {
		t.Fatalf("noop with message: %v", err)
	}
}
