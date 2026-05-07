package tick

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestBucketFiresAtCadence(t *testing.T) {
	s, clk := newTestScheduler(t)
	b := NewBucket(s, "combat", 100*time.Millisecond)

	var calls atomic.Int32
	b.Subscribe(func(context.Context) { calls.Add(1) })

	clk.Advance(100 * time.Millisecond)
	waitFor(t, func() bool { return calls.Load() == 1 }, "first pulse")

	clk.Advance(100 * time.Millisecond)
	waitFor(t, func() bool { return calls.Load() == 2 }, "second pulse")
}

func TestBucketFanOut(t *testing.T) {
	s, clk := newTestScheduler(t)
	b := NewBucket(s, "regen", 50*time.Millisecond)

	var a, c atomic.Int32
	b.Subscribe(func(context.Context) { a.Add(1) })
	b.Subscribe(func(context.Context) { c.Add(1) })

	clk.Advance(50 * time.Millisecond)
	waitFor(t, func() bool { return a.Load() == 1 && c.Load() == 1 }, "both subs fired")
}

func TestBucketUnsubscribe(t *testing.T) {
	s, clk := newTestScheduler(t)
	b := NewBucket(s, "areaReset", 50*time.Millisecond)

	var calls atomic.Int32
	cancel := b.Subscribe(func(context.Context) { calls.Add(1) })
	cancel()

	clk.Advance(100 * time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatalf("cancelled handler fired: %d", calls.Load())
	}
}

func TestBucketStopHaltsPulses(t *testing.T) {
	s, clk := newTestScheduler(t)
	b := NewBucket(s, "combat", 50*time.Millisecond)

	var calls atomic.Int32
	b.Subscribe(func(context.Context) { calls.Add(1) })

	clk.Advance(50 * time.Millisecond)
	waitFor(t, func() bool { return calls.Load() == 1 }, "first pulse")

	b.Stop()
	clk.Advance(100 * time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("bucket fired after Stop: %d", got)
	}
}

func TestBucketIsolation(t *testing.T) {
	s, clk := newTestScheduler(t)
	combat := NewBucket(s, "combat", 50*time.Millisecond)
	regen := NewBucket(s, "regen", 200*time.Millisecond)

	var combatCalls, regenCalls atomic.Int32
	combat.Subscribe(func(context.Context) { combatCalls.Add(1) })
	regen.Subscribe(func(context.Context) { regenCalls.Add(1) })

	clk.Advance(50 * time.Millisecond)
	waitFor(t, func() bool { return combatCalls.Load() == 1 }, "combat 1")
	if regenCalls.Load() != 0 {
		t.Fatalf("regen fired early: %d", regenCalls.Load())
	}

	clk.Advance(150 * time.Millisecond)
	waitFor(t, func() bool { return regenCalls.Load() == 1 }, "regen 1")
}

func TestNewBucketsDefaults(t *testing.T) {
	s := New()
	s.Start(context.Background())
	defer s.Stop()

	bs := NewBuckets(s)
	defer bs.Stop()

	if bs.Combat.Interval() != DefaultCombatInterval {
		t.Errorf("combat interval = %v, want %v", bs.Combat.Interval(), DefaultCombatInterval)
	}
	if bs.Regen.Interval() != DefaultRegenInterval {
		t.Errorf("regen interval = %v, want %v", bs.Regen.Interval(), DefaultRegenInterval)
	}
	if bs.AreaReset.Interval() != DefaultAreaResetInterval {
		t.Errorf("areaReset interval = %v, want %v", bs.AreaReset.Interval(), DefaultAreaResetInterval)
	}
	if bs.Decay.Interval() != DefaultDecayInterval {
		t.Errorf("decay interval = %v, want %v", bs.Decay.Interval(), DefaultDecayInterval)
	}
}

func TestNilSubscribeIsNoop(t *testing.T) {
	s, _ := newTestScheduler(t)
	b := NewBucket(s, "combat", 50*time.Millisecond)
	cancel := b.Subscribe(nil)
	cancel() // must not panic
}
