package flow

import (
	"errors"
	"strings"
	"testing"
)

func TestFlow_ValidateRejectsMalformed(t *testing.T) {
	tests := []struct {
		name    string
		flow    *Flow
		wantSub string
	}{
		{"nil flow", nil, "nil"},
		{"blank id", &Flow{Entry: "a", Steps: []Step{&TextStep{ID: "a"}}}, "blank ID"},
		{"no steps", &Flow{ID: "x", Entry: "a"}, "no steps"},
		{"nil step", &Flow{ID: "x", Entry: "a", Steps: []Step{nil}}, "is nil"},
		{"blank step id", &Flow{ID: "x", Entry: "a", Steps: []Step{&TextStep{}}}, "blank StepID"},
		{"dup step id", &Flow{ID: "x", Entry: "a",
			Steps: []Step{&TextStep{ID: "a"}, &TextStep{ID: "a"}}}, "duplicate"},
		{"missing entry", &Flow{ID: "x", Entry: "", Steps: []Step{&TextStep{ID: "a"}}}, "blank Entry"},
		{"entry not in steps", &Flow{ID: "x", Entry: "z",
			Steps: []Step{&TextStep{ID: "a"}}}, "not in Steps"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.flow.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("got %v, want substring %q", err, tc.wantSub)
			}
		})
	}
}

