package flow

import (
	"strings"
	"testing"
)

func TestConditionalStep_Auto(t *testing.T) {
	step := &ConditionalStep{
		ID:        "branch",
		Condition: "race",
		Cases: map[string]StepID{
			"aiel":  "ask_clan",
			"ogier": "ask_stedding",
		},
		Default: "common",
	}

	tests := []struct {
		name  string
		value string
		want  StepID
	}{
		{"aiel_case", "aiel", "ask_clan"},
		{"ogier_case", "ogier", "ask_stedding"},
		{"unknown_falls_to_default", "borderlander", "common"},
		{"missing_value_falls_to_default", "", "common"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := &State{Values: map[string]string{"race": tc.value}}
			got, err := step.Auto(nil, state)
			if err != nil {
				t.Fatalf("Auto: %v", err)
			}
			if got != tc.want {
				t.Errorf("next = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConditionalStep_NoDefault_NoMatch_Aborts(t *testing.T) {
	step := &ConditionalStep{
		ID:        "branch",
		Condition: "race",
		Cases:     map[string]StepID{"aiel": "ask_clan"},
	}
	state := &State{Values: map[string]string{"race": "ogier"}}
	_, err := step.Auto(nil, state)
	if err == nil {
		t.Fatal("expected abort error, got nil")
	}
	if !strings.Contains(err.Error(), "no case") {
		t.Errorf("error = %v, want mention of no case", err)
	}
}

func TestConditionalStep_EmptyCondition_Errors(t *testing.T) {
	step := &ConditionalStep{ID: "x", Cases: map[string]StepID{"a": "b"}}
	_, err := step.Auto(nil, &State{Values: map[string]string{}})
	if err == nil || !strings.Contains(err.Error(), "Condition") {
		t.Fatalf("expected Condition error, got %v", err)
	}
}

func TestConditionalStep_HandleIsDefensive(t *testing.T) {
	step := &ConditionalStep{ID: "x", Condition: "y", Cases: map[string]StepID{"a": "b"}}
	_, err := step.Handle(&State{}, "irrelevant")
	if err == nil {
		t.Fatal("Handle on no-prompt step should error")
	}
}
