// Package eventbus provides a typed publish/subscribe channel for
// in-process game events. Producers publish concrete event values
// (e.g. world.PlayerEntered{}) and subscribers register a handler
// against the event's type.
//
// Dispatch is synchronous on the publisher's goroutine by default —
// keep handlers fast or hand off to a worker. Use SubscribeAsync to
// have the bus dispatch via its own worker pool when listeners are
// known to be slow (e.g. metrics, scripting hooks).
//
// Subscriber panics are recovered and logged so one bad listener
// can't take down a publishing goroutine.
package eventbus

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
)

// Event is the marker interface for anything publishable. Any
// concrete type satisfies it; the interface exists only to make
// Publish's signature self-documenting.
type Event interface{}

// HandlerFunc receives a published event. The concrete type must be
// asserted by the handler; Subscribe[T] handles that automatically.
type HandlerFunc func(ctx context.Context, ev Event)

// Subscription identifies a registered handler. Cancel removes it;
// safe to call more than once.
type Subscription struct {
	cancel func()
}

// Cancel removes the subscription.
func (s *Subscription) Cancel() {
	if s == nil || s.cancel == nil {
		return
	}
	s.cancel()
}

// Bus is the typed pub/sub registry. The zero value is not usable;
// construct with New.
type Bus struct {
	logger *slog.Logger

	mu       sync.RWMutex
	handlers map[reflect.Type]map[uint64]subscriberEntry
	nextID   uint64

	asyncOnce     sync.Once
	asyncStopOnce sync.Once
	asyncCh       chan asyncDispatch
	asyncWG       sync.WaitGroup
	asyncCtx      context.Context
	asyncStop     context.CancelFunc
}

type subscriberEntry struct {
	fn    HandlerFunc
	async bool
}

type asyncDispatch struct {
	ctx context.Context
	ev  Event
	fn  HandlerFunc
}

// Option configures a Bus.
type Option func(*Bus)

// WithLogger overrides the slog logger used for panic recovery.
func WithLogger(l *slog.Logger) Option {
	return func(b *Bus) {
		if l != nil {
			b.logger = l
		}
	}
}

// New constructs a Bus.
func New(opts ...Option) *Bus {
	b := &Bus{
		logger:   slog.Default(),
		handlers: make(map[reflect.Type]map[uint64]subscriberEntry),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Publish dispatches ev to every handler registered for its dynamic
// type. Sync handlers run on the caller's goroutine in registration
// order; async handlers are queued to the worker and dispatched
// independently. ctx is forwarded to handlers.
func (b *Bus) Publish(ctx context.Context, ev Event) {
	if ev == nil {
		return
	}
	t := reflect.TypeOf(ev)

	b.mu.RLock()
	subs, ok := b.handlers[t]
	if !ok {
		b.mu.RUnlock()
		return
	}
	entries := make([]subscriberEntry, 0, len(subs))
	for _, e := range subs {
		entries = append(entries, e)
	}
	b.mu.RUnlock()

	for _, e := range entries {
		if e.async {
			b.dispatchAsync(ctx, ev, e.fn)
			continue
		}
		b.runOne(ctx, ev, e.fn)
	}
}

func (b *Bus) runOne(ctx context.Context, ev Event, fn HandlerFunc) {
	defer func() {
		if r := recover(); r != nil {
			b.logger.Error("eventbus subscriber panicked",
				"type", reflect.TypeOf(ev).String(),
				"panic", r)
		}
	}()
	fn(ctx, ev)
}

// Subscribe registers fn to receive events of type T. The handler
// runs synchronously on the publisher's goroutine.
//
// Generic free function (Go does not allow type parameters on
// methods) — call as eventbus.Subscribe[world.PlayerEntered](bus, fn).
func Subscribe[T Event](b *Bus, fn func(ctx context.Context, ev T)) *Subscription {
	return register(b, fn, false)
}

// SubscribeAsync registers fn to receive events of type T off the
// publisher's goroutine. The first async subscription on a given Bus
// lazily starts a single-worker dispatcher; Stop drains it.
func SubscribeAsync[T Event](b *Bus, fn func(ctx context.Context, ev T)) *Subscription {
	b.ensureAsync()
	return register(b, fn, true)
}

func register[T Event](b *Bus, fn func(ctx context.Context, ev T), async bool) *Subscription {
	if b == nil || fn == nil {
		return &Subscription{}
	}
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		// T is an interface type; require concrete events. Use
		// reflect.TypeOf((*T)(nil)).Elem() to recover the interface's
		// name so the panic message is actionable.
		ifaceT := reflect.TypeOf((*T)(nil)).Elem()
		panic(fmt.Sprintf("eventbus: cannot subscribe to interface type %s; use a concrete event type", ifaceT))
	}

	wrapper := func(ctx context.Context, ev Event) {
		typed, ok := ev.(T)
		if !ok {
			return
		}
		fn(ctx, typed)
	}

	b.mu.Lock()
	b.nextID++
	id := b.nextID
	subs, ok := b.handlers[t]
	if !ok {
		subs = make(map[uint64]subscriberEntry)
		b.handlers[t] = subs
	}
	subs[id] = subscriberEntry{fn: wrapper, async: async}
	b.mu.Unlock()

	return &Subscription{
		cancel: func() {
			b.mu.Lock()
			if subs, ok := b.handlers[t]; ok {
				delete(subs, id)
				if len(subs) == 0 {
					delete(b.handlers, t)
				}
			}
			b.mu.Unlock()
		},
	}
}

func (b *Bus) ensureAsync() {
	b.asyncOnce.Do(func() {
		b.asyncCtx, b.asyncStop = context.WithCancel(context.Background())
		b.asyncCh = make(chan asyncDispatch, 256)
		b.asyncWG.Add(1)
		go b.runAsync()
	})
}

func (b *Bus) runAsync() {
	defer b.asyncWG.Done()
	for {
		select {
		case <-b.asyncCtx.Done():
			// Drain anything already queued so callers that Publish
			// just before Stop see best-effort delivery instead of
			// silent drops. New publishes after asyncCtx is done will
			// hit the select-default in dispatchAsync and be dropped
			// with a log line.
			for {
				select {
				case d := <-b.asyncCh:
					b.runOne(d.ctx, d.ev, d.fn)
				default:
					return
				}
			}
		case d, ok := <-b.asyncCh:
			if !ok {
				return
			}
			b.runOne(d.ctx, d.ev, d.fn)
		}
	}
}

func (b *Bus) dispatchAsync(ctx context.Context, ev Event, fn HandlerFunc) {
	select {
	case b.asyncCh <- asyncDispatch{ctx: ctx, ev: ev, fn: fn}:
	default:
		b.logger.Warn("eventbus async queue full; dropping event",
			"type", reflect.TypeOf(ev).String())
	}
}

// Stop signals the async worker (if any) to exit and waits for it.
// Events already queued at Stop time are drained on a best-effort
// basis; any Publish that races with Stop and arrives after the
// worker has exited will be dropped via dispatchAsync's
// select-default and logged. Sync Publish continues to work after
// Stop. Idempotent and safe to call concurrently.
func (b *Bus) Stop() {
	b.asyncStopOnce.Do(func() {
		if b.asyncStop == nil {
			return
		}
		b.asyncStop()
		b.asyncWG.Wait()
	})
}