func TestFlow_ValidateHappy(t *testing.T) {
	f := &Flow{
		ID:    "demo",
		Entry: "name",
		Steps: []Step{
			&TextStep{ID: "name", PromptText: "Name?", StoreAs: "name", Next: ""},
		},
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if f.Step("name") == nil {
		t.Fatalf("byID lookup failed")
	}
}

func TestIsValidationError(t *testing.T) {
	if !IsValidationError(&ValidationError{Message: "x"}) {
		t.Fatal("typed VE should be recognised")
	}
	if IsValidationError(errors.New("plain")) {
		t.Fatal("plain error should not be a VE")
	}
	if IsValidationError(nil) {
		t.Fatal("nil should not be a VE")
	}
}

// runOK is a small helper that runs Submit and asserts no abort
// error. It returns done so tests can drive the flow without
// boilerplate.
func runOK(t *testing.T, r *Runner, input string) bool {
	t.Helper()
	done, err := r.Submit(input)
	if err != nil {
		t.Fatalf("Submit(%q): unexpected error: %v", input, err)
	}
	return done
}

func TestRunner_TextHappyPath(t *testing.T) {
	f := &Flow{
		ID:    "demo",
		Entry: "name",
		Steps: []Step{
			&TextStep{ID: "name", PromptText: "Name?", StoreAs: "name", Next: ""},
		},
	}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	render := &BufferRenderer{}
	r, err := NewRunner(f, nil, render, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(render.String(), "Name?") {
		t.Fatalf("entry prompt not rendered: %q", render.String())
	}
	if done := runOK(t, r, "Alice"); !done {
		t.Fatal("flow should complete after final text step")
	}
	if !r.State().Completed {
		t.Fatal("State.Completed must be true after final step")
	}
	if got := r.State().Values["name"]; got != "Alice" {
		t.Fatalf("Values[name]=%q, want Alice", got)
	}
}

func TestRunner_TextRequiredReprompts(t *testing.T) {
	f := &Flow{
		ID:    "demo",
		Entry: "name",
		Steps: []Step{
			&TextStep{ID: "name", PromptText: "Name?", StoreAs: "name", Next: ""},
		},
	}
	_ = f.Validate()
	render := &BufferRenderer{}
	r, _ := NewRunner(f, nil, render, nil, nil)
	_ = r.Start()
	render.Reset()

	done, err := r.Submit("   ")
	if err != nil {
		t.Fatalf("Submit empty: unexpected error: %v", err)
	}
	if done {
		t.Fatal("blank input should re-prompt, not complete")
	}
	if !strings.Contains(render.String(), "Required.") {
		t.Fatalf("missing required message; got %q", render.String())
	}
	if !strings.Contains(render.String(), "Name?") {
		t.Fatalf("re-prompt missing original prompt; got %q", render.String())
	}
	if r.State().Completed {
		t.Fatal("State.Completed must stay false on re-prompt")
	}
}

func TestRunner_TextValidatorRefRejected(t *testing.T) {
	f := &Flow{
		ID:    "demo",
		Entry: "name",
		Steps: []Step{
			&TextStep{ID: "name", PromptText: "Name?", Validator: "name_taken",
				StoreAs: "name", Next: ""},
		},
	}
	_ = f.Validate()
	render := &BufferRenderer{}
	val := NewValidatorRegistry()
	_ = val.Register("name_taken", func(_ *State, in string) error {
		if in == "bob" {
			return &ValidationError{Message: "Name 'bob' is taken."}
		}
		return nil
	})

	r, _ := NewRunner(f, nil, render, nil, val)
	_ = r.Start()
	render.Reset()

	if done, _ := r.Submit("bob"); done {
		t.Fatal("validator rejection should not complete the flow")
	}
	if !strings.Contains(render.String(), "is taken") {
		t.Fatalf("missing validation message; got %q", render.String())
	}
	// Recover by submitting an acceptable value.
	render.Reset()
	if !runOK(t, r, "alice") {
		t.Fatal("post-rejection submit should complete")
	}
	if got := r.State().Values["name"]; got != "alice" {
		t.Fatalf("Values[name]=%q, want alice", got)
	}
}

func TestRunner_UnregisteredValidatorAborts(t *testing.T) {
	f := &Flow{
		ID:    "demo",
		Entry: "name",
		Steps: []Step{
			&TextStep{ID: "name", Validator: "missing", PromptText: "Name?", Next: ""},
		},
	}
	_ = f.Validate()
	r, _ := NewRunner(f, nil, &BufferRenderer{}, nil, NewValidatorRegistry())
	_ = r.Start()

	_, err := r.Submit("x")
	if err == nil || !strings.Contains(err.Error(), "unregistered validator") {
		t.Fatalf("expected unregistered-validator error, got %v", err)
	}
}

func TestRunner_ActionRunsAfterHandle(t *testing.T) {
	f := &Flow{
		ID:    "demo",
		Entry: "name",
		Steps: []Step{
			&TextStep{ID: "name", PromptText: "Name?", Action: "finalize",
				StoreAs: "name", Next: ""},
		},
	}
	_ = f.Validate()
	actions := NewActionRegistry()
	called := false
	_ = actions.Register("finalize", func(s *State) error {
		called = true
		if s.Values["name"] != "alice" {
			t.Fatalf("action saw Values[name]=%q before commit", s.Values["name"])
		}
		return nil
	})
	r, _ := NewRunner(f, nil, &BufferRenderer{}, actions, nil)
	_ = r.Start()
	if !runOK(t, r, "alice") {
		t.Fatal("flow should complete")
	}
	if !called {
		t.Fatal("action never invoked")
	}
}

func TestRunner_ActionErrorAborts(t *testing.T) {
	f := &Flow{
		ID:    "demo",
		Entry: "name",
		Steps: []Step{
			&TextStep{ID: "name", Action: "boom", StoreAs: "name", Next: ""},
		},
	}
	_ = f.Validate()
	actions := NewActionRegistry()
	bang := errors.New("kaboom")
	_ = actions.Register("boom", func(*State) error { return bang })
	r, _ := NewRunner(f, nil, &BufferRenderer{}, actions, nil)
	_ = r.Start()

	_, err := r.Submit("alice")
	if err == nil || !errors.Is(err, bang) {
		t.Fatalf("expected wrapped kaboom; got %v", err)
	}
	if r.State().Completed {
		t.Fatal("Completed must stay false when an action aborts")
	}
}

func TestRunner_ChoiceBranchPerOption(t *testing.T) {
	f := &Flow{
		ID:    "demo",
		Entry: "pick",
		Steps: []Step{
			&ChoiceStep{
				ID:         "pick",
				PromptText: "Side?",
				StoreAs:    "side",
				Options: []ChoiceOption{
					{Label: "Light", Value: "light", Next: "good-end"},
					{Label: "Shadow", Value: "shadow", Next: "evil-end"},
				},
			},
			&TextStep{ID: "good-end", PromptText: "Bright.", StoreAs: "outcome", Next: ""},
			&TextStep{ID: "evil-end", PromptText: "Dark.", StoreAs: "outcome", Next: ""},
		},
	}
	_ = f.Validate()
	render := &BufferRenderer{}
	r, _ := NewRunner(f, nil, render, nil, nil)
	_ = r.Start()
	runOK(t, r, "Shadow") // label match
	if r.State().Current != "evil-end" {
		t.Fatalf("Current=%q, want evil-end", r.State().Current)
	}
	if got := r.State().Values["side"]; got != "shadow" {
		t.Fatalf("Values[side]=%q, want shadow", got)
	}
}

func TestRunner_ChoiceIndexAndUnknown(t *testing.T) {
	f := &Flow{
		ID:    "demo",
		Entry: "pick",
		Steps: []Step{
			&ChoiceStep{
				ID:         "pick",
				PromptText: "Pick.",
				StoreAs:    "v",
				Next:       "",
				Options: []ChoiceOption{
					{Label: "a", Value: "A"},
					{Label: "b", Value: "B"},
				},
			},
		},
	}
	_ = f.Validate()
	render := &BufferRenderer{}
	r, _ := NewRunner(f, nil, render, nil, nil)
	_ = r.Start()
	// Out-of-range index re-prompts.
	render.Reset()
	if done, _ := r.Submit("9"); done {
		t.Fatal("out-of-range index must re-prompt, not complete")
	}
	if !strings.Contains(render.String(), "between 1 and 2") {
		t.Fatalf("missing index range message; got %q", render.String())
	}
	// Unknown label re-prompts.
	render.Reset()
	if done, _ := r.Submit("zorglub"); done {
		t.Fatal("unknown label must re-prompt")
	}
	if !strings.Contains(render.String(), "Unknown option") {
		t.Fatalf("missing unknown-option message; got %q", render.String())
	}
	// Valid index advances.
	if !runOK(t, r, "2") {
		t.Fatal("index 2 should complete the flow")
	}
	if got := r.State().Values["v"]; got != "B" {
		t.Fatalf("Values[v]=%q, want B", got)
	}
}

func TestRunner_ConfirmRoutes(t *testing.T) {
	f := &Flow{
		ID:    "demo",
		Entry: "go",
		Steps: []Step{
			&ConfirmStep{ID: "go", PromptText: "Proceed?", OnYes: "done", OnNo: "abort"},
			&TextStep{ID: "done", PromptText: "Done.", StoreAs: "x", Next: ""},
			&TextStep{ID: "abort", PromptText: "Aborted.", StoreAs: "x", Next: ""},
		},
	}
	_ = f.Validate()
	r, _ := NewRunner(f, nil, &BufferRenderer{}, nil, nil)
	_ = r.Start()
	if done, _ := r.Submit("y"); done {
		t.Fatal("y should advance to done step, not complete the flow")
	}
	if r.State().Current != "done" {
		t.Fatalf("Current=%q, want done", r.State().Current)
	}
}

func TestRunner_ConfirmRejectsGarbage(t *testing.T) {
	f := &Flow{
		ID:    "demo",
		Entry: "go",
		Steps: []Step{
			&ConfirmStep{ID: "go", PromptText: "Sure?", OnYes: "", OnNo: ""},
		},
	}
	_ = f.Validate()
	render := &BufferRenderer{}
	r, _ := NewRunner(f, nil, render, nil, nil)
	_ = r.Start()
	render.Reset()
	if done, _ := r.Submit("maybe"); done {
		t.Fatal("garbage should re-prompt, not complete")
	}
	if !strings.Contains(render.String(), "Answer y or n") {
		t.Fatalf("missing parse-error message; got %q", render.String())
	}
}

func TestRunner_DoubleStartErrors(t *testing.T) {
	f := &Flow{
		ID:    "demo",
		Entry: "a",
		Steps: []Step{&TextStep{ID: "a", PromptText: "x", Next: ""}},
	}
	_ = f.Validate()
	r, _ := NewRunner(f, nil, &BufferRenderer{}, nil, nil)
	_ = r.Start()
	if err := r.Start(); err == nil {
		t.Fatal("second Start should error")
	}
}

func TestRunner_SubmitOnUnstartedErrors(t *testing.T) {
	f := &Flow{
		ID:    "demo",
		Entry: "a",
		Steps: []Step{&TextStep{ID: "a", PromptText: "x", Next: ""}},
	}
	_ = f.Validate()
	r, _ := NewRunner(f, nil, &BufferRenderer{}, nil, nil)
	if _, err := r.Submit("x"); err == nil {
		t.Fatal("Submit before Start should error")
	}
}

func TestRunner_NewRunnerRejectsUnvalidatedFlow(t *testing.T) {
	f := &Flow{ID: "x", Entry: "a", Steps: []Step{&TextStep{ID: "a"}}}
	// Skip Validate.
	_, err := NewRunner(f, nil, &BufferRenderer{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Validate") {
		t.Fatalf("want validate error, got %v", err)
	}
}

func TestRunner_CancelStopsFlow(t *testing.T) {
	f := &Flow{
		ID:    "demo",
		Entry: "a",
		Steps: []Step{&TextStep{ID: "a", PromptText: "x", StoreAs: "v", Next: "b"},
			&TextStep{ID: "b", PromptText: "y", StoreAs: "v2", Next: ""}},
	}
	_ = f.Validate()
	r, _ := NewRunner(f, nil, &BufferRenderer{}, nil, nil)
	_ = r.Start()
	runOK(t, r, "first")
	r.Cancel()
	if !r.State().Cancelled {
		t.Fatal("Cancelled must be true")
	}
	if _, err := r.Submit("second"); err == nil {
		t.Fatal("Submit on cancelled flow should error")
	}
}

func TestActionRegistry_DuplicateRejected(t *testing.T) {
	r := NewActionRegistry()
	if err := r.Register("k", func(*State) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("k", func(*State) error { return nil }); err == nil {
		t.Fatal("duplicate key should error")
	}
}

func TestActionRegistry_BlankAndNilRejected(t *testing.T) {
	r := NewActionRegistry()
	if err := r.Register("", func(*State) error { return nil }); err == nil {
		t.Fatal("blank key should error")
	}
	if err := r.Register("k", nil); err == nil {
		t.Fatal("nil fn should error")
	}
}

func TestValidatorRegistry_DuplicateRejected(t *testing.T) {
	r := NewValidatorRegistry()
	if err := r.Register("k", func(*State, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("k", func(*State, string) error { return nil }); err == nil {
		t.Fatal("duplicate key should error")
	}
}

func TestRegistries_KeysSorted(t *testing.T) {
	r := NewActionRegistry()
	for _, k := range []string{"zeta", "alpha", "middle"} {
		_ = r.Register(k, func(*State) error { return nil })
	}
	got := r.Keys()
	if len(got) != 3 || got[0] != "alpha" || got[1] != "middle" || got[2] != "zeta" {
		t.Fatalf("Keys not sorted: %v", got)
	}
}

func TestRunner_FullThreeStepFlowEndToEnd(t *testing.T) {
	// A canonical mini-flow: pick name → choose race → confirm.
	// Exercises every concept in the slice 1 surface: text + validator
	// (length check), choice + branching, confirm + action invocation.
	committed := false

	val := NewValidatorRegistry()
	_ = val.Register("min3", func(_ *State, in string) error {
		if len(strings.TrimSpace(in)) < 3 {
			return &ValidationError{Message: "Need at least 3 characters."}
		}
		return nil
	})
	act := NewActionRegistry()
	_ = act.Register("commit", func(s *State) error {
		committed = true
		if s.Values["name"] == "" || s.Values["race"] == "" {
			t.Fatalf("commit saw incomplete state: %+v", s.Values)
		}
		return nil
	})

	f := &Flow{
		ID:    "demo",
		Entry: "name",
		Steps: []Step{
			&TextStep{ID: "name", PromptText: "Name?", Validator: "min3", StoreAs: "name", Next: "race"},
			&ChoiceStep{ID: "race", PromptText: "Race?", StoreAs: "race", Next: "go",
				Options: []ChoiceOption{
					{Label: "Aiel", Value: "aiel"},
					{Label: "Ogier", Value: "ogier"},
				}},
			&ConfirmStep{ID: "go", PromptText: "Commit?", Action: "commit", OnYes: ""},
		},
	}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	r, _ := NewRunner(f, nil, &BufferRenderer{}, act, val)
	_ = r.Start()

	// Validator rejects too-short name.
	if done, _ := r.Submit("Al"); done {
		t.Fatal("short name should re-prompt")
	}
	runOK(t, r, "Alice")
	runOK(t, r, "1") // index match for Aiel
	if !runOK(t, r, "y") {
		t.Fatal("y on the confirm step should complete the flow")
	}
	if !committed {
		t.Fatal("commit action never ran")
	}
	if !r.State().Completed {
		t.Fatal("State.Completed must be true after confirm-yes")
	}
}
