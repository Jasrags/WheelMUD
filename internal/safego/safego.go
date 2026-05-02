// Package safego wraps long-lived goroutines in a panic-recovery
// shim so a runaway worker logs a stack trace instead of crashing
// the whole server.
//
// Use it for goroutines whose lifetime is the server itself —
// signal handlers, accept-loop watchdogs, per-session dispatchers,
// background flush loops. Goroutines that already recover (the
// tick scheduler's per-handler dispatch, the eventbus async worker)
// can keep their inline recover() — wrapping them again is harmless
// but redundant.
//
// Spawned goroutines are intentionally not restartable from inside
// safego: a panic that survives the recover means we have a logic
// bug to fix, and a silent restart loop would hide it. Callers that
// want restart-on-failure (the scheduler's tick loop, e.g.) layer
// their own retry on top.
package safego

import (
	"log/slog"
	"runtime/debug"
)

// Go runs fn in a new goroutine with a deferred recover that logs
// the panic value, the goroutine's name, and a stack trace at
// LevelError. Returns immediately.
func Go(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("safego: goroutine panicked",
					"goroutine", name,
					"panic", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}
