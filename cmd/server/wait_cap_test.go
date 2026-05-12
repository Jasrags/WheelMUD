package main

import (
	"strings"
	"testing"
)

// Phase F #32 slice 5c polish: the global wait-cap defense in depth.
//
// acquireWaitSlot increments outstandingWaits and refuses when over
// MaxOutstandingWaits. Refusal must NOT leave the counter incremented
// (otherwise a single refusal would permanently shave one slot off
// the cap). releaseWaitSlot decrements without underflowing.

func TestWaitCap_AcquireUntilCapThenRefuse(t *testing.T) {
	// Reset to a clean baseline so the test is order-independent
	// across the package's other (currently zero) tests.
	resetWaitSlots(t)

	for i := int32(1); i <= MaxOutstandingWaits; i++ {
		if err := acquireWaitSlot(); err != nil {
			t.Fatalf("acquire %d/%d: %v", i, MaxOutstandingWaits, err)
		}
	}
	// One past the cap must refuse.
	err := acquireWaitSlot()
	if err == nil {
		t.Fatal("acquire at cap+1: want refusal, got nil")
	}
	if !strings.Contains(err.Error(), "wait cap reached") {
		t.Fatalf("refusal text = %q, want it to mention 'wait cap reached'", err.Error())
	}
	// Refusal must have rolled the counter back, so the next
	// release+acquire cycle still works.
	releaseWaitSlot()
	if err := acquireWaitSlot(); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}

	// Drain so the test leaves no residue for sibling cases.
	resetWaitSlots(t)
}

func TestWaitCap_ReleaseAllowsReacquire(t *testing.T) {
	resetWaitSlots(t)

	if err := acquireWaitSlot(); err != nil {
		t.Fatalf("acquire #1: %v", err)
	}
	releaseWaitSlot()
	if err := acquireWaitSlot(); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}

	resetWaitSlots(t)
}

// resetWaitSlots drains the global counter back to zero so each test
// case starts from a known baseline. Test-only helper; production
// callers never zero the counter — fires always pair with the
// acquire that scheduled them.
func resetWaitSlots(t *testing.T) {
	t.Helper()
	for outstandingWaits.Load() > 0 {
		releaseWaitSlot()
	}
}
