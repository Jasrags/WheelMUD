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
	"log/slog"
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
	runner   *flow.Runner
	onDone   func(*flow.State)
	resuming bool // §O.2: skip Start and Resume() on OnEnter instead
}

// FlowLoader is the optional storage hook NewFlow calls before
// Start to attempt resume. Returns ok=false when no row exists
// (start fresh); any non-nil err is a soft failure — NewFlow logs
// and starts fresh rather than aborting. Implemented at the call
// site as a thin wrapper over repo.FlowStateRepo.Load.
type FlowLoader func(ctx context.Context, accountID int64, flowID string) (state flow.State, ok bool, err error)

// NewFlow builds a session-bound Flow mode. `actions` and
// `validators` may be nil — in that case any step referencing a
// registry-keyed action or validator errors at Submit time with a
// clear "unregistered" message. `onDone` is invoked after a normal
// flow completion with the final State; nil is a no-op (the
// typical case for ephemeral flows like wizdemo).
//
// `persister` is the §O.2 storage hook. When non-nil AND the Flow is
// `Resumable`, NewFlow attempts to load a prior state for
// (s.AccountID, fl.ID); on hit the runner's state is hydrated and
// OnEnter invokes Runner.Resume() instead of Start(). On miss the
// flow starts fresh. nil persister behaves identically to the §O.1
// in-memory path.
func NewFlow(
	s *telnet.Session,
	fl *flow.Flow,
	actions *flow.ActionRegistry,
	validators *flow.ValidatorRegistry,
	persister flow.Persister,
	loader FlowLoader,
	onDone func(*flow.State),
) (*Flow, error) {
	state := &flow.State{FlowID: fl.ID, AccountID: s.AccountID}
	resuming := false
	if fl.Resumable && loader != nil && s.AccountID != 0 {
		loaded, ok, err := loader(context.Background(), s.AccountID, fl.ID)
		if err != nil {
			// Hard read error — log and proceed fresh rather than
			// abort flow init. A bad row can't permanently lock a
			// player out of starting a new flow instance.
			slog.Warn("flow: resume load failed; starting fresh",
				"flow", fl.ID, "account", s.AccountID, "error", err)
		} else if ok && loaded.Current != "" && !loaded.Completed && !loaded.Cancelled {
			// Verify the persisted step still exists in the YAML.
			// Catalog drift (step renamed/removed) drops the row.
			if fl.Step(loaded.Current) != nil {
				*state = loaded
				state.FlowID = fl.ID
				state.AccountID = s.AccountID
				resuming = true
			} else if persister != nil {
				slog.Warn("flow: resume current step missing from catalog; discarding",
					"flow", fl.ID, "account", s.AccountID, "step", loaded.Current)
				_ = persister.Delete(context.Background(), s.AccountID, fl.ID)
			}
		}
	}
	r, err := flow.NewRunner(fl, state, &sessionRenderer{s: s}, actions, validators)
	if err != nil {
		return nil, err
	}
	r.SetPersister(persister)
	return &Flow{runner: r, onDone: onDone, resuming: resuming}, nil
}

// IsResuming reports whether NewFlow hydrated state from a prior
// session. Test-only — production callers don't branch on this.
func (m *Flow) IsResuming() bool { return m.resuming }

// OnEnter activates the runner — Start() renders the entry step's
// prompt, or Resume() re-renders the current step when this Flow
// was constructed against a hydrated state.
func (m *Flow) OnEnter(_ *telnet.Session) error {
	if m.resuming {
		return m.runner.Resume()
	}
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
