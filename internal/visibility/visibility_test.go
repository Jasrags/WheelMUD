package visibility

import (
	"net"
	"testing"

	"github.com/Jasrags/WheelMUD/telnet"
)

// newSess constructs a Session over a discarded pipe so SetHidden
// reaches the real crossMu-guarded field. The session never reads
// or writes — this is purely a holder for AuthLevel + hidden state.
func newSess(t *testing.T, auth telnet.AuthLevel, hidden bool) *telnet.Session {
	t.Helper()
	a, b := net.Pipe()
	t.Cleanup(func() {
		_ = a.Close()
		_ = b.Close()
	})
	s := telnet.NewSession(a)
	if s == nil {
		t.Fatal("NewSession returned nil")
	}
	s.AuthLevel = auth
	if hidden {
		s.SetHidden(true)
	}
	return s
}

func TestCanSee_Matrix(t *testing.T) {
	tests := []struct {
		name        string
		viewerAuth  telnet.AuthLevel
		targetHide  bool
		sameSession bool
		want        bool
	}{
		{"self always visible", telnet.AuthGuest, true, true, true},
		{"non-hidden visible to guest", telnet.AuthGuest, false, false, true},
		{"non-hidden visible to player", telnet.AuthPlayer, false, false, true},
		{"non-hidden visible to admin", telnet.AuthAdmin, false, false, true},
		{"hidden invisible to guest", telnet.AuthGuest, true, false, false},
		{"hidden invisible to player", telnet.AuthPlayer, true, false, false},
		{"hidden visible to admin", telnet.AuthAdmin, true, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viewer := newSess(t, tt.viewerAuth, false)
			var target *telnet.Session
			if tt.sameSession {
				target = viewer
			} else {
				target = newSess(t, telnet.AuthGuest, tt.targetHide)
			}
			if got := CanSee(viewer, target); got != tt.want {
				t.Errorf("CanSee = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanSee_NilTarget(t *testing.T) {
	viewer := newSess(t, telnet.AuthAdmin, false)
	if CanSee(viewer, nil) {
		t.Error("nil target should not be visible")
	}
}

func TestCanSee_NilViewer(t *testing.T) {
	visible := newSess(t, telnet.AuthGuest, false)
	hidden := newSess(t, telnet.AuthGuest, true)
	if !CanSee(nil, visible) {
		t.Error("nil viewer should still see non-hidden target")
	}
	if CanSee(nil, hidden) {
		t.Error("nil viewer should not see hidden target (treated as non-admin)")
	}
}

func TestVisiblePeers_FiltersAndPreservesOrder(t *testing.T) {
	viewer := newSess(t, telnet.AuthPlayer, false)
	a := newSess(t, telnet.AuthGuest, false) // visible
	b := newSess(t, telnet.AuthGuest, true)  // hidden — filtered
	c := newSess(t, telnet.AuthGuest, false) // visible

	got := VisiblePeers(viewer, []*telnet.Session{a, b, c})
	if len(got) != 2 || got[0] != a || got[1] != c {
		t.Errorf("VisiblePeers filtered wrong: got %v want [a c]", got)
	}
}

func TestVisiblePeers_EmptyAndNil(t *testing.T) {
	viewer := newSess(t, telnet.AuthGuest, false)
	if got := VisiblePeers(viewer, nil); got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
	if got := VisiblePeers(viewer, []*telnet.Session{}); got != nil {
		t.Errorf("empty input should return nil, got %v", got)
	}
}

func TestVisiblePeers_AdminSeesHidden(t *testing.T) {
	viewer := newSess(t, telnet.AuthAdmin, false)
	a := newSess(t, telnet.AuthGuest, true)
	b := newSess(t, telnet.AuthGuest, false)

	got := VisiblePeers(viewer, []*telnet.Session{a, b})
	if len(got) != 2 {
		t.Errorf("admin should see both, got %d", len(got))
	}
}
