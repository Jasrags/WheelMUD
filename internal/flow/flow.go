// Package flow is the Phase O engine — a generic multi-step
// interactive Flow runner. A Flow is an ordered set of Steps; the
// Runner drives the State through them by handing input to the
// active step, validating, optionally invoking a registered Action,
// and advancing to the next StepID until completion.
//
// Phase O architecture decisions baked in (see docs/PLAN.md Phase O):
//
//   - Step is an interface; each kind (TextStep, ChoiceStep,
//     ConfirmStep, …) implements it. New kinds plug in without
//     touching the runner.
//   - Validation errors are typed: a Step.Handle returning
//     *ValidationError tells the runner to re-prompt the same step
//     with the error message. Any other error type aborts the flow.
//   - Engine output goes through a Renderer interface so tests use
//     a BufferRenderer and production wraps a *telnet.Session via
//     an adapter defined at the call site (O.1).
//   - State is plain-data (no pointers, no funcs in fields) so the
//     O.2 persistence layer can JSON-marshal it without refactoring.
//   - Actions and validators are referenced from steps by string and
//     resolved through ActionRegistry / ValidatorRegistry. New
//     behaviour requires a recompile and a Register call; YAML
//     content (O.1) references them by name.
//
// This package has zero `telnet` imports — keep it that way so the
// engine stays transport-agnostic and the mode adapter (O.1) is the
// only bridge to a Session.
package flow

import "fmt"

// StepID is the unique identifier of a step inside a Flow. Empty
// StepID ("") signals flow completion when returned from a Step.Handle
// or when assigned to Flow.Entry of a Flow with no steps.
type StepID string

// Flow is the static definition of a multi-step interactive flow.
// Constructed once and shared across goroutines; never mutated after
// validation. Steps may appear in any order in the slice — the
// runner walks them by ID, not by index.
type Flow struct {
	// ID is the catalog-unique identifier (e.g. "oedit", "chargen").
	// Used as a key into the future flow_state persistence table and
	// referenced by the O.1 YAML loader.
	ID string

	// Entry is the StepID of the first step the runner activates.
	// Empty Entry on a Flow with non-zero Steps is a Validate error.
	Entry StepID

	// Steps holds every step in this flow. Order is informational
	// only; the runner finds steps by StepID() via byID.
	Steps []Step

	// Resumable hints to the O.2 persistence layer whether mid-flow
	// state should survive disconnects. Currently informational —
	// O.0 keeps everything in-memory.
	Resumable bool

	// byID is the runtime lookup map built by Validate. nil before
	// Validate runs; callers must call Validate before using any
	// Runner against the flow.
	byID map[StepID]Step
}

// Validate builds the lookup map and enforces well-formedness:
//
//   - non-empty ID
//   - non-empty Entry that resolves to a Step in Steps
//   - every Step has a non-empty StepID()
//   - no duplicate StepIDs
//
// Returns an error suitable for boot-time loading; the runner refuses
// to Start a non-Validated flow.
func (f *Flow) Validate() error {
	if f == nil {
		return fmt.Errorf("flow: nil")
	}
	if f.ID == "" {
		return fmt.Errorf("flow: blank ID")
	}
	if len(f.Steps) == 0 {
		return fmt.Errorf("flow %q: no steps", f.ID)
	}
	f.byID = make(map[StepID]Step, len(f.Steps))
	for i, s := range f.Steps {
		if s == nil {
			return fmt.Errorf("flow %q: step %d is nil", f.ID, i)
		}
		id := s.StepID()
		if id == "" {
			return fmt.Errorf("flow %q: step %d has blank StepID", f.ID, i)
		}
		if _, dup := f.byID[id]; dup {
			return fmt.Errorf("flow %q: duplicate step %q", f.ID, id)
		}
		f.byID[id] = s
	}
	if f.Entry == "" {
		return fmt.Errorf("flow %q: blank Entry", f.ID)
	}
	if _, ok := f.byID[f.Entry]; !ok {
		return fmt.Errorf("flow %q: Entry %q not in Steps", f.ID, f.Entry)
	}
	return nil
}

// Step returns the step with the given ID, or nil if absent. Callers
// must Validate() the flow first; an unvalidated Flow always returns
// nil here.
func (f *Flow) Step(id StepID) Step {
	if f == nil || f.byID == nil {
		return nil
	}
	return f.byID[id]
}

// State is the runtime-mutable per-runner snapshot. Built when a
// Runner starts and threaded through every Step.Handle call. Plain
// data so the O.2 persistence layer can serialize it as JSON without
// changes here.
type State struct {
	// FlowID is the parent Flow.ID; redundant with the Runner's
	// reference but persisted for crash-recovery resume.
	FlowID string

	// AccountID is the running account, or 0 for tests / anonymous
	// flows. Future persistence layer keys (account_id, flow_id).
	AccountID int64

	// Current is the StepID the runner is presently awaiting input
	// for. Empty Current means the flow has completed or has not
	// started yet (Runner.Start sets it to Flow.Entry).
	Current StepID

	// Values accumulates per-step output keyed by the step's StoreAs
	// field. A step may store nothing (write-through to side-effect
	// only via an action), in which case its StoreAs is "" and it
	// does not appear here.
	Values map[string]string

	// Completed is true after a Step.Handle returns ("", nil) — the
	// flow's normal exit. Distinct from cancelled flows.
	Completed bool

	// Cancelled is true after Runner.Cancel is called. The current
	// step never completes; downstream consumers should treat this
	// as a user-initiated abort.
	Cancelled bool
}

// SetValue is the canonical write path for Values. Treats nil
// Values as empty rather than panicking — useful when a Step.Handle
// runs against a freshly-zeroed State (e.g. test fixtures).
func (s *State) SetValue(key, val string) {
	if s.Values == nil {
		s.Values = map[string]string{}
	}
	s.Values[key] = val
}

// ValidationError is the typed re-prompt signal. A Step.Handle
// returning a non-nil *ValidationError tells the Runner to re-display
// the step's prompt and append Message before reading the next
// input line. Any other error type aborts the flow.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "validation error"
	}
	return e.Message
}

// IsValidationError reports whether err is a re-prompt signal. Use
// this when the caller wants to distinguish input-shape errors from
// system errors without a type assertion.
func IsValidationError(err error) bool {
	_, ok := err.(*ValidationError)
	return ok
}
