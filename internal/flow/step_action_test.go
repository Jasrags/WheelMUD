package flow

import (
	"errors"
	"strings"
	"testing"
)

// stubRuntime is the minimal AutoRuntime needed for ActionStep tests.
type stubRuntime struct {
	actions *ActionRegistry
}

func (s *stubRuntime) Actions() *ActionRegistry { return s.actions }

func TestActionStep_Auto_Success(t *testing.T) {
	reg := NewActionRegistry()
	fired := false
	if err := reg.Register("commit", func(s *State) error { fired = true; return nil }); err != nil {
		t.Fatal(err)
	}
	step := &ActionStep{ID: "save", Action: "commit", Next: "after"}
	got, err := step.Auto(&stubRuntime{actions: reg}, &State{})
	if err != nil {
		t.Fatalf("Auto: %v", err)
	}
	if !fired {
		t.Error("action not invoked")
	}
	if got != "after" {
		t.Errorf("next = %q, want after", got)
	}
}

func TestActionStep_Auto_UnregisteredAction(t *testing.T) {
	reg := NewActionRegistry()
	step := &ActionStep{ID: "save", Action: "missing", Next: "after"}
	_, err := step.Auto(&stubRuntime{actions: reg}, &State{})
	if err == nil || !strings.Contains(err.Error(), "unregistered") {
		t.Fatalf("err = %v, want unregistered", err)
	}
}

func TestActionStep_Auto_ActionFails(t *testing.T) {
	reg := NewActionRegistry()
	sentinel := errors.New("disk full")
	_ = reg.Register("commit", func(s *State) error { return sentinel })
	step := &ActionStep{ID: "save", Action: "commit", Next: "after"}
	_, err := step.Auto(&stubRuntime{actions: reg}, &State{})
	if err == nil || !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wrapping %v", err, sentinel)
	}
}

func TestActionStep_Auto_BlankActionErrors(t *testing.T) {
	step := &ActionStep{ID: "save", Next: "after"}
	_, err := step.Auto(&stubRuntime{actions: NewActionRegistry()}, &State{})
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("err = %v, want Action required", err)
	}
}

func TestActionStep_HandleIsDefensive(t *testing.T) {
	step := &ActionStep{ID: "save"}
	_, err := step.Handle(&State{}, "irrelevant")
	if err == nil {
		t.Fatal("Handle on no-prompt step should error")
	}
}
