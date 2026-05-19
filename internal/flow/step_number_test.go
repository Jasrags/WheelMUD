package flow

import (
	"strings"
	"testing"
)

func TestNumberStep_Handle(t *testing.T) {
	step := &NumberStep{ID: "age", PromptText: "Age?", Min: 16, Max: 120, StoreAs: "age", Next: "done"}

	tests := []struct {
		name      string
		input     string
		wantNext  StepID
		wantStore string // value stored under "age"; "" means not stored
		wantValid bool   // true = re-prompt expected
	}{
		{"happy_path", "30", "done", "30", false},
		{"min_boundary", "16", "done", "16", false},
		{"max_boundary", "120", "done", "120", false},
		{"below_min", "15", "age", "", true},
		{"above_max", "121", "age", "", true},
		{"non_numeric", "thirty", "age", "", true},
		{"blank", "  ", "age", "", true},
		{"whitespace_around_num", "  42  ", "done", "42", false},
		{"negative", "-5", "age", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := &State{Values: map[string]string{}}
			got, err := step.Handle(state, tc.input)
			if got != tc.wantNext {
				t.Errorf("next = %q, want %q", got, tc.wantNext)
			}
			if tc.wantValid && !IsValidationError(err) {
				t.Errorf("expected ValidationError, got %v", err)
			}
			if !tc.wantValid && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if state.Values["age"] != tc.wantStore {
				t.Errorf("stored = %q, want %q", state.Values["age"], tc.wantStore)
			}
		})
	}
}

func TestNumberStep_AllowEmpty(t *testing.T) {
	step := &NumberStep{ID: "n", Min: 0, Max: 9, StoreAs: "n", Next: "done", AllowEmpty: true}
	state := &State{Values: map[string]string{}}
	got, err := step.Handle(state, "")
	if err != nil || got != "done" {
		t.Fatalf("AllowEmpty empty input: next=%q err=%v, want next=done err=nil", got, err)
	}
	if v, ok := state.Values["n"]; ok {
		t.Errorf("empty input should not store anything, got %q", v)
	}
}

func TestNumberStep_ValidationMessages(t *testing.T) {
	step := &NumberStep{ID: "n", Min: 1, Max: 5, StoreAs: "n", Next: "done"}
	state := &State{Values: map[string]string{}}
	_, err := step.Handle(state, "9")
	ve, ok := err.(*ValidationError)
	if !ok || !strings.Contains(ve.Message, "1 and 5") {
		t.Errorf("range error message = %v, want mention of 1 and 5", err)
	}
	_, err = step.Handle(state, "abc")
	ve, ok = err.(*ValidationError)
	if !ok || !strings.Contains(ve.Message, "whole number") {
		t.Errorf("parse error message = %v, want mention of whole number", err)
	}
}
