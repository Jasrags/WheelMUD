package telnet

import (
	"sync"
	"time"
)

// floodGate is a per-session token bucket measuring outbound bytes.
// Tokens are bytes; the bucket holds up to `burst` bytes and refills
// at `rate` bytes per second. WriteRaw consults the gate before
// hitting the kernel: when there aren't enough tokens, the write is
// silently dropped at the gate. This bounds the damage from a runaway
// script (or a malicious peer-triggered amplification) without
// affecting normal-paced output.
//
// Drop-silent is the configured policy (per §M.2 plan): a connected
// player who legitimately runs into the cap loses scrollback lines
// but stays connected, and the next allowed write resumes cleanly.
// The dispatcher's own prompt/command echoes go through WriteRaw too;
// dropping a prompt isn't catastrophic — the client redraws on the
// next input — but if the cap is tuned too tight, an operator will
// see Debug-level "flood: dropped" log entries.
//
// Zero-value floodGate (gate == nil at the Session level) means
// "unlimited" — Session.WriteRaw skips the check. The standard config
// path runs through NewFloodGate at Session construction.
type floodGate struct {
	mu     sync.Mutex
	tokens float64 // current bytes-budget
	max    float64 // burst capacity (also initial fill)
	rate   float64 // bytes per second
	last   time.Time
	clock  func() time.Time
}

// NewFloodGate constructs a gate refilling at `rate` bytes per second
// with a `burst` capacity (initial fill = burst). A non-positive rate
// or burst disables the gate (Allow returns true for any size).
func NewFloodGate(rate, burst int) *floodGate {
	if rate <= 0 || burst <= 0 {
		return nil
	}
	now := time.Now()
	return &floodGate{
		tokens: float64(burst),
		max:    float64(burst),
		rate:   float64(rate),
		last:   now,
		clock:  time.Now,
	}
}

// Allow consumes n bytes from the bucket. Returns true when the bucket
// had enough tokens (and they were deducted); false when the write
// should be dropped. A nil gate (the default-construction sentinel)
// always allows so call sites stay a single statement.
//
// Tokens refill linearly between calls. The bucket never overfills
// past `max`. Sub-byte fractional tokens accumulate so a slow trickle
// of small writes doesn't lose bytes to rounding.
func (g *floodGate) Allow(n int) bool {
	if g == nil || n <= 0 {
		return true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.clock()
	elapsed := now.Sub(g.last).Seconds()
	if elapsed > 0 {
		g.tokens += elapsed * g.rate
		if g.tokens > g.max {
			g.tokens = g.max
		}
		g.last = now
	}
	if g.tokens < float64(n) {
		return false
	}
	g.tokens -= float64(n)
	return true
}
