// Package mode hosts the per-mode handlers that share the dispatcher's
// mode-stack contract. This file adds the §O.1 Flow adapter: a Mode
// implementation that pushes a flow.Runner onto a session and pops
// itself off on completion / abort / cancel.
//
// Slash-prefix meta-commands (`/cancel`, `/back`, `/help`) work
// uniformly across every Flow because they're parsed here before
// input reaches the Step. Bare input goes to Runner.Submit; the
// engine handles its own re-prompt on *ValidationError.
package mode

import (
	"context"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/flow"
	"github.com/Jasrags/WheelMUD/telnet"
)

// Flow wraps a *flow.Runner as a mode-stack participant. Construct
// via NewFlow; PushMode activates it. On completion or hard abort,
// Handle pops the mode off the stack so the player lands back in
// whatever was below (typically Game).
//
// Concurrency: a Flow mode is per-session — never share one across
// sessions. The underlying Runner is itself single-goroutine; all
// access happens on the dispatcher goroutine.
type Flow struct {
	runner *flow.Runner
	onDone func(*flow.State)
}

// NewFlow builds a session-bound Flow mode. `actions` and
// `validators` may be nil — in that case any step referencing a
// registry-keyed action or validator errors at Submit time with a
// clear "unregistered" message. `onDone` is invoked after a normal
// flow completion with the final State; nil is a no-op (the
// typical case for ephemeral flows like wizdemo).
func NewFlow(
	s *telnet.Session,
	fl *flow.Flow,
	actions *flow.ActionRegistry,
	validators *flow.ValidatorRegistry,
	onDone func(*flow.State),
) (*Flow, error) {
	state := &flow.State{FlowID: fl.ID, AccountID: s.AccountID}
	r, err := flow.NewRunner(fl, state, &sessionRenderer{s: s}, actions, validators)
	if err != nil {
		return nil, err
	}
	return &Flow{runner: r, onDone: onDone}, nil
}

// OnEnter activates the runner — Start() renders the entry step's
// prompt to the session. Session.PushMode rolls back on a non-nil
// return; if Start fails, the player never lands in the flow mode.
func (m *Flow) OnEnter(_ *telnet.Session) error {
	return m.runner.Start()
}

// OnExit is a no-op for now. A future slice that wants to emit a
// "back to game" banner can hook in here.
func (m *Flow) OnExit(_ *telnet.Session) error { return nil }

// Prompt returns empty — the step's prompt was already written by
// the runner during the last advance, and the dispatcher caches
// it. Returning a non-empty prompt here would double-render.
func (m *Flow) Prompt(_ context.Context, _ *telnet.Session) string { return "" }

// Handle is the dispatcher's input hook. Slash-prefixed lines are
// meta-commands handled in-place; bare input feeds the runner.
//
// Outcomes:
//   - meta `/cancel|/quit|/abort`: cancel + pop, yellow notice
//   - meta `/back`: re-render current step's prompt
//   - meta `/help`: print the meta-command cheat-sheet
//   - other `/foo`: print an "unknown" yellow notice
//   - bare input + Runner success: render handled by runner, keep mode
//   - bare input + ValidationError: runner already re-prompted, keep mode
//   - bare input + hard abort: print red "flow aborted" + pop
//   - bare input + completion: print green "Flow complete." + pop
func (m *Flow) Handle(_ context.Context, s *telnet.Session, line string) error {
	if strings.HasPrefix(line, "/") {
		return m.handleMeta(s, line)
	}
	done, err := m.runner.Submit(line)
	if err != nil {
		// Runner.Submit consumes *ValidationError internally (writes
		// the re-prompt via the renderer and returns nil). The only
		// non-nil err here is a hard abort — render a red banner with
		// the defanged error and pop. defang prevents an error
		// message containing `{{` / `}}` from breaking the cfmt span.
		_ = s.WriteString("{{flow aborted: " + display.Defang(err.Error(), "unknown") + "}}::red\r\n")
		return s.PopMode()
	}
	if done {
		if m.onDone != nil {
			m.onDone(m.runner.State())
		}
		_ = s.WriteString("{{Flow complete.}}::green\r\n")
		return s.PopMode()
	}
	return nil
}

func (m *Flow) handleMeta(s *telnet.Session, line string) error {
	cmd := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "/")))
	switch cmd {
	case "cancel", "quit", "abort":
		m.runner.Cancel()
		_ = s.WriteString("{{Flow cancelled.}}::yellow\r\n")
		return s.PopMode()
	case "back":
		// O.1: re-render current step. Full history-based back
		// navigation (with action undo) is deferred to §O.3.
		step := m.runner.CurrentStep()
		if step == nil {
			return nil
		}
		return s.WriteString(step.Prompt(m.runner.State()))
	case "help":
		return s.WriteString(
			"{{Flow controls: }}::gray{{/cancel}}::cyan{{ abort, }}::gray" +
				"{{/back}}::cyan{{ re-show prompt, }}::gray" +
				"{{/help}}::cyan{{ this message.}}::gray\r\n")
	default:
		return s.WriteString(
			"{{Unknown flow command. Try /cancel, /back, /help.}}::yellow\r\n")
	}
}

// sessionRenderer adapts a *telnet.Session to flow.Renderer. The
// engine writes already-cfmt-formatted strings; Session.WriteString
// renders + frames them in the session's prompt-aware way.
//
// Defined here (not in internal/flow) because the engine MUST stay
// transport-agnostic — `flow` has no telnet import, and the only
// bridge lives here at the mode layer.
type sessionRenderer struct{ s *telnet.Session }

func (r *sessionRenderer) Write(msg string) error { return r.s.WriteString(msg) }
