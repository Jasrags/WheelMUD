package flow

import (
	"fmt"
	"strings"
	"testing"
)

// chainStep is a test-only AutoAdvancer that always advances to the
// configured Next. Used to build deterministic auto-advance chains.
type chainStep struct {
	ID   StepID
	Next StepID
}

func (s *chainStep) StepID() StepID                               { return s.ID }
func (s *chainStep) Prompt(*State) string                         { return "" }
func (s *chainStep) ValidatorRef() string                         { return "" }
func (s *chainStep) ActionRef() string                            { return "" }
func (s *chainStep) Handle(*State, string) (StepID, error)        { return s.ID, fmt.Errorf("Handle on chainStep") }
func (s *chainStep) Auto(_ AutoRuntime, _ *State) (StepID, error) { return s.Next, nil }

func newRunnerFor(t *testing.T, fl *Flow) (*Runner, *BufferRenderer) {
	t.Helper()
	if err := fl.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	br := &BufferRenderer{}
	r, err := NewRunner(fl, &State{FlowID: fl.ID}, br, nil, nil)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	return r, br
}

func TestRunner_AutoAdvance_ChainsThroughToRenderStep(t *testing.T) {
	// auto1 → auto2 → auto3 → text("end") → complete
	fl := &Flow{
		ID:    "chain",
		Entry: "a1",
		Steps: []Step{
			&chainStep{ID: "a1", Next: "a2"},
			&chainStep{ID: "a2", Next: "a3"},
			&chainStep{ID: "a3", Next: "end"},
			&TextStep{ID: "end", PromptText: "stop?", StoreAs: "x", Next: ""},
		},
	}
	r, br := newRunnerFor(t, fl)
	if err := r.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// The runner should have chained past all 3 auto steps and landed
	// on the text step's prompt.
	if br.String() != "stop?" {
		t.Errorf("rendered = %q, want \"stop?\"", br.String())
	}
	if r.State().Current != "end" {
		t.Errorf("Current = %q, want end", r.State().Current)
	}
	// Submitting one input finalises the flow.
	done, err := r.Submit("ok")
	if err != nil || !done {
		t.Fatalf("submit: done=%v err=%v", done, err)
	}
}

func TestRunner_AutoAdvance_ChainToCompletion(t *testing.T) {
	// Empty-Next auto step terminates the flow.
	fl := &Flow{
		ID:    "chain",
		Entry: "a1",
		Steps: []Step{
			&chainStep{ID: "a1", Next: "a2"},
			&chainStep{ID: "a2", Next: ""}, // terminates
		},
	}
	r, _ := newRunnerFor(t, fl)
	if err := r.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !r.State().Completed {
		t.Errorf("Completed=false after auto-chain to empty Next")
	}
}

func TestRunner_AutoAdvance_RecursionCapAborts(t *testing.T) {
	// Self-loop forces MaxAutoChain to trip.
	fl := &Flow{
		ID:    "loop",
		Entry: "spin",
		Steps: []Step{
			&chainStep{ID: "spin", Next: "spin"},
		},
	}
	r, _ := newRunnerFor(t, fl)
	err := r.Start()
	if err == nil {
		t.Fatal("expected MaxAutoChain abort, got nil")
	}
	if !strings.Contains(err.Error(), "MaxAutoChain") {
		t.Errorf("error = %v, want mention of MaxAutoChain", err)
	}
}

func TestRunner_AutoAdvance_ActionStepFiresAndChains(t *testing.T) {
	reg := NewActionRegistry()
	fired := false
	_ = reg.Register("commit", func(s *State) error { fired = true; s.SetValue("k", "v"); return nil })

	fl := &Flow{
		ID:    "act",
		Entry: "save",
		Steps: []Step{
			&ActionStep{ID: "save", Action: "commit", Next: "end"},
			&TextStep{ID: "end", PromptText: "stop?", StoreAs: "stop", Next: ""},
		},
	}
	if err := fl.Validate(); err != nil {
		t.Fatal(err)
	}
	br := &BufferRenderer{}
	r, _ := NewRunner(fl, &State{FlowID: fl.ID}, br, reg, nil)
	if err := r.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !fired {
		t.Error("action did not fire")
	}
	if r.State().Values["k"] != "v" {
		t.Errorf("action did not mutate state: %+v", r.State().Values)
	}
	if br.String() != "stop?" {
		t.Errorf("rendered = %q, want stop?", br.String())
	}
}

func TestRunner_AutoAdvance_ConditionalRoutesOnValue(t *testing.T) {
	fl := &Flow{
		ID:    "cond",
		Entry: "pick",
		Steps: []Step{
			&ChoiceStep{
				ID:         "pick",
				PromptText: "kind?",
				StoreAs:    "kind",
				Options: []ChoiceOption{
					{Label: "weapon", Value: "weapon"},
					{Label: "armor", Value: "armor"},
				},
				Next: "branch",
			},
			&ConditionalStep{
				ID:        "branch",
				Condition: "kind",
				Cases: map[string]StepID{
					"weapon": "wpn",
					"armor":  "arm",
				},
			},
			&TextStep{ID: "wpn", PromptText: "weapon name?", Next: ""},
			&TextStep{ID: "arm", PromptText: "armor name?", Next: ""},
		},
	}
	if err := fl.Validate(); err != nil {
		t.Fatal(err)
	}
	br := &BufferRenderer{}
	r, _ := NewRunner(fl, &State{FlowID: fl.ID}, br, nil, nil)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(br.String(), "kind?") {
		t.Fatalf("entry prompt = %q, want kind?", br.String())
	}
	br.Reset()
	if _, err := r.Submit("weapon"); err != nil {
		t.Fatal(err)
	}
	// Conditional should have auto-routed to "wpn" and rendered its prompt.
	if !strings.Contains(br.String(), "weapon name?") {
		t.Errorf("after submit: rendered = %q, want weapon name?", br.String())
	}
	if r.State().Current != "wpn" {
		t.Errorf("Current = %q, want wpn", r.State().Current)
	}
}
