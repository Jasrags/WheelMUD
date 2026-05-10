package lua

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	gluua "github.com/yuin/gopher-lua"

	"github.com/Jasrags/WheelMUD/internal/scripts"
)

// Sentinel errors returned by Runner.Run. Callers (the trigger Lua
// action handler in Slice 1; dialogue / quest action handlers in
// later slices) switch on these to decide whether to count the
// failure against the per-trigger fault budget. ErrUnknownScript
// is a bookkeeping miss (the trigger references a name that the
// catalog does not contain) — we still treat it as a fault so the
// trigger auto-disables and stops repeatedly missing.
var (
	ErrUnknownScript = errors.New("lua: unknown script")
	ErrTimeout       = errors.New("lua: execution timed out")
	ErrLuaError      = errors.New("lua: runtime error")
)

// poolSize caps the number of pooled LStates. Each script holds an
// LState for the duration of one call; the pool serves concurrent
// trigger fires without re-paying the env-strip cost. 8 is the V1
// guess — tune via Runner.Stats() once we have telemetry.
const poolSize = 8

// Runner runs catalog scripts in pooled, sandboxed LStates. Safe to
// call Run concurrently — each invocation borrows its own LState.
// We manage the LState list explicitly (rather than via sync.Pool)
// so Stop can deterministically close every state and we cap total
// concurrency at poolSize. A blocking semaphore (the buffered free
// channel) makes a 9th concurrent call wait for a release rather
// than synthesizing a fresh LState that escapes our shutdown.
type Runner struct {
	cat    *scripts.Catalog
	logger *slog.Logger

	free   chan *gluua.LState // buffered up to poolSize; available LStates
	all    []*gluua.LState    // every LState ever borrowed; Stop closes them all
	allMu  sync.Mutex

	stopOnce sync.Once
	stopped  bool
	stopMu   sync.RWMutex
}

// NewRunner constructs a Runner backed by cat. logger is optional;
// nil falls back to slog.Default.
func NewRunner(cat *scripts.Catalog, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	r := &Runner{
		cat:    cat,
		logger: logger,
		free:   make(chan *gluua.LState, poolSize),
	}
	// Pre-allocate the full pool so we never have to spin up new
	// LStates under load (and so Stop knows exactly which states
	// to close).
	for i := 0; i < poolSize; i++ {
		l := NewSandboxedState()
		r.allMu.Lock()
		r.all = append(r.all, l)
		r.allMu.Unlock()
		r.free <- l
	}
	return r
}

// Stop drains the pool and closes every LState. Idempotent.
// Should be invoked from the shutdown drain BEFORE bus.Stop so any
// in-flight script invocation cancels cleanly via its parent ctx.
func (r *Runner) Stop() {
	r.stopOnce.Do(func() {
		r.stopMu.Lock()
		r.stopped = true
		r.stopMu.Unlock()
		r.allMu.Lock()
		states := r.all
		r.all = nil
		r.allMu.Unlock()
		for _, l := range states {
			l.Close()
		}
	})
}

// Run executes the catalog script named `name` with bind applied
// just before the call (so the consumer can register per-call
// globals like `say` / `emote` / `ctx`). The script body is run
// against the borrowed LState's prototype — the prototype itself
// is shared and immutable across invocations.
//
// ctx is a parent context the runner wraps with CallTimeout. A
// canceled parent (session teardown / shutdown) propagates so the
// script gets ErrTimeout immediately. The classification logic
// inspects the gopher-lua error to surface the right sentinel.
func (r *Runner) Run(ctx context.Context, name string, bind func(*gluua.LState)) error {
	r.stopMu.RLock()
	stopped := r.stopped
	r.stopMu.RUnlock()
	if stopped {
		return ErrTimeout // engine is winding down; treat as timeout
	}

	script, ok := r.cat.Get(name)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownScript, name)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()

	l, err := r.borrow(timeoutCtx)
	if err != nil {
		return fmt.Errorf("%w: borrow: %v", ErrTimeout, err)
	}
	defer r.release(l)

	// gopher-lua's SetContext propagates cancellation: a canceled
	// ctx aborts the next instruction with a runtime error. Combined
	// with SetMx this gives us two stop conditions (instruction-cap
	// + wall-clock).
	l.SetContext(timeoutCtx)
	defer l.RemoveContext()

	// bind callback registers per-call globals (say / emote / log /
	// ctx). Done after SetContext so the bound functions can access
	// the timeout ctx if they need to.
	if bind != nil {
		bind(l)
	}

	// Push the prototype as a fresh closure each call. The proto is
	// shared but the closure carries its own upvalues / pc — re-use
	// across calls would be unsafe.
	closure := l.NewFunctionFromProto(script.Compiled)
	l.Push(closure)

	if err := l.PCall(0, gluua.MultRet, nil); err != nil {
		return classify(err)
	}
	return nil
}

// classify maps a gopher-lua error onto our sentinel set. The
// upstream errors are all *ApiError values; we string-match on the
// embedded message to distinguish the cap / timeout / generic
// classes since gopher-lua doesn't expose typed sentinels for
// these. The substring checks are stable (rarely-changing literals
// in gopher-lua's source).
func classify(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "context deadline exceeded"):
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	default:
		return fmt.Errorf("%w: %v", ErrLuaError, err)
	}
}

func (r *Runner) borrow(ctx context.Context) (*gluua.LState, error) {
	select {
	case l := <-r.free:
		resetSandbox(l)
		return l, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *Runner) release(l *gluua.LState) {
	if l == nil {
		return
	}
	// Wipe per-call globals so the next borrower starts clean.
	// V1 surface: say / emote / log / ctx. V2 (Phase F #32 slice 2)
	// adds the quest table + push_mode. V3 (Phase F #32 slice 3)
	// adds apply_affect, give_item, target. Reset is cheap; keep the
	// list in lock-step with APIBindings.Bind.
	for _, name := range []string{"say", "emote", "log", "ctx", "quest", "push_mode", "apply_affect", "give_item", "target"} {
		l.SetGlobal(name, gluua.LNil)
	}
	// Best-effort return; if the runner has been stopped the
	// channel is unread and the LState has been closed already.
	select {
	case r.free <- l:
	default:
	}
}
