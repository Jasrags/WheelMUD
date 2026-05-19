package flow

import "fmt"

// ConditionalStep is a no-prompt step that routes based on a prior
// step's stored value. Useful when a `choice` step's per-option
// `next:` isn't enough — e.g., a `text` step whose answer determines
// the next path, or a downstream step that needs to peek at a value
// stored several steps earlier.
//
// Implements AutoAdvancer. Lookup table semantics: Cases[
// state.Values[Condition]] is the next StepID, falling back to
// Default when the case is absent. Empty Default + no case match
// aborts the flow with an error pointing at the missing case.
//
// Mandatory fields: ID, Condition, Cases (≥1). Default is optional
// but recommended — a flow that aborts because of a typo in a player-
// supplied value is worse UX than a fallback "ask again" path.
type ConditionalStep struct {
	ID        StepID            `yaml:"id"`
	Condition string            `yaml:"condition"`
	Cases     map[string]StepID `yaml:"cases"`
	Default   StepID            `yaml:"default"`
}

func (s *ConditionalStep) StepID() StepID       { return s.ID }
func (s *ConditionalStep) ValidatorRef() string { return "" }
func (s *ConditionalStep) ActionRef() string    { return "" }
func (s *ConditionalStep) Prompt(*State) string { return "" }

// Handle is the defensive twin for ActionStep.Handle — should never
// fire because Runner.advanceTo intercepts AutoAdvancer.
func (s *ConditionalStep) Handle(*State, string) (StepID, error) {
	return s.ID, fmt.Errorf("ConditionalStep %q: Submit hit a no-prompt step (should auto-advance)", s.ID)
}

// Auto looks up state.Values[Condition] in Cases. Falls back to
// Default when neither the case nor Default applies, returning a
// clear error so the flow aborts visibly rather than ending in a
// silent zero StepID.
func (s *ConditionalStep) Auto(_ AutoRuntime, state *State) (StepID, error) {
	if s.Condition == "" {
		return "", fmt.Errorf("ConditionalStep %q: Condition is required", s.ID)
	}
	if len(s.Cases) == 0 && s.Default == "" {
		return "", fmt.Errorf("ConditionalStep %q: no cases and no default", s.ID)
	}
	value := state.Values[s.Condition]
	if next, ok := s.Cases[value]; ok {
		return next, nil
	}
	if s.Default != "" {
		return s.Default, nil
	}
	return "", fmt.Errorf("ConditionalStep %q: no case for %s=%q and no default", s.ID, s.Condition, value)
}
