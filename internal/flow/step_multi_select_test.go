package flow

import (
	"strings"
	"testing"
)

func newMultiSelect() *MultiSelectStep {
	return &MultiSelectStep{
		ID:         "feats",
		PromptText: "Pick feats:",
		Options: []ChoiceOption{
			{Label: "Toughness", Value: "toughness"},
			{Label: "Dodge", Value: "dodge"},
			{Label: "Power Attack", Value: "power_attack"},
		},
		MinPicks: 1,
		MaxPicks: 2,
		StoreAs:  "feats",
		Next:     "done",
	}
}

func TestMultiSelectStep_Prompt_IncludesBoundsHint(t *testing.T) {
	step := newMultiSelect()
	got := step.Prompt(nil)
	for _, want := range []string{"Pick feats:", "1) Toughness", "2) Dodge", "pick 1–2"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q in %q", want, got)
		}
	}
}

func TestMultiSelectStep_Handle_Indices(t *testing.T) {
	step := newMultiSelect()
	state := &State{Values: map[string]string{}}
	got, err := step.Handle(state, "1, 3")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "done" {
		t.Errorf("next = %q, want done", got)
	}
	if state.Values["feats"] != `["toughness","power_attack"]` {
		t.Errorf("stored = %q, want JSON array", state.Values["feats"])
	}
}

func TestMultiSelectStep_Handle_Labels_CaseInsensitive(t *testing.T) {
	step := newMultiSelect()
	state := &State{Values: map[string]string{}}
	if _, err := step.Handle(state, "toughness, dodge"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if state.Values["feats"] != `["toughness","dodge"]` {
		t.Errorf("stored = %q", state.Values["feats"])
	}
}

func TestMultiSelectStep_Handle_MixedIndexAndLabel(t *testing.T) {
	step := newMultiSelect()
	state := &State{Values: map[string]string{}}
	if _, err := step.Handle(state, "1, Dodge"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if state.Values["feats"] != `["toughness","dodge"]` {
		t.Errorf("stored = %q", state.Values["feats"])
	}
}

func TestMultiSelectStep_Handle_RejectsDuplicates(t *testing.T) {
	step := newMultiSelect()
	_, err := step.Handle(&State{Values: map[string]string{}}, "1, Toughness")
	if !IsValidationError(err) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestMultiSelectStep_Handle_RejectsUnknown(t *testing.T) {
	step := newMultiSelect()
	for _, tok := range []string{"99", "Nonsense", "0"} {
		_, err := step.Handle(&State{Values: map[string]string{}}, tok)
		if !IsValidationError(err) {
			t.Errorf("token %q: expected ValidationError, got %v", tok, err)
		}
	}
}

func TestMultiSelectStep_Handle_RejectsOutOfBounds(t *testing.T) {
	step := newMultiSelect() // 1..2 picks
	// Too few.
	_, err := step.Handle(&State{Values: map[string]string{}}, "")
	if !IsValidationError(err) {
		t.Errorf("empty input with MinPicks>0: expected ValidationError, got %v", err)
	}
	// Too many.
	_, err = step.Handle(&State{Values: map[string]string{}}, "1, 2, 3")
	if !IsValidationError(err) {
		t.Errorf("3 picks with MaxPicks=2: expected ValidationError, got %v", err)
	}
}

func TestMultiSelectStep_Handle_EmptyAllowedWhenMinZero(t *testing.T) {
	step := newMultiSelect()
	step.MinPicks = 0
	state := &State{Values: map[string]string{}}
	got, err := step.Handle(state, "")
	if err != nil || got != "done" {
		t.Fatalf("empty allowed: got=%q err=%v", got, err)
	}
	if state.Values["feats"] != "[]" {
		t.Errorf("empty stored = %q, want []", state.Values["feats"])
	}
}

func TestMultiSelectStep_Handle_WhitespaceTrimmed(t *testing.T) {
	step := newMultiSelect()
	state := &State{Values: map[string]string{}}
	if _, err := step.Handle(state, "  1  ,   dodge  "); err != nil {
		t.Fatalf("err: %v", err)
	}
	if state.Values["feats"] != `["toughness","dodge"]` {
		t.Errorf("stored = %q", state.Values["feats"])
	}
}
