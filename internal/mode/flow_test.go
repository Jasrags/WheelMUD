package mode

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/flow"
	"github.com/Jasrags/WheelMUD/telnet"
)

// waitContains polls the safeBuf for up to ~500ms waiting for the
// expected substring. net.Pipe Write blocks until Read returns, but
// the drainer goroutine's append to safeBuf happens AFTER the Read
// returns, so the test thread can observe an empty buffer if it
// checks immediately. The existing flow_test cases tolerated this
// race because subsequent Handle() calls synchronized on the next
// Write; the §O.2 resume tests assert immediately after PushMode
// with no follow-up Submit, so they need an explicit wait.
func waitContains(t *testing.T, buf *safeBuf, want string) string {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if s := buf.String(); strings.Contains(s, want) {
			return s
		}
		time.Sleep(time.Millisecond)
	}
	return buf.String()
}

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
	m, err := NewFlow(f.s, fl, actions, validators, nil, nil, onDone)
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
	if got := waitContains(t, f.captured, "Name?"); !strings.Contains(got, "Name?") {
		t.Fatalf("entry prompt missing: %q", got)
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
	if got := waitContains(t, f.captured, "Flow complete."); !strings.Contains(got, "Flow complete.") {
		t.Fatalf("completion banner missing: %q", got)
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
	if got := waitContains(t, f.captured, "Required."); !strings.Contains(got, "Required.") {
		t.Fatalf("missing validation message: %q", got)
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
	got := waitContains(t, f.captured, "Flow cancelled.")
	if !strings.Contains(got, "Flow cancelled.") {
		t.Fatalf("cancel banner missing: %q", got)
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
	if got := waitContains(t, f.captured, "Name?"); !strings.Contains(got, "Name?") {
		t.Fatalf("re-render missing prompt: %q", got)
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
	got := waitContains(t, f.captured, "/cancel")
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
	got := waitContains(t, f.captured, "Unknown flow command")
	if !strings.Contains(got, "Unknown flow command") {
		t.Fatalf("missing unknown-cmd message: %q", got)
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
	if got := waitContains(t, f.captured, "flow aborted"); !strings.Contains(got, "flow aborted") {
		t.Fatalf("missing aborted banner: %q", got)
	}
	if cur, ok := f.s.CurrentMode().(*stubMode); !ok || cur.name != "game" {
		t.Fatalf("expected stubMode game after abort, got %T", f.s.CurrentMode())
	}
}

// fakeLoader returns the seeded state once per (account, flow) key,
// recording the lookup so tests can assert it was hit.
type fakeLoader struct {
	state flow.State
	ok    bool
	err   error
	calls int
}

func (f *fakeLoader) Load(_ context.Context, _ int64, _ string) (flow.State, bool, error) {
	f.calls++
	return f.state, f.ok, f.err
}

// fakeModePersister captures Save/Delete calls.
type fakeModePersister struct {
	saves   int
	deletes int
}

func (p *fakeModePersister) Save(_ context.Context, _ *flow.State) error { p.saves++; return nil }
func (p *fakeModePersister) Delete(_ context.Context, _ int64, _ string) error {
	p.deletes++
	return nil
}

func TestFlowMode_Resume_HydratesAndRendersCurrentStep(t *testing.T) {
	f := newFlowFixture(t)
	fl := &flow.Flow{
		ID:        "test",
		Entry:     "name",
		Resumable: true,
		Steps: []flow.Step{
			&flow.TextStep{ID: "name", PromptText: "Name?", StoreAs: "name", Next: "confirm"},
			&flow.ConfirmStep{ID: "confirm", PromptText: "Commit?", OnYes: "", OnNo: ""},
		},
	}
	if err := fl.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	loader := &fakeLoader{
		state: flow.State{
			FlowID:    "test",
			AccountID: 7,
			Current:   "confirm",
			Values:    map[string]string{"name": "Moiraine"},
		},
		ok: true,
	}
	persister := &fakeModePersister{}
	m, err := NewFlow(f.s, fl, nil, nil, persister, loader.Load, nil)
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}
	if !m.IsResuming() {
		t.Fatal("NewFlow should mark resuming when loader returned a hydrated row")
	}
	if loader.calls != 1 {
		t.Fatalf("loader called %d times, want 1", loader.calls)
	}
	if err := f.s.PushMode(m); err != nil {
		t.Fatalf("push flow: %v", err)
	}
	// OnEnter should re-render the confirm step's prompt, not the
	// entry step's.
	got := waitContains(t, f.captured, "Commit?")
	if strings.Contains(got, "Name?") {
		t.Errorf("resume rendered entry prompt: %q", got)
	}
	if !strings.Contains(got, "Commit?") {
		t.Errorf("resume did not re-render confirm prompt: %q", got)
	}
}

func TestFlowMode_Resume_SkippedWhenFlowNotResumable(t *testing.T) {
	f := newFlowFixture(t)
	fl := makeDemoFlow(t) // not Resumable
	loader := &fakeLoader{
		state: flow.State{FlowID: "test", AccountID: 7, Current: "confirm"},
		ok:    true,
	}
	m, err := NewFlow(f.s, fl, nil, nil, nil, loader.Load, nil)
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}
	if m.IsResuming() {
		t.Fatal("non-resumable Flow should not resume")
	}
	if loader.calls != 0 {
		t.Fatalf("loader called for non-resumable flow: %d times", loader.calls)
	}
}

func TestFlowMode_Resume_FreshStartOnLoaderMiss(t *testing.T) {
	f := newFlowFixture(t)
	fl := &flow.Flow{
		ID:        "test",
		Entry:     "name",
		Resumable: true,
		Steps: []flow.Step{
			&flow.TextStep{ID: "name", PromptText: "Name?", Next: ""},
		},
	}
	_ = fl.Validate()
	loader := &fakeLoader{ok: false}
	m, err := NewFlow(f.s, fl, nil, nil, nil, loader.Load, nil)
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}
	if m.IsResuming() {
		t.Fatal("miss should not resume")
	}
	if err := f.s.PushMode(m); err != nil {
		t.Fatal(err)
	}
	got := waitContains(t, f.captured, "Name?")
	if !strings.Contains(got, "Name?") {
		t.Fatalf("entry prompt missing after miss: %q", got)
	}
}

func TestFlowMode_Resume_DiscardsRowOnCatalogDrift(t *testing.T) {
	f := newFlowFixture(t)
	fl := &flow.Flow{
		ID:        "test",
		Entry:     "name",
		Resumable: true,
		Steps: []flow.Step{
			&flow.TextStep{ID: "name", PromptText: "Name?", Next: ""},
		},
	}
	_ = fl.Validate()
	// Persisted state points at a step that's been removed from YAML.
	loader := &fakeLoader{
		state: flow.State{FlowID: "test", AccountID: 7, Current: "ghost"},
		ok:    true,
	}
	persister := &fakeModePersister{}
	m, err := NewFlow(f.s, fl, nil, nil, persister, loader.Load, nil)
	if err != nil {
		t.Fatalf("NewFlow: %v", err)
	}
	if m.IsResuming() {
		t.Fatal("dangling-step row should NOT resume")
	}
	if persister.deletes != 1 {
		t.Errorf("expected 1 delete to clear the orphan row, got %d", persister.deletes)
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
