package flow

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// MultiSelectStep presents a numbered options list and accepts a
// comma-separated list of indices and/or labels. Stores the chosen
// values as a JSON-encoded string array under StoreAs so a downstream
// action can unmarshal into a []string without changing the underlying
// State.Values map[string]string contract.
//
// Constraints MinPicks / MaxPicks (both inclusive) bound the count;
// 0/0 means "any number, including zero". Duplicates trigger a
// re-prompt — selecting the same option twice in one submission is a
// user-visible error rather than silently deduplicating.
//
// Mandatory fields: ID, PromptText, Options (≥1), Next.
type MultiSelectStep struct {
	ID         StepID         `yaml:"id"`
	PromptText string         `yaml:"prompt"`
	Options    []ChoiceOption `yaml:"options"`

	MinPicks int `yaml:"min_picks"`
	MaxPicks int `yaml:"max_picks"`

	Validator string `yaml:"validator"`
	Action    string `yaml:"action"`

	StoreAs string `yaml:"store_as"`
	Next    StepID `yaml:"next"`
}

func (s *MultiSelectStep) StepID() StepID       { return s.ID }
func (s *MultiSelectStep) ValidatorRef() string { return s.Validator }
func (s *MultiSelectStep) ActionRef() string    { return s.Action }

func (s *MultiSelectStep) Prompt(*State) string {
	var b strings.Builder
	b.WriteString(s.PromptText)
	for i, opt := range s.Options {
		fmt.Fprintf(&b, "\r\n  %d) %s", i+1, opt.Label)
	}
	bounds := s.boundsHint()
	if bounds != "" {
		fmt.Fprintf(&b, "\r\n(%s; comma-separated)", bounds)
	} else {
		b.WriteString("\r\n(comma-separated)")
	}
	b.WriteString("\r\n")
	return b.String()
}

func (s *MultiSelectStep) boundsHint() string {
	switch {
	case s.MinPicks > 0 && s.MaxPicks > 0 && s.MinPicks == s.MaxPicks:
		return fmt.Sprintf("pick exactly %d", s.MinPicks)
	case s.MinPicks > 0 && s.MaxPicks > 0:
		return fmt.Sprintf("pick %d–%d", s.MinPicks, s.MaxPicks)
	case s.MinPicks > 0:
		return fmt.Sprintf("pick at least %d", s.MinPicks)
	case s.MaxPicks > 0:
		return fmt.Sprintf("pick up to %d", s.MaxPicks)
	}
	return ""
}

// Handle parses a comma-separated list of indices or labels and
// stores the resulting JSON array under StoreAs. ValidationErrors:
//   - empty input when MinPicks > 0
//   - unknown token (not a parseable index in range or a label match)
//   - duplicate token
//   - count outside [MinPicks, MaxPicks]
func (s *MultiSelectStep) Handle(state *State, input string) (StepID, error) {
	tokens := splitMultiSelect(input)
	if len(tokens) == 0 {
		if s.MinPicks > 0 {
			return s.ID, &ValidationError{Message: "Pick at least one option."}
		}
		// Zero picks allowed — store an empty array and advance.
		if s.StoreAs != "" {
			state.SetValue(s.StoreAs, "[]")
		}
		return s.Next, nil
	}
	values := make([]string, 0, len(tokens))
	seen := map[string]struct{}{}
	for _, tok := range tokens {
		opt, ok := s.lookupOption(tok)
		if !ok {
			return s.ID, &ValidationError{Message: fmt.Sprintf("Unknown option %q.", tok)}
		}
		if _, dup := seen[opt.Value]; dup {
			return s.ID, &ValidationError{Message: fmt.Sprintf("Duplicate pick %q.", opt.Label)}
		}
		seen[opt.Value] = struct{}{}
		values = append(values, opt.Value)
	}
	if s.MinPicks > 0 && len(values) < s.MinPicks {
		return s.ID, &ValidationError{Message: fmt.Sprintf("Pick at least %d.", s.MinPicks)}
	}
	if s.MaxPicks > 0 && len(values) > s.MaxPicks {
		return s.ID, &ValidationError{Message: fmt.Sprintf("Pick at most %d.", s.MaxPicks)}
	}
	if s.StoreAs != "" {
		encoded, err := json.Marshal(values)
		if err != nil {
			// Marshal of a []string never fails in practice; defensive.
			return s.ID, fmt.Errorf("MultiSelectStep %q: marshal picks: %w", s.ID, err)
		}
		state.SetValue(s.StoreAs, string(encoded))
	}
	return s.Next, nil
}

func splitMultiSelect(input string) []string {
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func (s *MultiSelectStep) lookupOption(tok string) (ChoiceOption, bool) {
	if n, err := strconv.Atoi(tok); err == nil {
		if n >= 1 && n <= len(s.Options) {
			return s.Options[n-1], true
		}
		return ChoiceOption{}, false
	}
	lower := strings.ToLower(tok)
	for _, opt := range s.Options {
		if strings.ToLower(opt.Label) == lower {
			return opt, true
		}
	}
	return ChoiceOption{}, false
}
