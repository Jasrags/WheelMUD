package flow

import "strings"

// TextStep is the simplest interactive step: render PromptText,
// accept any non-empty input (after trim), optionally run an
// external validator, store under StoreAs, advance to Next.
//
// Mandatory fields: ID, PromptText, Next. StoreAs may be empty if
// the input is consumed for side-effect only (the action handles
// it); otherwise the input lands in state.Values[StoreAs].
type TextStep struct {
	ID         StepID `yaml:"id"`
	PromptText string `yaml:"prompt"`

	// Validator is the optional ValidatorRegistry key the Runner
	// invokes before Handle.
	Validator string `yaml:"validator"`

	// Action is the optional ActionRegistry key the Runner invokes
	// after Handle succeeds.
	Action string `yaml:"action"`

	// StoreAs keys state.Values for the validated input. Empty
	// means do not persist the input — the action is expected to
	// consume it.
	StoreAs string `yaml:"store_as"`

	// Next is the StepID to advance to on success. Empty Next
	// signals flow completion.
	Next StepID `yaml:"next"`

	// AllowEmpty toggles the intrinsic "non-empty" check. By
	// default, blank input re-prompts with a "Required." message;
	// setting AllowEmpty=true lets a step skip with no value.
	AllowEmpty bool `yaml:"allow_empty"`
}

func (s *TextStep) StepID() StepID         { return s.ID }
func (s *TextStep) ValidatorRef() string   { return s.Validator }
func (s *TextStep) ActionRef() string      { return s.Action }
func (s *TextStep) Prompt(*State) string   { return s.PromptText }

func (s *TextStep) Handle(state *State, input string) (StepID, error) {
	trimmed := strings.TrimSpace(input)
	if !s.AllowEmpty && trimmed == "" {
		return s.ID, &ValidationError{Message: "Required."}
	}
	if s.StoreAs != "" {
		state.SetValue(s.StoreAs, trimmed)
	}
	return s.Next, nil
}
