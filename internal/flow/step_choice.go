package flow

import (
	"fmt"
	"strconv"
	"strings"
)

// ChoiceOption is one selectable answer in a ChoiceStep. Label is
// the human-readable text shown in the prompt; Value is what lands
// in state.Values[StoreAs] when selected; Next is the per-option
// branching target (empty Next falls back to ChoiceStep.Next).
type ChoiceOption struct {
	Label string `yaml:"label"`
	Value string `yaml:"value"`
	Next  StepID `yaml:"next"`
}

// ChoiceStep presents a numbered list of options and accepts either
// a 1-based index or a case-insensitive label match. Per-option Next
// gives branching for free — chargen's "if Aiel, ask clan" pattern
// becomes a per-option Next, no conditional-step kind required for
// the simple cases.
//
// Mandatory fields: ID, PromptText, Options (≥1). StoreAs may be
// empty if the selected Value is consumed only via Next (rare but
// valid). Next is the fallback for options that leave their own
// Next blank.
type ChoiceStep struct {
	ID         StepID         `yaml:"id"`
	PromptText string         `yaml:"prompt"`
	Options    []ChoiceOption `yaml:"options"`

	Validator string `yaml:"validator"`
	Action    string `yaml:"action"`

	StoreAs string `yaml:"store_as"`
	Next    StepID `yaml:"next"`
}

func (s *ChoiceStep) StepID() StepID       { return s.ID }
func (s *ChoiceStep) ValidatorRef() string { return s.Validator }
func (s *ChoiceStep) ActionRef() string    { return s.Action }

// Prompt renders PromptText followed by a numbered options list.
// The rendering deliberately uses no cfmt tags — the renderer (or
// the caller's wrapper) is responsible for any colouring. Keeping
// the engine output palette-free lets the O.1 mode adapter style
// uniformly without per-step cfmt.
func (s *ChoiceStep) Prompt(*State) string {
	var b strings.Builder
	b.WriteString(s.PromptText)
	for i, opt := range s.Options {
		fmt.Fprintf(&b, "\r\n  %d) %s", i+1, opt.Label)
	}
	b.WriteString("\r\n")
	return b.String()
}

// Handle parses input as either a 1-based index or a label match.
// Case-insensitive label compare; whitespace trimmed. Returns a
// ValidationError on unknown input.
func (s *ChoiceStep) Handle(state *State, input string) (StepID, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return s.ID, &ValidationError{Message: "Pick one of the listed options."}
	}
	// Index match first — natural with a numbered list.
	if n, err := strconv.Atoi(trimmed); err == nil {
		if n < 1 || n > len(s.Options) {
			return s.ID, &ValidationError{
				Message: fmt.Sprintf("Choose between 1 and %d.", len(s.Options)),
			}
		}
		opt := s.Options[n-1]
		return s.selected(state, opt), nil
	}
	// Label match — case-insensitive.
	lower := strings.ToLower(trimmed)
	for _, opt := range s.Options {
		if strings.ToLower(opt.Label) == lower {
			return s.selected(state, opt), nil
		}
	}
	return s.ID, &ValidationError{Message: "Unknown option."}
}

func (s *ChoiceStep) selected(state *State, opt ChoiceOption) StepID {
	if s.StoreAs != "" {
		state.SetValue(s.StoreAs, opt.Value)
	}
	if opt.Next != "" {
		return opt.Next
	}
	return s.Next
}
