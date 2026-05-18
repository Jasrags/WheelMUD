package telnet

import (
	"testing"
	"time"
)

func TestBadInputTracker_NilSafe(t *testing.T) {
	var nilTracker *BadInputTracker
	if got := nilTracker.Record(&Session{}); !got {
		t.Errorf("nil tracker should always allow, got %v", got)
	}
	nilTracker.Forget(&Session{}) // shouldn't panic
}

func TestBadInputTracker_NilSession(t *testing.T) {
	tr := NewBadInputTracker(time.Second, 3)
	if got := tr.Record(nil); !got {
		t.Errorf("nil session should always allow, got %v", got)
	}
}

func TestBadInputTracker_AllowsBurstThenThrottles(t *testing.T) {
	tr := NewBadInputTracker(time.Second, 3)
	now := time.Unix(0, 0)
	tr.clock = func() time.Time { return now }

	s := &Session{}
	// First 3 hits allowed.
	for i := 0; i < 3; i++ {
		if !tr.Record(s) {
			t.Fatalf("hit %d should be allowed, was throttled", i+1)
		}
	}
	// 4th hit within the window is dropped.
	if tr.Record(s) {
		t.Error("hit 4 should be throttled, was allowed")
	}
	// 5th hit also dropped — count keeps climbing but stays over.
	if tr.Record(s) {
		t.Error("hit 5 should be throttled, was allowed")
	}
}

func TestBadInputTracker_WindowExpiryResets(t *testing.T) {
	tr := NewBadInputTracker(time.Second, 2)
	now := time.Unix(0, 0)
	tr.clock = func() time.Time { return now }

	s := &Session{}
	if !tr.Record(s) {
		t.Fatal("hit 1 should allow")
	}
	if !tr.Record(s) {
		t.Fatal("hit 2 should allow")
	}
	if tr.Record(s) {
		t.Fatal("hit 3 in same window should throttle")
	}
	// Advance past the window.
	now = now.Add(2 * time.Second)
	if !tr.Record(s) {
		t.Error("hit after window expiry should allow")
	}
}

func TestBadInputTracker_PerSession(t *testing.T) {
	tr := NewBadInputTracker(time.Second, 1)
	now := time.Unix(0, 0)
	tr.clock = func() time.Time { return now }

	s1 := &Session{}
	s2 := &Session{}
	if !tr.Record(s1) {
		t.Fatal("s1 first hit allow")
	}
	if tr.Record(s1) {
		t.Error("s1 second hit should throttle")
	}
	// s2 has its own counter.
	if !tr.Record(s2) {
		t.Error("s2 first hit should allow despite s1 throttle")
	}
	if tr.Record(s2) {
		t.Error("s2 second hit should throttle")
	}
}

func TestBadInputTracker_Forget(t *testing.T) {
	tr := NewBadInputTracker(time.Second, 1)
	now := time.Unix(0, 0)
	tr.clock = func() time.Time { return now }

	s := &Session{}
	tr.Record(s)
	tr.Record(s) // throttle anchor
	tr.Forget(s)

	// After forget, the next hit anchors a fresh window.
	if !tr.Record(s) {
		t.Error("post-Forget hit should allow")
	}
}

func TestBadInputTracker_ZeroBurstDisablesThrottle(t *testing.T) {
	tr := NewBadInputTracker(time.Second, 0)
	now := time.Unix(0, 0)
	tr.clock = func() time.Time { return now }

	s := &Session{}
	for i := 0; i < 100; i++ {
		if !tr.Record(s) {
			t.Fatalf("zero-burst tracker should never throttle (i=%d)", i)
		}
	}
}
