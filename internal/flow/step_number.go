package flow

import (
	"fmt"
	"strconv"
	"strings"
)

// NumberStep collects an integer in [Min, Max] inclusive. The parsed
// value is stored under StoreAs as a strconv-formatted decimal so the
// downstream JSON round-trip stays string-typed like every other
// value in State.Values.
//
// Mandatory fields: ID, PromptText, Next. Min/Max default to 0/0 which
// only accepts 0; consumers should always set both. AllowEmpty=false
// (the default) re-prompts on blank input; AllowEmpty=true short-
// circuits to Next without storing anything (useful for optional age,
// optional manual stat override, etc.).
type NumberStep struct {
	ID         StepID `yaml:"id"`
	PromptText string `yaml:"prompt"`

	Min int `yaml:"min"`
	Max int `yaml:"max"`

	Validator string `yaml:"validator"`
	Action    string `yaml:"action"`

	StoreAs string `yaml:"store_as"`
	Next    StepID `yaml:"next"`

	AllowEmpty bool `yaml:"allow_empty"`
}

func (s *NumberStep) StepID() StepID       { return s.ID }
func (s *NumberStep) ValidatorRef() string { return s.Validator }
func (s *NumberStep) ActionRef() string    { return s.Action }
func (s *NumberStep) Prompt(*State) string { return s.PromptText }

func (s *NumberStep) Handle(state *State, input string) (StepID, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		if s.AllowEmpty {
			return s.Next, nil
		}
		return s.ID, &ValidationError{Message: "Enter a number."}
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil {
		return s.ID, &ValidationError{Message: fmt.Sprintf("%q is not a whole number.", trimmed)}
	}
	if n < s.Min || n > s.Max {
		return s.ID, &ValidationError{
			Message: fmt.Sprintf("Enter a number between %d and %d.", s.Min, s.Max),
		}
	}
	if s.StoreAs != "" {
		state.SetValue(s.StoreAs, strconv.Itoa(n))
	}
	return s.Next, nil
}
