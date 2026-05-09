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

// ErrActionFaulted is the sentinel an action handler returns to
// signal a fault that should count against the trigger's
// consecutive-fault budget. Any error wrapping this sentinel
// increments triggers.consecutive_faults; at FaultThreshold the
// runner sets triggers.disabled=1 and persists. A non-faulted
// success path resets the counter to 0 — flapping triggers don't
// accidentally trip the limit.
//
// The Lua action handler wraps `lua.ErrLuaError` /
// `lua.ErrInstructionLimit` / `lua.ErrTimeout` /
// `lua.ErrUnknownScript` with this sentinel. Other action handlers
// (say, emote, noop) generally don't fault — their payload-decode
// failures are silent no-ops, not budgeted faults.
var ErrActionFaulted = errors.New("trigger: action faulted")

// FaultThreshold is the per-trigger consecutive-fault count at
// which the runner auto-disables the trigger. Tunable later if
// noisy-fault content shows up.
const FaultThreshold = 5

// Fire dispatches every trigger in triggers against owner. Errors
// from individual handlers are logged and SWALLOWED — one bad
// trigger does not abort the rest of the fan-out, mirroring the
// eventbus subscriber-panic policy.
//
// Fault-budget interactions (Phase F #32 slice 1):
//   - Triggers whose Disabled flag is true are skipped silently
//     (the world loader resets the flag on every LoadAndSync, so
//     a tedit-grade unstick is "redeploy world").
//   - Handler errors wrapping ErrActionFaulted increment the
//     running consecutive_faults counter; at FaultThreshold the
//     trigger is auto-disabled and the new state persists.
//   - Handler success (nil err) resets the counter to 0 if it was
//     non-zero. Counter resets are persisted only when they
//     actually transition (zero → zero stays a no-op write).
func (r *Runner) Fire(ctx context.Context, owner OwnerRef, ev EventCtx, triggers []repo.Trigger) {
	if r == nil || len(triggers) == 0 {
		return
	}
	logger := loggerOr(r.deps)
	for _, t := range triggers {
		if t.Disabled {
			continue
		}
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
		err := handler(ctx, r.deps, owner, ev, json.RawMessage(t.Payload))
		if err == nil {
			r.recordSuccess(ctx, t)
			continue
		}
		if errors.Is(err, ErrActionFaulted) {
			r.recordFault(ctx, t, err)
			continue
		}
		logger.Warn("trigger handler error",
			"action", t.Action,
			"event", string(t.Event),
			"trigger_id", t.ID,
			"error", err)
	}
}

// recordFault bumps consecutive_faults on the in-memory registry
// trigger row (so the next Fire sees the new count) and persists
// the change. Auto-disables when the threshold is reached.
func (r *Runner) recordFault(ctx context.Context, t repo.Trigger, cause error) {
	logger := loggerOr(r.deps)
	newCount := t.ConsecutiveFaults + 1
	disabled := newCount >= FaultThreshold
	r.reg.UpdateFault(t.ID, newCount, disabled)
	logger.Warn("trigger fault",
		"trigger_id", t.ID,
		"action", t.Action,
		"event", string(t.Event),
		"consecutive_faults", newCount,
		"disabled", disabled,
		"error", cause)
	if r.deps.Triggers != nil {
		if err := r.deps.Triggers.RecordTriggerFault(ctx, t.ID, newCount, disabled); err != nil {
			logger.Warn("trigger fault persist failed",
				"trigger_id", t.ID, "error", err)
		}
	}
}

// recordSuccess resets a non-zero fault counter back to zero. A
// no-op when the counter is already zero (the common case).
func (r *Runner) recordSuccess(ctx context.Context, t repo.Trigger) {
	if t.ConsecutiveFaults == 0 && !t.Disabled {
		return
	}
	r.reg.UpdateFault(t.ID, 0, false)
	if r.deps.Triggers != nil {
		if err := r.deps.Triggers.RecordTriggerFault(ctx, t.ID, 0, false); err != nil {
			loggerOr(r.deps).Warn("trigger fault reset persist failed",
				"trigger_id", t.ID, "error", err)
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
