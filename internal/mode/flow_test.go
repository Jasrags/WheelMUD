package mode

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/flow"
	"github.com/Jasrags/WheelMUD/telnet"
)

// flowFixture builds a session + drained peer + a basic flow with
// optional registries. Tests call enter() with whatever flow shape
// they want to exercise.
type flowFixture struct {
	s        *telnet.Session
	captured *safeBuf
}

func newFlowFixture(t *testing.T) *flowFixture {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })
	captured := &safeBuf{}
	// drainPeer with the 500ms idle deadline exits its goroutine
	// when the test pauses between writes — fine for fast suites,
	// flaky under -race. Use a deadline-less drain that only exits
	// when the peer closes (at t.Cleanup).
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := client.Read(buf)
			if n > 0 {
				captured.write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	s := telnet.NewSession(server)
	s.AccountID = 7
	s.CharacterID = 1
	s.CharacterName = "Tester"
	s.AuthLevel = telnet.AuthPlayer

	// Stand-in game mode underneath so PopMode lands us back in a
	// real (no-op) mode rather than the empty-stack edge case.
	if err := s.PushMode(&stubMode{name: "game"}); err != nil {
		t.Fatalf("seed game push: %v", err)
	}
	return &flowFixture{s: s, captured: captured}
}

func (f *flowFixture) enter(t *testing.T, fl *flow.Flow,
	actions *flow.ActionRegistry, validators *flow.ValidatorRegistry,
	onDone func(*flow.State)) *Flow {
	t.Helper()
	m, err := NewFlow(f.s, fl, actions, validators, onDone)
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}
	if err := f.s.PushMode(m); err != nil {
		t.Fatalf("push flow: %v", err)
	}
	return m
}

// makeDemoFlow returns a small flow: name (text required) → confirm (y/n).
func makeDemoFlow(t *testing.T) *flow.Flow {
	t.Helper()
	fl := &flow.Flow{
		ID:    "test",
		Entry: "name",
		Steps: []flow.Step{
			&flow.TextStep{ID: "name", PromptText: "Name?", StoreAs: "name", Next: "confirm"},
			&flow.ConfirmStep{ID: "confirm", PromptText: "Commit?", OnYes: "", OnNo: ""},
		},
	}
	if err := fl.Validate(); err != nil {
		t.Fatalf("flow validate: %v", err)
	}
	return fl
}

func TestFlowMode_HappyPathCompletesAndPops(t *testing.T) {
	f := newFlowFixture(t)
	fl := makeDemoFlow(t)
	committed := false
	m := f.enter(t, fl, nil, nil, func(s *flow.State) { committed = true })
	ctx := context.Background()

	// Entry prompt rendered by OnEnter.
	if !strings.Contains(f.captured.String(), "Name?") {
		t.Fatalf("entry prompt missing: %q", f.captured.String())
	}

	// Submit each step's input.
	if err := m.Handle(ctx, f.s, "Alice"); err != nil {
		t.Fatalf("Handle name: %v", err)
	}
	if err := m.Handle(ctx, f.s, "y"); err != nil {
		t.Fatalf("Handle confirm: %v", err)
	}

	if !committed {
		t.Fatal("onDone never invoked")
	}
	if !strings.Contains(f.captured.String(), "Flow complete.") {
		t.Fatalf("completion banner missing: %q", f.captured.String())
	}
	// Stack popped — game mode is back on top.
	if cur, ok := f.s.CurrentMode().(*stubMode); !ok || cur.name != "game" {
		t.Fatalf("CurrentMode = %T %+v, want stubMode 'game'", f.s.CurrentMode(), f.s.CurrentMode())
	}
}

func TestFlowMode_ValidationErrorReprompts(t *testing.T) {
	f := newFlowFixture(t)
	fl := &flow.Flow{
		ID:    "test",
		Entry: "name",
		Steps: []flow.Step{
			// Default TextStep AllowEmpty=false → blank input triggers
			// the engine's Required. re-prompt path.
			&flow.TextStep{ID: "name", PromptText: "Name?", Next: ""},
		},
	}
	_ = fl.Validate()
	m := f.enter(t, fl, nil, nil, nil)
	ctx := context.Background()
	f.captured.Reset()

	if err := m.Handle(ctx, f.s, "   "); err != nil {
		t.Fatalf("Handle whitespace: unexpected mode error: %v", err)
	}
	// Re-prompt happened via the engine; Required. + Name? both visible.
	if !strings.Contains(f.captured.String(), "Required.") {
		t.Fatalf("missing validation message: %q", f.captured.String())
	}
	// Mode did NOT pop — still on the flow mode.
	if _, ok := f.s.CurrentMode().(*Flow); !ok {
		t.Fatalf("CurrentMode = %T, want *Flow (re-prompt should keep mode)", f.s.CurrentMode())
	}
}

