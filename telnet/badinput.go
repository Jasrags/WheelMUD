package telnet

import (
	"sync"
	"time"
)

// BadInputTracker counts unknown / refused command attempts per
// session and throttles further responses after a configurable burst.
// Closes the privilege-enumeration timing channel by guaranteeing the
// dispatcher does the same constant-time work for both `ErrUnknownCommand`
// (verb doesn't exist) and AuthLevel-denial (verb exists but caller
// isn't privileged) before writing the identical "Unknown command"
// response.
//
// Zero-value is a no-op tracker — Record returns true (allow) every
// call, so callers don't need a nil guard. Use NewBadInputTracker to
// turn on rate limiting.
type BadInputTracker struct {
	mu         sync.Mutex
	entries    map[*Session]*badInputEntry
	window     time.Duration
	maxInBurst int
	clock      func() time.Time
}

// badInputEntry tracks one session's rolling bad-input count. The
// first hit anchors the window; subsequent hits within `window` bump
// `count`. Once the window expires, the next hit resets the anchor.
type badInputEntry struct {
	firstAt time.Time
	count   int
}

// NewBadInputTracker constructs a tracker that allows up to
// maxInBurst bad inputs from a given session inside `window` before
// silently dropping further responses. Sensible defaults: window =
// 30s, maxInBurst = 20.
//
// A zero or negative maxInBurst disables throttling but keeps the
// telemetry side-effect (counter still bumps so future consumers
// can attach a metric). A zero window collapses to "any past hit
// counts" — operators should always pass a positive window in real
// use.
func NewBadInputTracker(window time.Duration, maxInBurst int) *BadInputTracker {
	return &BadInputTracker{
		entries:    make(map[*Session]*badInputEntry),
		window:     window,
		maxInBurst: maxInBurst,
		clock:      time.Now,
	}
}

// Record bumps the per-session bad-input counter and returns whether
// the dispatcher should still write its response. Returns true (allow)
// on a nil tracker so the call site stays a single statement.
//
// Semantics:
//   - First hit of a window: count := 1, firstAt := now, allow.
//   - Hit within window, count <= maxInBurst: count++, allow.
//   - Hit within window, count > maxInBurst: drop (return false).
//   - Hit after window expires: anchor a fresh window, allow.
//
// The drop is silent: the dispatcher should still consume the input
// and return nil (no error to surface) so the connection stays open.
// Logging the throttle event is the caller's responsibility.
func (t *BadInputTracker) Record(s *Session) bool {
	if t == nil || s == nil {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.clock()
	e, ok := t.entries[s]
	if !ok || (t.window > 0 && now.Sub(e.firstAt) > t.window) {
		t.entries[s] = &badInputEntry{firstAt: now, count: 1}
		return true
	}
	e.count++
	if t.maxInBurst <= 0 {
		return true
	}
	if e.count > t.maxInBurst {
		return false
	}
	return true
}

// Forget removes the per-session entry. Called from the dispatcher
// when a session ends so the tracker doesn't leak entries for
// disconnected clients. Cheap to call unconditionally; a no-op for a
// session that never tripped the tracker.
func (t *BadInputTracker) Forget(s *Session) {
	if t == nil || s == nil {
		return
	}
	t.mu.Lock()
	delete(t.entries, s)
	t.mu.Unlock()
}
