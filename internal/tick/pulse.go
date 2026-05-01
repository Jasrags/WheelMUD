package tick

import (
	"context"
	"sync"
	"time"
)

// Bucket is a named group of subscribers that all fire at the same
// cadence. It is one Scheduler subscription that fans out to its own
// list — keeping cadence math centralized and letting tests swap a
// single bucket without touching the global scheduler.
//
// Buckets are typically created by NewBuckets at server startup and
// shared across the codebase.
//
// Dispatch model: each pulse runs every subscriber sequentially on
// the scheduler's per-tick goroutine. A slow handler delays every
// other handler in the same bucket and keeps that goroutine alive
// past the tick boundary. Subscribers MUST be non-blocking and
// short-lived; spawn your own goroutine for real work.
type Bucket struct {
	name  string
	every time.Duration
	sub   *Subscription

	mu       sync.RWMutex
	handlers map[uint64]HandlerFunc
	nextID   uint64
}

// NewBucket registers a new pulse bucket on the given scheduler.
// every is the bucket's cadence (independent of the scheduler's Hz,
// though the bucket can only fire on tick boundaries — every is
// rounded up internally to the nearest tick).
//
// Returns the Bucket; call Stop to release the underlying scheduler
// subscription. Buckets are intended to live for the process
// lifetime, so Stop is mainly for tests.
func NewBucket(s *Scheduler, name string, every time.Duration) *Bucket {
	b := &Bucket{
		name:     name,
		every:    every,
		handlers: make(map[uint64]HandlerFunc),
	}
	b.sub = s.Subscribe(every, b.fire)
	return b
}

// Name returns the bucket's name.
func (b *Bucket) Name() string { return b.name }

// Interval returns the bucket's cadence.
func (b *Bucket) Interval() time.Duration { return b.every }

// Subscribe registers fn to fire each time the bucket pulses.
// Returns a cancel func.
func (b *Bucket) Subscribe(fn HandlerFunc) func() {
	if fn == nil {
		return func() {}
	}
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	b.handlers[id] = fn
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		delete(b.handlers, id)
		b.mu.Unlock()
	}
}

// Stop releases the bucket's scheduler subscription. Subsequent
// pulses will not fire, but already-dispatched handler goroutines
// continue per the HandlerFunc contract.
func (b *Bucket) Stop() {
	b.sub.Cancel()
}

func (b *Bucket) fire(ctx context.Context) {
	b.mu.RLock()
	handlers := make([]HandlerFunc, 0, len(b.handlers))
	for _, h := range b.handlers {
		handlers = append(handlers, h)
	}
	b.mu.RUnlock()

	for _, h := range handlers {
		h(ctx)
	}
}

// Buckets bundles the named pulse buckets used by the game loop.
// Add new buckets here as new subsystems land (regen, areaReset,
// scripting, etc.) so cadence policy stays in one place.
type Buckets struct {
	Combat    *Bucket
	Regen     *Bucket
	AreaReset *Bucket
}

// Default cadences for the game-loop pulse buckets. These can be
// tuned later; the values are conservative starting points modeled on
// classic DikuMUD-family servers.
const (
	DefaultCombatInterval    = 4 * time.Second
	DefaultRegenInterval     = 30 * time.Second
	DefaultAreaResetInterval = 5 * time.Minute
)

// NewBuckets registers the default game-loop buckets on s.
func NewBuckets(s *Scheduler) *Buckets {
	return &Buckets{
		Combat:    NewBucket(s, "combat", DefaultCombatInterval),
		Regen:     NewBucket(s, "regen", DefaultRegenInterval),
		AreaReset: NewBucket(s, "areaReset", DefaultAreaResetInterval),
	}
}

// Stop cancels every bucket's scheduler subscription.
func (bs *Buckets) Stop() {
	if bs == nil {
		return
	}
	if bs.Combat != nil {
		bs.Combat.Stop()
	}
	if bs.Regen != nil {
		bs.Regen.Stop()
	}
	if bs.AreaReset != nil {
		bs.AreaReset.Stop()
	}
}
