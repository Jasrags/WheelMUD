package cmd

import (
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/telnet"
)

func TestWizinvis_TogglesHiddenAndAcks(t *testing.T) {
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	wi := NewWizinvis(nil)

	runCmd(t, wi, s, "wizinvis")
	if !s.IsHidden() {
		t.Fatal("after first toggle, expected hidden = true")
	}
	if !strings.Contains(conn.String(), "fade from sight") {
		t.Fatalf("missing fade-out ack; got %q", conn.String())
	}
	conn.Reset()

	runCmd(t, wi, s, "wizinvis")
	if s.IsHidden() {
		t.Fatal("after second toggle, expected hidden = false")
	}
	if !strings.Contains(conn.String(), "return to view") {
		t.Fatalf("missing return ack; got %q", conn.String())
	}
}

func TestWho_HidesWizinvisFromNonAdmin(t *testing.T) {
	sessions, alice, bob, aOut, bOut := commPair(t)
	bob.AuthLevel = telnet.AuthAdmin
	bob.SetHidden(true)
	who := NewWho(sessions, whoTestChars(t))

	// Alice (player) runs `who` — Bob is hidden and should not appear.
	runCmd(t, who, alice, "who")
	if strings.Contains(aOut.String(), "Bob") {
		t.Fatalf("alice saw hidden admin Bob: %q", aOut.String())
	}
	if !strings.Contains(aOut.String(), "Alice") {
		t.Fatalf("alice missing from her own who: %q", aOut.String())
	}

	// Bob (admin) runs `who` — sees Alice and himself with the * marker.
	runCmd(t, who, bob, "who")
	if !strings.Contains(bOut.String(), "Bob") {
		t.Fatalf("admin Bob did not see himself: %q", bOut.String())
	}
	if !strings.Contains(bOut.String(), "*") {
		t.Fatalf("admin view missing wizinvis marker: %q", bOut.String())
	}
}

func TestTell_HiddenAdminAppearsOfflineToPlayer(t *testing.T) {
	sessions, alice, bob, aOut, bOut := commPair(t)
	bob.AuthLevel = telnet.AuthAdmin
	bob.SetHidden(true)
	tell := NewTell(sessions, nil)

	runCmd(t, tell, alice, "Bob hi there")

	if !strings.Contains(aOut.String(), "no one by that name") {
		t.Fatalf("expected offline-shaped error to alice; got %q", aOut.String())
	}
	if strings.Contains(bOut.String(), "Alice tells you") {
		t.Fatalf("hidden admin still received tell: %q", bOut.String())
	}
}

func TestTell_HiddenAdminVisibleToOtherAdmin(t *testing.T) {
	sessions, alice, bob, _, bOut := commPair(t)
	alice.AuthLevel = telnet.AuthAdmin
	bob.AuthLevel = telnet.AuthAdmin
	bob.SetHidden(true)
	tell := NewTell(sessions, nil)

	runCmd(t, tell, alice, "Bob ops chat")

	if !strings.Contains(bOut.String(), "Alice tells you") {
		t.Fatalf("admin->hidden-admin tell did not deliver: %q", bOut.String())
	}
}

func TestOnlineNameCandidates_FiltersHiddenForNonAdmin(t *testing.T) {
	sessions, alice, bob, _, _ := commPair(t)
	bob.AuthLevel = telnet.AuthAdmin
	bob.SetHidden(true)

	// Non-admin viewer (alice): no candidates for "Bo".
	got := onlineNameCandidates(alice, sessions, "Bo")
	for _, c := range got {
		if strings.EqualFold(c.Text, "Bob") {
			t.Fatalf("non-admin completer leaked hidden admin: %+v", got)
		}
	}

	// Admin viewer sees Bob normally.
	alice.AuthLevel = telnet.AuthAdmin
	got = onlineNameCandidates(alice, sessions, "Bo")
	found := false
	for _, c := range got {
		if strings.EqualFold(c.Text, "Bob") {
			found = true
		}
	}
	if !found {
		t.Fatalf("admin completer missing hidden admin Bob: %+v", got)
	}
}
