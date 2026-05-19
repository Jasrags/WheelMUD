package flow

import (
	"errors"
	"fmt"
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
	}, nil
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
	return r.renderer.Write(step.Prompt(r.state))
}

// Submit feeds one line of input to the active step. Returns
// done=true and nil err when the flow has completed normally. A
// *ValidationError from validator or Handle re-prompts the same
// step (returns done=false, nil err); the caller can keep calling
// Submit with new input. Any other error aborts the flow and is
// returned untouched — caller should treat the runner as dead.
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
		return true, nil
	}
	nextStep := r.flow.Step(next)
	if nextStep == nil {
		return false, fmt.Errorf("flow %q: step %q advanced to unknown step %q", r.flow.ID, step.StepID(), next)
	}
	if err := r.renderer.Write(nextStep.Prompt(r.state)); err != nil {
		return false, fmt.Errorf("flow %q: render step %q: %w", r.flow.ID, next, err)
	}
	return false, nil
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
}
