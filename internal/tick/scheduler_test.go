package tick

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock provides a deterministic clock + manual ticker for tests.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
	ch  chan time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start, ch: make(chan time.Time, 64)}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	t := f.now
	f.mu.Unlock()
	f.ch <- t
}

func (f *fakeClock) NewChan(d time.Duration) (<-chan time.Time, func()) {
	return f.ch, func() {}
}

func newTestScheduler(t *testing.T) (*Scheduler, *fakeClock) {
	t.Helper()
	clk := newFakeClock(time.Unix(0, 0))
	s := New(WithClock(clk.Now, clk.NewChan))
	s.Start(context.Background())
	t.Cleanup(s.Stop)
	return s, clk
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting: %s", msg)
}

func TestSubscribeFiresAtInterval(t *testing.T) {
	s, clk := newTestScheduler(t)
	var calls atomic.Int32
	s.Subscribe(100*time.Millisecond, func(context.Context) { calls.Add(1) })

	clk.Advance(100 * time.Millisecond)
	waitFor(t, func() bool { return calls.Load() == 1 }, "first call")

	clk.Advance(100 * time.Millisecond)
	waitFor(t, func() bool { return calls.Load() == 2 }, "second call")
}

func TestSubscribeNotDueYet(t *testing.T) {
	s, clk := newTestScheduler(t)
	var calls atomic.Int32
	s.Subscribe(200*time.Millisecond, func(context.Context) { calls.Add(1) })

	clk.Advance(100 * time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatalf("handler fired early: %d", calls.Load())
	}
}

func TestAfterFiresOnce(t *testing.T) {
	s, clk := newTestScheduler(t)
	var calls atomic.Int32
	s.After(50*time.Millisecond, func(context.Context) { calls.Add(1) })

	clk.Advance(50 * time.Millisecond)
	waitFor(t, func() bool { return calls.Load() == 1 }, "first fire")

	clk.Advance(50 * time.Millisecond)
	clk.Advance(50 * time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("After fired more than once: %d", got)
	}
}

func TestCancelRemovesSubscription(t *testing.T) {
	s, clk := newTestScheduler(t)
	var calls atomic.Int32
	sub := s.Subscribe(50*time.Millisecond, func(context.Context) { calls.Add(1) })
	sub.Cancel()

	clk.Advance(100 * time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatalf("cancelled subscription fired: %d", calls.Load())
	}

	// Cancel is idempotent.
	sub.Cancel()
}

func TestStopWaitsForHeartbeat(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	s := New(WithClock(clk.Now, clk.NewChan))
	s.Start(context.Background())
	s.Stop()
	s.Stop() // idempotent
}

func TestStopBeforeStartIsNoop(t *testing.T) {
	s := New()
	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop before Start deadlocked")
	}
}

func TestSubscriberPanicDoesNotKillScheduler(t *testing.T) {
	s, clk := newTestScheduler(t)
	var calls atomic.Int32
	s.Subscribe(50*time.Millisecond, func(context.Context) { panic("boom") })
	s.Subscribe(50*time.Millisecond, func(context.Context) { calls.Add(1) })

	clk.Advance(50 * time.Millisecond)
	waitFor(t, func() bool { return calls.Load() == 1 }, "second handler still runs")
}

func TestZeroIntervalSubscribeIsNoop(t *testing.T) {
	s, clk := newTestScheduler(t)
	var calls atomic.Int32
	sub := s.Subscribe(0, func(context.Context) { calls.Add(1) })
	if sub == nil {
		t.Fatal("Subscribe returned nil")
	}
	clk.Advance(time.Second)
	time.Sleep(10 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatalf("zero-interval handler fired: %d", calls.Load())
	}
}

func TestContextCancelledOnStop(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	s := New(WithClock(clk.Now, clk.NewChan))
	s.Start(context.Background())

	seen := make(chan struct{}, 1)
	s.Subscribe(10*time.Millisecond, func(ctx context.Context) {
		<-ctx.Done()
		seen <- struct{}{}
	})

	clk.Advance(10 * time.Millisecond)
	// give handler time to enter
	time.Sleep(20 * time.Millisecond)
	s.Stop()

	select {
	case <-seen:
	case <-time.After(time.Second):
		t.Fatal("handler did not observe ctx cancellation")
	}
}
