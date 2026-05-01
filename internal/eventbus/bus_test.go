package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fooEvent struct{ N int }
type barEvent struct{ S string }

func TestSyncPublishToTypedSubscriber(t *testing.T) {
	b := New()
	var got int32
	Subscribe(b, func(_ context.Context, ev fooEvent) {
		atomic.StoreInt32(&got, int32(ev.N))
	})
	b.Publish(context.Background(), fooEvent{N: 42})
	if got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
}

func TestPublishWrongTypeIsIgnored(t *testing.T) {
	b := New()
	var fooCalls, barCalls atomic.Int32
	Subscribe(b, func(context.Context, fooEvent) { fooCalls.Add(1) })
	Subscribe(b, func(context.Context, barEvent) { barCalls.Add(1) })

	b.Publish(context.Background(), fooEvent{N: 1})
	if fooCalls.Load() != 1 || barCalls.Load() != 0 {
		t.Fatalf("foo=%d bar=%d", fooCalls.Load(), barCalls.Load())
	}

	b.Publish(context.Background(), barEvent{S: "hi"})
	if fooCalls.Load() != 1 || barCalls.Load() != 1 {
		t.Fatalf("foo=%d bar=%d", fooCalls.Load(), barCalls.Load())
	}
}

func TestMultipleSubscribersFanOut(t *testing.T) {
	b := New()
	var calls atomic.Int32
	Subscribe(b, func(context.Context, fooEvent) { calls.Add(1) })
	Subscribe(b, func(context.Context, fooEvent) { calls.Add(1) })
	Subscribe(b, func(context.Context, fooEvent) { calls.Add(1) })

	b.Publish(context.Background(), fooEvent{})
	if calls.Load() != 3 {
		t.Fatalf("got %d, want 3", calls.Load())
	}
}

func TestCancelRemovesSubscriber(t *testing.T) {
	b := New()
	var calls atomic.Int32
	sub := Subscribe(b, func(context.Context, fooEvent) { calls.Add(1) })
	sub.Cancel()
	b.Publish(context.Background(), fooEvent{})
	if calls.Load() != 0 {
		t.Fatalf("cancelled subscriber fired: %d", calls.Load())
	}
	sub.Cancel() // idempotent
}

func TestPublishNoSubscribers(t *testing.T) {
	b := New()
	b.Publish(context.Background(), fooEvent{}) // must not panic
}

func TestPublishNilEventIsNoop(t *testing.T) {
	b := New()
	var calls atomic.Int32
	Subscribe(b, func(context.Context, fooEvent) { calls.Add(1) })
	b.Publish(context.Background(), nil)
	if calls.Load() != 0 {
		t.Fatalf("nil event dispatched: %d", calls.Load())
	}
}

func TestSubscriberPanicIsRecovered(t *testing.T) {
	b := New()
	var calls atomic.Int32
	Subscribe(b, func(context.Context, fooEvent) { panic("boom") })
	Subscribe(b, func(context.Context, fooEvent) { calls.Add(1) })

	b.Publish(context.Background(), fooEvent{})
	if calls.Load() != 1 {
		t.Fatalf("second handler did not run: %d", calls.Load())
	}
}

func TestAsyncSubscriberDispatch(t *testing.T) {
	b := New()
	defer b.Stop()

	var wg sync.WaitGroup
	wg.Add(1)
	var got atomic.Int32
	SubscribeAsync(b, func(_ context.Context, ev fooEvent) {
		got.Store(int32(ev.N))
		wg.Done()
	})

	b.Publish(context.Background(), fooEvent{N: 7})
	doneCh := make(chan struct{})
	go func() { wg.Wait(); close(doneCh) }()
	select {
	case <-doneCh:
	case <-time.After(time.Second):
		t.Fatal("async handler did not fire")
	}
	if got.Load() != 7 {
		t.Fatalf("got %d, want 7", got.Load())
	}
}

func TestStopWithoutAsyncIsNoop(t *testing.T) {
	b := New()
	b.Stop()
}

func TestSubscribeNilFnIsNoop(t *testing.T) {
	b := New()
	sub := Subscribe[fooEvent](b, nil)
	sub.Cancel()
	b.Publish(context.Background(), fooEvent{})
}

func TestConcurrentPublishAndSubscribe(t *testing.T) {
	b := New()
	defer b.Stop()

	var calls atomic.Int32
	for i := 0; i < 8; i++ {
		Subscribe(b, func(context.Context, fooEvent) { calls.Add(1) })
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				b.Publish(context.Background(), fooEvent{})
			}
		}()
	}
	wg.Wait()

	if got := calls.Load(); got != 16*50*8 {
		t.Fatalf("calls=%d, want %d", got, 16*50*8)
	}
}
