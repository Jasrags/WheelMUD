package flow

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Runner drives a single Flow instance against a single State. Not
// safe for concurrent use — each running flow gets its own Runner.
// The Runner is created at flow start and discarded on Cancel or
// Completion.
//
// Lifecycle:
//
//	r := NewRunner(flow, state, renderer, actions, validators)
//	r.Start()             // renders the entry step's prompt
//	done, err := r.Submit("alice")   // feeds input, advances
//	done, err := r.Submit("...")     // …until done == true
//
// Run-time errors come back from Submit: an unknown validator/action
// ref, a non-ValidationError from Handle, or an action returning
// non-nil. Any of these aborts the flow — state.Completed stays
// false, state.Cancelled stays false, but state.Current still points
// at the failing step for diagnostics.
type Runner struct {
	flow       *Flow
	state      *State
	renderer   Renderer
	actions    *ActionRegistry
	validators *ValidatorRegistry
	persister  Persister
	now        func() time.Time // injectable clock for deterministic tests
}

// NewRunner constructs a Runner. Returns an error if the flow is
// nil, has not been Validate()d, or if the renderer is nil. State
// may be nil — a fresh State is allocated and seeded from flow.ID
// in that case. Registries may be nil — any step referencing a
// validator or action then errors at Submit time with a clearer
// "registry not wired" message than a nil-deref panic.
func NewRunner(flow *Flow, state *State, renderer Renderer, actions *ActionRegistry, validators *ValidatorRegistry) (*Runner, error) {
	if flow == nil {
		return nil, errors.New("flow: nil Flow")
	}
	if flow.byID == nil {
		return nil, fmt.Errorf("flow %q: must call Validate before NewRunner", flow.ID)
	}
	if renderer == nil {
		return nil, errors.New("flow: nil Renderer")
	}
	if state == nil {
		state = &State{FlowID: flow.ID}
	}
	if state.Values == nil {
		state.Values = map[string]string{}
	}
	return &Runner{
		flow:       flow,
		state:      state,
		renderer:   renderer,
		actions:    actions,
		validators: validators,
		now:        func() time.Time { return time.Now().UTC() },
	}, nil
}

// SetPersister attaches an optional storage hook. Safe to call before
// Start; a nil Persister disables the save/delete path. Tests use
// this to inject a fake persister without a constructor explosion.
func (r *Runner) SetPersister(p Persister) {
	r.persister = p
}

// SetClock overrides the timestamp source for StartedAt / UpdatedAt.
// Test-only — production callers leave this alone.
func (r *Runner) SetClock(now func() time.Time) {
	if now != nil {
		r.now = now
	}
}

// State exposes the runner's underlying State for inspection. Tests
// (and the O.2 persistence layer) snapshot Values / Current /
// Completed through this accessor.
func (r *Runner) State() *State { return r.state }

// CurrentStep returns the active step (Flow.Step(State.Current)) or
// nil if the flow has not started, has completed, or has been
// cancelled. Used by the §O.1 mode adapter to re-render the active
// step's prompt on `/back`.
func (r *Runner) CurrentStep() Step {
	if r.state.Current == "" {
		return nil
	}
	return r.flow.Step(r.state.Current)
}

// Start activates the flow's Entry step and renders its prompt.
// Idempotent: calling Start a second time on the same runner is an
// error (Current is non-empty).
func (r *Runner) Start() error {
	if r.state.Current != "" {
		return fmt.Errorf("flow %q: already started at %q", r.flow.ID, r.state.Current)
	}
	if r.state.Completed || r.state.Cancelled {
		return fmt.Errorf("flow %q: already terminated", r.flow.ID)
	}
	r.state.Current = r.flow.Entry
	step := r.flow.Step(r.flow.Entry)
	if step == nil {
		// Defensive — Validate should have caught this.
		return fmt.Errorf("flow %q: entry step %q not found", r.flow.ID, r.flow.Entry)
	}
	now := r.now()
	if r.state.StartedAt.IsZero() {
		r.state.StartedAt = now
	}
	if err := r.persistSave(now); err != nil {
		return err
	}
	return r.renderer.Write(step.Prompt(r.state))
}

// Resume re-renders the current step's prompt against a hydrated
// State. The mode adapter calls this on reconnect when it found an
// existing flow_state row for (AccountID, FlowID). State.Current must
// already point at the step the player was awaiting input for —
// supplied by Persister-backed hydration before Resume is called.
//
// Distinct from Start: Resume never advances state, never invokes an
// action, and never calls Persister.Save (the persisted row is
// already correct; only a real Submit changes it).
func (r *Runner) Resume() error {
	if r.state.Current == "" {
		return fmt.Errorf("flow %q: Resume on non-hydrated state (Current is empty)", r.flow.ID)
	}
	if r.state.Completed || r.state.Cancelled {
		return fmt.Errorf("flow %q: Resume on terminated state", r.flow.ID)
	}
	step := r.flow.Step(r.state.Current)
	if step == nil {
		return fmt.Errorf("flow %q: Resume current step %q not in catalog", r.flow.ID, r.state.Current)
	}
	return r.renderer.Write(step.Prompt(r.state))
}

