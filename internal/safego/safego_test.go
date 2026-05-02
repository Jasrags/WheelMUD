package safego

import (
	"sync"
	"testing"
)

func TestGo_RunsFunction(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	ran := false
	Go("test-runner", func() {
		defer wg.Done()
		ran = true
	})
	wg.Wait()
	if !ran {
		t.Fatal("fn was not invoked")
	}
}

func TestGo_RecoversFromPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	// A panic inside the goroutine must not crash the test process.
	// safego.Go's deferred recover catches it; we just need the
	// goroutine to terminate cleanly so wg.Wait returns.
	Go("test-panicker", func() {
		defer wg.Done()
		panic("boom")
	})
	wg.Wait()
}
