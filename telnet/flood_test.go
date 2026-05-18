package telnet

import (
	"testing"
	"time"
)

func TestFloodGate_NilAlwaysAllows(t *testing.T) {
	var g *floodGate
	if !g.Allow(1<<20) || !g.Allow(0) {
		t.Error("nil gate should always allow")
	}
}

func TestNewFloodGate_ZeroOrNegativeDisabled(t *testing.T) {
	if g := NewFloodGate(0, 1024); g != nil {
		t.Error("zero rate should produce nil gate")
	}
	if g := NewFloodGate(1024, 0); g != nil {
		t.Error("zero burst should produce nil gate")
	}
	if g := NewFloodGate(-1, 1024); g != nil {
		t.Error("negative rate should produce nil gate")
	}
}

func TestFloodGate_BurstThenDrop(t *testing.T) {
	g := NewFloodGate(100 /*B/s*/, 500 /*burst*/)
	now := time.Unix(0, 0)
	g.clock = func() time.Time { return now }
	// Reset bookkeeping so test time starts at 0 with full burst.
	g.last = now

	// Burst budget = 500B. Five 100-byte writes drain it.
	for i := 0; i < 5; i++ {
		if !g.Allow(100) {
			t.Fatalf("write %d should fit in burst", i+1)
		}
	}
	// 6th write at the same instant has 0 tokens left → drop.
	if g.Allow(100) {
		t.Error("write past burst should drop")
	}
}

func TestFloodGate_RefillsOverTime(t *testing.T) {
	g := NewFloodGate(100 /*B/s*/, 200)
	now := time.Unix(0, 0)
	g.clock = func() time.Time { return now }
	g.last = now
	g.tokens = 0 // start empty so refill is the only signal

	if g.Allow(50) {
		t.Fatal("empty bucket should drop")
	}
	// Advance 1 second → 100 tokens refilled.
	now = now.Add(time.Second)
	if !g.Allow(50) {
		t.Error("after 1s refill, 50B should fit")
	}
	// 50B remaining of the refilled 100; advance another 0.4s → 40 more.
	now = now.Add(400 * time.Millisecond)
	if !g.Allow(90) {
		t.Error("after partial refill, 90B should fit")
	}
	// Bucket should be empty-ish now.
	if g.Allow(50) {
		t.Error("drained bucket should drop")
	}
}

func TestFloodGate_BurstCapped(t *testing.T) {
	g := NewFloodGate(100, 200)
	now := time.Unix(0, 0)
	g.clock = func() time.Time { return now }
	g.last = now

	// Idle for 10s — without capping, refill would be 1000 tokens.
	now = now.Add(10 * time.Second)
	// Burst cap is 200, so we should be able to write 200 at once but no more.
	if !g.Allow(200) {
		t.Fatal("burst-sized write after idle should fit")
	}
	if g.Allow(1) {
		t.Error("bucket should be capped at burst, not over-refilled")
	}
}
