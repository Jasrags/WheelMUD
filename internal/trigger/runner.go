package trigger

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// Runner ties a Registry, the action handlers, and an ActionDeps
// bundle together. Dispatcher resolves owners and calls Fire; tests
// call Fire directly with a synthetic owner list.
type Runner struct {
	reg     *Registry
	actions *ActionRegistry
	deps    ActionDeps
}

// NewRunner builds a Runner. reg and actions must be non-nil; deps
// is captured by value (action handlers see exactly what was passed
// at construction time).
func NewRunner(reg *Registry, actions *ActionRegistry, deps ActionDeps) *Runner {
	return &Runner{reg: reg, actions: actions, deps: deps}
}

// ErrUnknownAction is returned (logged, not propagated to the
// publisher) when a trigger names an action with no registered
// handler. The fan-out continues with the next trigger.
var ErrUnknownAction = errors.New("trigger: unknown action")

// Fire dispatches every trigger in triggers against owner. Errors
// from individual handlers are logged and SWALLOWED — one bad
// trigger does not abort the rest of the fan-out, mirroring the
// eventbus subscriber-panic policy.
func (r *Runner) Fire(ctx context.Context, owner OwnerRef, ev EventCtx, triggers []repo.Trigger) {
	if r == nil || len(triggers) == 0 {
		return
	}
	logger := loggerOr(r.deps)
	for _, t := range triggers {
		handler := r.actions.Lookup(t.Action)
		if handler == nil {
			logger.Warn("trigger: unknown action",
				"action", t.Action,
				"event", string(t.Event),
				"trigger_id", t.ID,
				"owner_kind", string(t.OwnerKind),
				"owner_id", t.OwnerID)
			continue
		}
		if err := handler(ctx, r.deps, owner, ev, json.RawMessage(t.Payload)); err != nil {
			logger.Warn("trigger handler error",
				"action", t.Action,
				"event", string(t.Event),
				"trigger_id", t.ID,
				"error", err)
		}
	}
}

// FireForOwner is the common-case shortcut: look up triggers for
// (owner.Kind, owner.ID, ev.Event) and Fire them.
func (r *Runner) FireForOwner(ctx context.Context, owner OwnerRef, ev EventCtx) {
	if r == nil || r.reg == nil {
		return
	}
	r.Fire(ctx, owner, ev, r.reg.ForOwnerEvent(owner.Kind, owner.ID, ev.Event))
}

// Registry returns the underlying registry (for tests).
func (r *Runner) Registry() *Registry { return r.reg }
