package main

import (
	"errors"
	"log/slog"
	"time"

	"github.com/Jasrags/WheelMUD/internal/cmd"
	"github.com/Jasrags/WheelMUD/internal/safego"
)

// RequestShutdown schedules a graceful shutdown after delay, with an
// optional reason broadcast to all sessions during the countdown.
// Returns cmd.ErrShutdownPending if a countdown is already in flight.
//
// Implements cmd.ShutdownController.
func (srv *server) RequestShutdown(reason string, delay time.Duration) error {
	return srv.scheduleStop(reason, delay, false)
}

// RequestReboot is RequestShutdown plus rebootOnExit, so main()
// re-execs the binary after the drain/flush sequence.
//
// Implements cmd.ShutdownController.
func (srv *server) RequestReboot(reason string, delay time.Duration) error {
	return srv.scheduleStop(reason, delay, true)
}

// RequestAbort cancels an in-flight countdown. Returns an error
// (not nil-terminated) if no countdown is pending so the operator
// gets explicit feedback rather than silent acceptance.
//
// Implements cmd.ShutdownController.
func (srv *server) RequestAbort() error {
	srv.shutdownMu.Lock()
	if !srv.shutdownPending {
		srv.shutdownMu.Unlock()
		return errors.New("no shutdown pending")
	}
	cancel := srv.shutdownCancel
	srv.shutdownPending = false
	srv.shutdownCancel = nil
	srv.rebootOnExit.Store(false)
	srv.shutdownMu.Unlock()

	close(cancel)
	srv.broadcast("{{*** Shutdown cancelled. ***}}::green")
	return nil
}

func (srv *server) scheduleStop(reason string, delay time.Duration, reboot bool) error {
	srv.shutdownMu.Lock()
	if srv.shutdownPending {
		srv.shutdownMu.Unlock()
		return cmd.ErrShutdownPending
	}
	cancel := make(chan struct{})
	srv.shutdownPending = true
	srv.shutdownCancel = cancel
	// Stamp rebootOnExit while still holding shutdownMu so a racing
	// RequestAbort that runs between this unlock and the goroutine
	// spawn cannot leave a stale `true` set after the abort cleared
	// it.
	if reboot {
		srv.rebootOnExit.Store(true)
	}
	srv.shutdownMu.Unlock()

	verb := "Shutdown"
	if reboot {
		verb = "Reboot"
	}
	srv.announceCountdownStart(verb, reason, delay)

	safego.Go("shutdown-countdown", func() {
		srv.runCountdown(verb, reason, delay, cancel)
	})
	return nil
}

func (srv *server) announceCountdownStart(verb, reason string, delay time.Duration) {
	msg := verb + " in " + delay.Round(time.Second).String() + "."
	if reason != "" {
		msg = verb + " in " + delay.Round(time.Second).String() + ": " + reason
	}
	srv.broadcast("{{*** " + msg + " ***}}::yellow")
}

// runCountdown sleeps the remaining delay in chunks, broadcasting at
// the standard {60,30,10,5..0}s marks. Returns early if cancel fires.
// On natural completion it triggers stopSignal, which feeds into the
// existing shutdown-watcher path.
func (srv *server) runCountdown(verb, reason string, delay time.Duration, cancel <-chan struct{}) {
	marks := []time.Duration{
		60 * time.Second, 30 * time.Second, 10 * time.Second,
		5 * time.Second, 4 * time.Second, 3 * time.Second,
		2 * time.Second, 1 * time.Second,
	}
	deadline := time.Now().Add(delay)

	for _, m := range marks {
		if m >= delay {
			continue
		}
		wait := time.Until(deadline.Add(-m))
		if wait <= 0 {
			continue
		}
		select {
		case <-time.After(wait):
		case <-cancel:
			return
		}
		tag := verb + " in " + m.String() + "."
		if reason != "" {
			tag = verb + " in " + m.String() + ": " + reason
		}
		srv.broadcast("{{*** " + tag + " ***}}::yellow")
	}

	// Sleep any remaining tail to the deadline.
	if rem := time.Until(deadline); rem > 0 {
		select {
		case <-time.After(rem):
		case <-cancel:
			return
		}
	}

	srv.broadcast("{{*** " + verb + " now. ***}}::red")

	// Mark the request as no longer pending before triggering the
	// stop signal. The pending guard is only there to reject a
	// second concurrent shutdown, not to gate teardown.
	srv.shutdownMu.Lock()
	srv.shutdownPending = false
	srv.shutdownCancel = nil
	srv.shutdownMu.Unlock()

	if srv.stopSignal != nil {
		srv.stopSignal()
	}
}

// broadcast sends msg to every live session via WriteAsync (the only
// safe cross-session write path; see CLAUDE.md). Failures are logged
// at Debug — a closed connection is not a coordinator-level error.
func (srv *server) broadcast(msg string) {
	for _, s := range srv.sessions.Snapshot() {
		if err := s.WriteAsync(msg); err != nil {
			slog.Debug("shutdown broadcast: write failed",
				"session", s.RemoteAddress, "error", err)
		}
	}
}
