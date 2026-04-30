package session

import (
	"net"
	"testing"

	"github.com/Jasrags/WheelMUD/telnet"
)

func newSession(t *testing.T) *telnet.Session {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })
	return telnet.NewSession(server)
}

func TestRegistry_BindReturnsPrevious(t *testing.T) {
	r := NewRegistry()
	s1 := newSession(t)
	s2 := newSession(t)

	if prev := r.Bind(42, s1); prev != nil {
		t.Fatalf("first Bind prev = %v, want nil", prev)
	}
	if prev := r.Bind(42, s2); prev != s1 {
		t.Fatalf("second Bind prev = %v, want s1", prev)
	}
	if got := r.Lookup(42); got != s2 {
		t.Fatalf("Lookup = %v, want s2", got)
	}
}

func TestRegistry_UnbindCompareAndDelete(t *testing.T) {
	r := NewRegistry()
	s1 := newSession(t)
	s2 := newSession(t)

	r.Bind(42, s1)
	r.Bind(42, s2) // s1 has been kicked

	// s1's teardown defer fires after the kick — must NOT blow away s2.
	r.Unbind(42, s1)
	if got := r.Lookup(42); got != s2 {
		t.Fatalf("stale Unbind clobbered the new binding: got %v, want s2", got)
	}

	// s2's own teardown — should now clear.
	r.Unbind(42, s2)
	if got := r.Lookup(42); got != nil {
		t.Fatalf("Unbind of bound session left it: %v", got)
	}
}

func TestRegistry_UnbindUnknownIsNoop(t *testing.T) {
	r := NewRegistry()
	s := newSession(t)
	r.Unbind(99, s) // no panic, no error
	if got := r.Lookup(99); got != nil {
		t.Fatalf("Lookup = %v after no-op Unbind, want nil", got)
	}
}

func TestRegistry_Snapshot(t *testing.T) {
	r := NewRegistry()
	s1 := newSession(t)
	s2 := newSession(t)
	r.Bind(1, s1)
	r.Bind(2, s2)

	snap := r.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snap has %d entries, want 2", len(snap))
	}
	if snap[1] != s1 || snap[2] != s2 {
		t.Fatalf("snapshot entries wrong: %+v", snap)
	}

	// Mutating the snapshot must not affect the registry.
	delete(snap, 1)
	if r.Lookup(1) != s1 {
		t.Fatal("snapshot mutation leaked into registry")
	}
}
