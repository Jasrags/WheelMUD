package tick

import (
	"context"
	"sync"
	"time"
)

// AfterCtx schedules fn to fire once after d, but auto-cancels if ctx
// is canceled first. The handler will not run after ctx.Done closes,
// even if d has already elapsed and the dispatch goroutine is in
// flight (the wrapped handler re-checks ctx).
//
// This is the building block for session- or room-scoped delayed
// actions: pass a per-session context and the timer dies with the
// session.
//
// Returns a Subscription whose Cancel removes the timer and releases
// the watcher goroutine. Cancel is safe to call multiple times and
// after the timer has fired.
func AfterCtx(s *Scheduler, ctx context.Context, d time.Duration, fn HandlerFunc) *Subscription {
	if s == nil || fn == nil {
		return &Subscription{}
	}
	if ctx == nil {
		return s.After(d, fn)
	}

	// Wrap fn so an in-flight dispatch still respects ctx cancellation.
	wrapped := func(handlerCtx context.Context) {
		if ctx.Err() != nil {
			return
		}
		fn(handlerCtx)
	}

	inner := s.After(d, wrapped)

	// Watch ctx; cancel the inner subscription if ctx fires before the
	// timer. The watcher exits via the stop channel when the caller
	// cancels the returned Subscription.
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			inner.Cancel()
		case <-stop:
		}
	}()

	var once sync.Once
	return &Subscription{
		cancel: func() {
			inner.Cancel()
			once.Do(func() { close(stop) })
		},
	}
}
