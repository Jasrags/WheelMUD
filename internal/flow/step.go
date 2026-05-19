package flow

// Step is one node in a Flow. Each kind (TextStep, ChoiceStep,
// ConfirmStep, …) implements this interface; the Runner does not
// know about kinds, only about Steps.
//
// Lifecycle inside the Runner:
//
//  1. Runner.Start (or post-advance) renders the active step's
//     Prompt() through the Renderer.
//  2. Each Runner.Submit(input) call lands in Handle(state, input).
//  3. Handle parses input, validates, mutates state, optionally
//     looks up + invokes an action via state's runner context, and
//     returns the next StepID. Empty StepID ("") signals flow
//     completion.
//  4. A *ValidationError return tells the Runner to re-prompt this
//     same step with the error message appended.
//
// Steps MUST be deterministic from (state, input) and side-effect-
// free EXCEPT for the registered action they reference. The Runner
// resolves and invokes the action AFTER Handle returns successfully,
// which keeps the side-effect path separate from validation.
type Step interface {
	// StepID returns the step's flow-unique identifier. Returned
	// value must be stable across the step's lifetime.
	StepID() StepID

	// Prompt renders the user-facing prompt for this step. May
	// inspect state.Values to interpolate prior answers. Must not
	// mutate state.
	Prompt(state *State) string

	// Handle parses raw input, applies any intrinsic validation
	// (a ChoiceStep checking the input matches an option, a
	// ConfirmStep parsing yes/no), and reports the next StepID.
	// Returning an empty StepID and nil error means the flow has
	// completed. A *ValidationError return re-prompts the same
	// step. Any other error aborts the flow.
	//
	// Registry-based validation (ValidatorRef → ValidatorFn) is
	// orchestrated by the Runner BEFORE Handle is invoked; Handle
	// only sees input that has passed the external validator.
	//
	// Handle may mutate state.Values via state.SetValue, but must
	// not modify state.Current / Completed / Cancelled — those are
	// Runner-owned.
	Handle(state *State, input string) (StepID, error)

	// ValidatorRef returns the registry key of a ValidatorFn the
	// Runner invokes BEFORE Handle. Empty string means no external
	// validation; intrinsic validation in Handle still runs. Used
	// primarily by TextStep where the caller wires a domain-
	// specific check (name uniqueness, regex match, …); choice and
	// confirm steps typically return "" because their validation
	// is intrinsic.
	ValidatorRef() string

	// ActionRef returns the registry key of an ActionFn to invoke
	// AFTER Handle succeeds. Empty string means no action. The
	// Runner aborts the flow if a non-empty ref doesn't resolve in
	// the registry.
	ActionRef() string
}
