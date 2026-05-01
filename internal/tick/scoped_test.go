package tick

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestAfterCtxFiresWhenCtxLive(t *testing.T) {
	s, clk := newTestScheduler(t)
	var calls atomic.Int32
	ctx := context.Background()
	AfterCtx(s, ctx, 50*time.Millisecond, func(context.Context) { calls.Add(1) })

	clk.Advance(50 * time.Millisecond)
	waitFor(t, func() bool { return calls.Load() == 1 }, "fired with live ctx")
}

func TestAfterCtxSuppressedAfterCancel(t *testing.T) {
	s, clk := newTestScheduler(t)
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	AfterCtx(s, ctx, 50*time.Millisecond, func(context.Context) { calls.Add(1) })

	cancel()
	// Give the watcher goroutine a moment to call Cancel on the sub.
	time.Sleep(10 * time.Millisecond)
	clk.Advance(100 * time.Millisecond)
	time.Sleep(10 * time.Millisecond)

	if got := calls.Load(); got != 0 {
		t.Fatalf("handler fired after ctx cancel: %d", got)
	}
}

func TestAfterCtxRaceCtxCancelsAtFire(t *testing.T) {
	// Even if the timer has been dispatched, the wrapped handler should
	// re-check ctx before invoking the user fn.
	s, clk := newTestScheduler(t)
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	AfterCtx(s, ctx, 50*time.Millisecond, func(context.Context) { calls.Add(1) })

	cancel()
	clk.Advance(50 * time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("ctx-checking wrapper allowed handler to run: %d", got)
	}
}

func TestAfterCtxManualCancelReleasesWatcher(t *testing.T) {
	// Cancelling the subscription itself must release the watcher
	// goroutine so it doesn't leak.
	s, clk := newTestScheduler(t)
	ctx := context.Background()
	sub := AfterCtx(s, ctx, time.Hour, func(context.Context) {})
	sub.Cancel()
	sub.Cancel() // idempotent

	clk.Advance(time.Second)
	time.Sleep(10 * time.Millisecond)
}

func TestAfterCtxNilCtxFallsBackToAfter(t *testing.T) {
	s, clk := newTestScheduler(t)
	var calls atomic.Int32
	AfterCtx(s, nil, 50*time.Millisecond, func(context.Context) { calls.Add(1) })

	clk.Advance(50 * time.Millisecond)
	waitFor(t, func() bool { return calls.Load() == 1 }, "fired with nil ctx")
}
