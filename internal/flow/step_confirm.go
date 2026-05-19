package flow

import "strings"

// ConfirmStep is a yes/no terminator — typically the final step in a
// flow ("Commit these changes?"). Accepts y/yes/n/no (case-insensitive
// after trim); OnYes / OnNo route the runner to different next steps.
//
// Empty OnNo means "stay on this step" (re-prompt) which is rarely
// what callers want; a confirm step typically routes No to either an
// earlier review step or to flow completion (empty StepID).
//
// Mandatory fields: ID, PromptText, OnYes. OnNo defaults to staying
// on the step if blank, but most consumers should set it explicitly.
type ConfirmStep struct {
	ID         StepID `yaml:"id"`
	PromptText string `yaml:"prompt"`

	Action string `yaml:"action"`

	OnYes StepID `yaml:"on_yes"`
	OnNo  StepID `yaml:"on_no"`
}

func (s *ConfirmStep) StepID() StepID       { return s.ID }
func (s *ConfirmStep) ValidatorRef() string { return "" }
func (s *ConfirmStep) ActionRef() string    { return s.Action }
func (s *ConfirmStep) Prompt(*State) string { return s.PromptText + " (y/n) " }

func (s *ConfirmStep) Handle(state *State, input string) (StepID, error) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "y", "yes":
		return s.OnYes, nil
	case "n", "no":
		if s.OnNo == "" {
			return s.ID, &ValidationError{Message: "Cancelled — choose y to confirm."}
		}
		return s.OnNo, nil
	default:
		return s.ID, &ValidationError{Message: "Answer y or n."}
	}
}
