// Package tick provides a single-goroutine heartbeat scheduler used as
// the substrate for pulse buckets, scheduled actions, and graceful
// shutdown drains.
//
// The scheduler ticks at a fixed Hz and fans out to subscribers in
// their own goroutines (fire-and-forget). Slow subscribers cannot
// stall the heartbeat. A slog warning is emitted if a tick's fan-out
// dispatch itself takes longer than slowDispatchThreshold.
package tick

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultHz             = 1
	slowDispatchThreshold = 50 * time.Millisecond
)

// HandlerFunc is invoked on every tick a subscription is due. The
// context is the scheduler's run context and is canceled on Stop.
//
// Handlers MUST be non-blocking and short-lived: each invocation
// runs in its own goroutine that the scheduler does not track or
// await on Stop. A handler that blocks past Stop will outlive the
// scheduler, holding any captured resources. If a handler needs to
// do real work, it should hand off to a worker pool of its own.
type HandlerFunc func(ctx context.Context)

// Subscription is returned by Subscribe / After. Cancel removes the
// handler; calling Cancel more than once is safe.
type Subscription struct {
	id     uint64
	cancel func()
}

// Cancel removes the subscription. Safe to call concurrently and
// multiple times.
func (s *Subscription) Cancel() {
	if s == nil || s.cancel == nil {
		return
	}
	s.cancel()
}

// Scheduler drives a heartbeat goroutine and dispatches due
// subscriptions on each tick. The zero value is not usable; construct
// with New.
type Scheduler struct {
	hz       int
	now      func() time.Time
	newChan  func(d time.Duration) (<-chan time.Time, func())
	logger   *slog.Logger

	mu       sync.Mutex
	subs     map[uint64]*subscription
	nextID   uint64

	startOnce sync.Once
	stopOnce  sync.Once
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	running   atomic.Bool
}

type subscription struct {
	id       uint64
	every    time.Duration
	next     time.Time
	once     bool
	handler  HandlerFunc
}

// Option configures a Scheduler.
type Option func(*Scheduler)

// WithHz overrides the default 1 Hz heartbeat. Must be > 0.
func WithHz(hz int) Option {
	return func(s *Scheduler) {
		if hz > 0 {
			s.hz = hz
		}
	}
}

// WithClock injects a clock seam for tests. now returns the current
// time; newChan returns a channel that fires at the requested
// interval and a stop func to release resources.
func WithClock(now func() time.Time, newChan func(d time.Duration) (<-chan time.Time, func())) Option {
	return func(s *Scheduler) {
		if now != nil {
			s.now = now
		}
		if newChan != nil {
			s.newChan = newChan
		}
	}
}

// WithLogger overrides the slog logger used for slow-dispatch warnings.
func WithLogger(l *slog.Logger) Option {
	return func(s *Scheduler) {
		if l != nil {
			s.logger = l
		}
	}
}

// New constructs a Scheduler. Call Start to begin ticking.
func New(opts ...Option) *Scheduler {
	s := &Scheduler{
		hz:     defaultHz,
		now:    time.Now,
		logger: slog.Default(),
		subs:   make(map[uint64]*subscription),
		done:   make(chan struct{}),
	}
	s.newChan = func(d time.Duration) (<-chan time.Time, func()) {
		t := time.NewTicker(d)
		return t.C, t.Stop
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start launches the heartbeat goroutine. Subsequent calls are
// no-ops. The scheduler runs until Stop is called.
func (s *Scheduler) Start(parent context.Context) {
	s.startOnce.Do(func() {
		if parent == nil {
			parent = context.Background()
		}
		s.ctx, s.cancel = context.WithCancel(parent)
		s.running.Store(true)
		go s.run()
	})
}

// Stop cancels the run context and waits for the heartbeat goroutine
// to exit. Any in-flight subscriber goroutines observe ctx
// cancellation but are not awaited — fire-and-forget by design.
// Stop is a no-op if Start was never called.
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		if s.cancel == nil {
			// Start was never called; nothing to drain.
			return
		}
		s.cancel()
		<-s.done
		s.running.Store(false)
	})
}

// Subscribe registers fn to run every `every`. The first invocation
// fires after `every` from now. Returns a Subscription whose Cancel
// removes the handler.
//
// Subscribing before Start is supported; the handler will fire once
// the heartbeat begins.
func (s *Scheduler) Subscribe(every time.Duration, fn HandlerFunc) *Subscription {
	if every <= 0 || fn == nil {
		return &Subscription{}
	}
	return s.add(every, fn, false)
}

// After schedules fn to run once after d. The subscription is
// auto-cancelled after firing.
func (s *Scheduler) After(d time.Duration, fn HandlerFunc) *Subscription {
	if d < 0 || fn == nil {
		return &Subscription{}
	}
	return s.add(d, fn, true)
}

func (s *Scheduler) add(every time.Duration, fn HandlerFunc, once bool) *Subscription {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	sub := &subscription{
		id:      id,
		every:   every,
		next:    s.now().Add(every),
		once:    once,
		handler: fn,
	}
	s.subs[id] = sub
	s.mu.Unlock()

	return &Subscription{
		id: id,
		cancel: func() {
			s.mu.Lock()
			delete(s.subs, id)
			s.mu.Unlock()
		},
	}
}

func (s *Scheduler) run() {
	defer close(s.done)

	interval := time.Second / time.Duration(s.hz)
	if interval <= 0 {
		interval = time.Second
	}
	c, stop := s.newChan(interval)
	defer stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-c:
			s.tick()
		}
	}
}

func (s *Scheduler) tick() {
	dispatchStart := s.now()
	now := dispatchStart

	s.mu.Lock()
	due := make([]*subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		if !now.Before(sub.next) {
			due = append(due, sub)
			if sub.once {
				delete(s.subs, sub.id)
			} else {
				sub.next = now.Add(sub.every)
			}
		}
	}
	s.mu.Unlock()

	for _, sub := range due {
		fn := sub.handler
		go func() {
			defer func() {
				if r := recover(); r != nil {
					s.logger.Error("tick subscriber panicked", "panic", r)
				}
			}()
			fn(s.ctx)
		}()
	}

	if elapsed := time.Since(dispatchStart); elapsed > slowDispatchThreshold {
		s.logger.Warn("tick fan-out exceeded threshold",
			"elapsed", elapsed,
			"threshold", slowDispatchThreshold,
			"due", len(due))
	}
}