// Submit feeds one line of input to the active step. Returns
// done=true and nil err when the flow has completed normally.
//
// *ValidationError is consumed internally: when a validator or
// Step.Handle returns one, Submit calls reprompt (which writes the
// error message + re-renders the prompt via the Renderer) and
// returns (false, nil) on a successful re-prompt. Callers therefore
// never see a *ValidationError in the returned err — the only
// non-nil err is a hard abort. If reprompt's underlying
// Renderer.Write fails, Submit returns that write error
// (non-ValidationError) and the caller should treat the flow as
// dead.
//
// Submit on a completed or cancelled flow returns an error; the
// caller should check r.State().Completed / Cancelled before
// submitting again.
func (r *Runner) Submit(input string) (done bool, err error) {
	if r.state.Completed {
		return true, fmt.Errorf("flow %q: already completed", r.flow.ID)
	}
	if r.state.Cancelled {
		return false, fmt.Errorf("flow %q: cancelled", r.flow.ID)
	}
	if r.state.Current == "" {
		return false, fmt.Errorf("flow %q: not started", r.flow.ID)
	}
	step := r.flow.Step(r.state.Current)
	if step == nil {
		return false, fmt.Errorf("flow %q: current step %q vanished", r.flow.ID, r.state.Current)
	}

	// Stage 1: registry-based validation.
	if vref := step.ValidatorRef(); vref != "" {
		fn, ok := r.validators.Lookup(vref)
		if !ok {
			return false, fmt.Errorf("flow %q: step %q references unregistered validator %q", r.flow.ID, step.StepID(), vref)
		}
		if err := fn(r.state, input); err != nil {
			if ve, isV := err.(*ValidationError); isV {
				return false, r.reprompt(step, ve)
			}
			return false, fmt.Errorf("flow %q: step %q validator %q: %w", r.flow.ID, step.StepID(), vref, err)
		}
	}

	// Stage 2: intrinsic Handle.
	next, err := step.Handle(r.state, input)
	if err != nil {
		if ve, isV := err.(*ValidationError); isV {
			return false, r.reprompt(step, ve)
		}
		return false, fmt.Errorf("flow %q: step %q handle: %w", r.flow.ID, step.StepID(), err)
	}

	// Stage 3: post-success action.
	if aref := step.ActionRef(); aref != "" {
		fn, ok := r.actions.Lookup(aref)
		if !ok {
			return false, fmt.Errorf("flow %q: step %q references unregistered action %q", r.flow.ID, step.StepID(), aref)
		}
		if err := fn(r.state); err != nil {
			return false, fmt.Errorf("flow %q: step %q action %q: %w", r.flow.ID, step.StepID(), aref, err)
		}
	}

	// Stage 4: advance.
	r.state.Current = next
	if next == "" {
		r.state.Completed = true
		r.persistDelete()
		return true, nil
	}
	nextStep := r.flow.Step(next)
	if nextStep == nil {
		return false, fmt.Errorf("flow %q: step %q advanced to unknown step %q", r.flow.ID, step.StepID(), next)
	}
	// Save AFTER advancing Current so the persisted row points at the
	// step the runner is now awaiting input for — that's where Resume
	// will re-render.
	if err := r.persistSave(r.now()); err != nil {
		return false, err
	}
	if err := r.renderer.Write(nextStep.Prompt(r.state)); err != nil {
		return false, fmt.Errorf("flow %q: render step %q: %w", r.flow.ID, next, err)
	}
	return false, nil
}

// persistSave routes through the optional Persister. Skipped when no
// persister is wired or when the state is anonymous (AccountID == 0,
// i.e. tests). UpdatedAt is refreshed on every call so the repo can
// drive LRU eviction.
func (r *Runner) persistSave(now time.Time) error {
	if r.persister == nil || r.state.AccountID == 0 {
		return nil
	}
	r.state.UpdatedAt = now
	if err := r.persister.Save(context.Background(), r.state); err != nil {
		return fmt.Errorf("flow %q: persist save: %w", r.flow.ID, err)
	}
	return nil
}

// persistDelete drops the persisted row. Errors are swallowed — the
// flow already succeeded/cancelled and the next eviction sweep will
// clean up. Skipped when no persister is wired or the state is
// anonymous.
func (r *Runner) persistDelete() {
	if r.persister == nil || r.state.AccountID == 0 {
		return
	}
	_ = r.persister.Delete(context.Background(), r.state.AccountID, r.flow.ID)
}

// reprompt renders the validation-error message followed by the
// step's prompt again. State.Current stays put.
func (r *Runner) reprompt(step Step, ve *ValidationError) error {
	if err := r.renderer.Write(ve.Message + "\r\n"); err != nil {
		return fmt.Errorf("flow %q: render validation error: %w", r.flow.ID, err)
	}
	return r.renderer.Write(step.Prompt(r.state))
}

// Cancel marks the flow cancelled. Subsequent Submits error; the
// renderer is not written to (the caller decides whether to show a
// cancellation message — flows that share a cancel verb may want
// uniform wording controlled by the dispatcher, not the engine).
func (r *Runner) Cancel() {
	if r.state.Completed || r.state.Cancelled {
		return
	}
	r.state.Cancelled = true
	r.persistDelete()
}
