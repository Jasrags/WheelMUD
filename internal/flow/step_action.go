package flow

import (
	"fmt"
)

// ActionStep is a no-prompt step that fires a registered action and
// auto-advances to Next. Useful for side-effects that don't collect
// input — recording an admin_audit row, computing a derived value
// from prior steps, persisting a draft, etc.
//
// Implements AutoAdvancer; the Runner walks past it without
// rendering or waiting for Submit. Action lookup failure aborts the
// flow with a clear error; the action's own return error aborts the
// flow with the underlying error wrapped.
//
// Mandatory fields: ID, Action. Next may be empty to terminate the
// flow on action success.
type ActionStep struct {
	ID     StepID `yaml:"id"`
	Action string `yaml:"action"`
	Next   StepID `yaml:"next"`
}

func (s *ActionStep) StepID() StepID       { return s.ID }
func (s *ActionStep) ValidatorRef() string { return "" }

// ActionRef returns "" deliberately: the Stage-3 action hook in
// Runner.Submit is for steps that ALSO render a prompt and accept
// input. ActionStep's side-effect runs from Auto, not from that
// hook.
func (s *ActionStep) ActionRef() string    { return "" }
func (s *ActionStep) Prompt(*State) string { return "" }

// Handle is never called on a healthy code path — the AutoAdvancer
// branch in Runner.advanceTo intercepts ActionStep before Submit can
// reach it. Implemented defensively for the "Action step somehow
// became Current and someone Submitted" case.
func (s *ActionStep) Handle(*State, string) (StepID, error) {
	return s.ID, fmt.Errorf("ActionStep %q: Submit hit a no-prompt step (should auto-advance)", s.ID)
}

// Auto resolves Action against the runtime registry and invokes it.
// Returns Next on success.
func (s *ActionStep) Auto(rt AutoRuntime, state *State) (StepID, error) {
	if s.Action == "" {
		return "", fmt.Errorf("ActionStep %q: Action is required", s.ID)
	}
	reg := rt.Actions()
	fn, ok := reg.Lookup(s.Action)
	if !ok {
		return "", fmt.Errorf("ActionStep %q: unregistered action %q", s.ID, s.Action)
	}
	if err := fn(state); err != nil {
		return "", fmt.Errorf("ActionStep %q: action %q: %w", s.ID, s.Action, err)
	}
	return s.Next, nil
}