func TestFlowMode_CancelPops(t *testing.T) {
	f := newFlowFixture(t)
	fl := makeDemoFlow(t)
	m := f.enter(t, fl, nil, nil, nil)
	ctx := context.Background()
	f.captured.Reset()

	if err := m.Handle(ctx, f.s, "/cancel"); err != nil {
		t.Fatalf("Handle /cancel: %v", err)
	}
	if !strings.Contains(f.captured.String(), "Flow cancelled.") {
		t.Fatalf("cancel banner missing: %q", f.captured.String())
	}
	if cur, ok := f.s.CurrentMode().(*stubMode); !ok || cur.name != "game" {
		t.Fatalf("expected stubMode game after cancel, got %T", f.s.CurrentMode())
	}
}

func TestFlowMode_BackRerendersCurrentStep(t *testing.T) {
	f := newFlowFixture(t)
	fl := makeDemoFlow(t)
	m := f.enter(t, fl, nil, nil, nil)
	ctx := context.Background()
	f.captured.Reset()

	if err := m.Handle(ctx, f.s, "/back"); err != nil {
		t.Fatalf("Handle /back: %v", err)
	}
	if !strings.Contains(f.captured.String(), "Name?") {
		t.Fatalf("re-render missing prompt: %q", f.captured.String())
	}
	// Mode stays put.
	if _, ok := f.s.CurrentMode().(*Flow); !ok {
		t.Fatal("CurrentMode should remain *Flow after /back")
	}
}

func TestFlowMode_HelpDisplaysCheatSheet(t *testing.T) {
	f := newFlowFixture(t)
	fl := makeDemoFlow(t)
	m := f.enter(t, fl, nil, nil, nil)
	ctx := context.Background()
	f.captured.Reset()

	if err := m.Handle(ctx, f.s, "/help"); err != nil {
		t.Fatalf("Handle /help: %v", err)
	}
	got := f.captured.String()
	for _, want := range []string{"/cancel", "/back", "/help"} {
		if !strings.Contains(got, want) {
			t.Errorf("/help output missing %q in %q", want, got)
		}
	}
}

func TestFlowMode_UnknownSlashCommand(t *testing.T) {
	f := newFlowFixture(t)
	fl := makeDemoFlow(t)
	m := f.enter(t, fl, nil, nil, nil)
	ctx := context.Background()
	f.captured.Reset()

	if err := m.Handle(ctx, f.s, "/garbage"); err != nil {
		t.Fatalf("Handle /garbage: %v", err)
	}
	if !strings.Contains(f.captured.String(), "Unknown flow command") {
		t.Fatalf("missing unknown-cmd message: %q", f.captured.String())
	}
	if _, ok := f.s.CurrentMode().(*Flow); !ok {
		t.Fatal("CurrentMode should remain *Flow after /garbage")
	}
}

func TestFlowMode_HardAbortPopsRed(t *testing.T) {
	// A step that references an unregistered validator triggers a
	// hard abort from the runner. The mode must surface the message
	// and pop the stack.
	f := newFlowFixture(t)
	fl := &flow.Flow{
		ID:    "x",
		Entry: "name",
		Steps: []flow.Step{
			&flow.TextStep{ID: "name", PromptText: "Name?", Validator: "missing", Next: ""},
		},
	}
	_ = fl.Validate()
	m := f.enter(t, fl, nil, flow.NewValidatorRegistry(), nil)
	ctx := context.Background()
	f.captured.Reset()

	if err := m.Handle(ctx, f.s, "alice"); err != nil {
		t.Fatalf("Handle: unexpected mode error: %v", err)
	}
	if !strings.Contains(f.captured.String(), "flow aborted") {
		t.Fatalf("missing aborted banner: %q", f.captured.String())
	}
	if cur, ok := f.s.CurrentMode().(*stubMode); !ok || cur.name != "game" {
		t.Fatalf("expected stubMode game after abort, got %T", f.s.CurrentMode())
	}
}

func TestFlowMode_OnDoneOptional(t *testing.T) {
	// onDone=nil must not crash the runner.
	f := newFlowFixture(t)
	fl := makeDemoFlow(t)
	m := f.enter(t, fl, nil, nil, nil)
	ctx := context.Background()
	_ = m.Handle(ctx, f.s, "Alice")
	if err := m.Handle(ctx, f.s, "y"); err != nil {
		t.Fatalf("Handle confirm: %v", err)
	}
}
