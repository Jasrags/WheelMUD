package persist

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestManager_RegistersAndFlushesInOrder(t *testing.T) {
	m := New()
	var order []string
	m.Register("first", func(_ context.Context) error {
		order = append(order, "first")
		return nil
	})
	m.Register("second", func(_ context.Context) error {
		order = append(order, "second")
		return nil
	})
	m.FlushAll(context.Background())
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("order = %v, want [first second]", order)
	}
}

func TestManager_ContinuesAfterError(t *testing.T) {
	m := New()
	var ran atomic.Int32
	m.Register("boom", func(_ context.Context) error {
		ran.Add(1)
		return errors.New("nope")
	})
	m.Register("after", func(_ context.Context) error {
		ran.Add(1)
		return nil
	})
	m.FlushAll(context.Background())
	if ran.Load() != 2 {
		t.Fatalf("ran = %d, want 2 (post-error saver should still fire)", ran.Load())
	}
}

func TestManager_AbortsOnContextCanceled(t *testing.T) {
	m := New()
	var ran atomic.Int32
	m.Register("first", func(_ context.Context) error {
		ran.Add(1)
		return nil
	})
	m.Register("second", func(_ context.Context) error {
		ran.Add(1)
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m.FlushAll(ctx)
	// First saver may run before the ctx check on the second
	// iteration, but second must not.
	if ran.Load() > 1 {
		t.Fatalf("ran = %d, expected to abort before second saver", ran.Load())
	}
}

func TestManager_NilSaverIgnored(t *testing.T) {
	m := New()
	m.Register("nil", nil)
	m.FlushAll(context.Background()) // must not panic
}
