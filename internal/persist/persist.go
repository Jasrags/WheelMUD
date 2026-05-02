// Package persist coordinates periodic + shutdown saves across the
// server. It is the seam between subsystems that produce mutable
// state (combat, regen, weave-resolution, session presence) and the
// repos that durably store it.
//
// Today most state is already write-through: every mutation goes
// straight to a repo. The Manager exists to host the savers for
// state that is NOT write-through — combat HP that ticks faster
// than we want to round-trip to disk, idle/play-time counters,
// dirty-bit batches. Each subsystem registers a name + flush
// function; the tick scheduler's `save` bucket calls FlushAll
// every 30s, and srv.shutdown() runs one final pass before the
// drain budget expires.
//
// Savers must be idempotent and bounded: a single FlushAll cycle
// runs every saver in series, and a slow saver delays the rest.
// Anything bigger than a couple seconds belongs in a worker queue.
package persist

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// SaverFunc flushes one subsystem's accumulated state to its repos.
// Returning an error logs but does not abort the FlushAll cycle —
// other savers still run, and the next tick retries.
type SaverFunc func(ctx context.Context) error

// Manager owns the registered savers and runs them on demand.
// Construct via New, register at startup, call FlushAll from a
// tick bucket and from shutdown.
type Manager struct {
	mu     sync.Mutex
	savers []namedSaver
}

type namedSaver struct {
	name string
	fn   SaverFunc
}

// New returns an empty Manager.
func New() *Manager {
	return &Manager{}
}

// Register adds a saver under the given name. Names appear in slog
// fields and are intended for ops/debugging; duplicate names are
// allowed but discouraged. Registration is one-shot at startup.
func (m *Manager) Register(name string, fn SaverFunc) {
	if fn == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.savers = append(m.savers, namedSaver{name: name, fn: fn})
}

// FlushAll runs every registered saver in registration order. Each
// saver gets the same ctx so a deadline (e.g. shutdown's 10s drain
// budget) is honored. Logs the total elapsed time at Debug, plus
// any individual error at Warn.
//
// Safe to call concurrently with Register, though startup is
// expected to register everything before the first FlushAll.
func (m *Manager) FlushAll(ctx context.Context) {
	m.mu.Lock()
	savers := append([]namedSaver(nil), m.savers...)
	m.mu.Unlock()

	start := time.Now()
	for _, s := range savers {
		if ctx.Err() != nil {
			slog.Warn("persist: aborting flush, context done",
				"after", s.name, "elapsed", time.Since(start))
			return
		}
		if err := s.fn(ctx); err != nil {
			slog.Warn("persist: saver failed", "saver", s.name, "error", err)
		}
	}
	slog.Debug("persist: flush complete", "savers", len(savers), "elapsed", time.Since(start))
}
