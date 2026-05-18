package cmd

import (
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/telnet"
)

func TestEmote_BroadcastsToRoom(t *testing.T) {
	sessions, alice, _, aOut, bOut := commPair(t)
	em := NewEmote(sessions)

	runCmd(t, em, alice, "tilts her head, considering.")

	if !strings.Contains(aOut.String(), "You tilts her head, considering.") {
		t.Fatalf("alice self: %q", aOut.String())
	}
	if !strings.Contains(bOut.String(), "Alice tilts her head, considering.") {
		t.Fatalf("bob other: %q", bOut.String())
	}
	if !strings.Contains(aOut.String(), "\x1b[35m") {
		t.Fatalf("alice output missing purple style: %q", aOut.String())
	}
}

func TestEmote_EmptyArgRejected(t *testing.T) {
	sessions, alice, _, aOut, bOut := commPair(t)
	em := NewEmote(sessions)

	// Dispatcher enforces MinArgs=1 normally — runCmd bypasses that
	// gate, but the runtime sanitizer must still refuse a blank body.
	runCmd(t, em, alice, "   ")

	if !strings.Contains(aOut.String(), "Emote what?") {
		t.Fatalf("alice: %q", aOut.String())
	}
	if bOut.String() != "" {
		t.Fatalf("bob should not have received anything: %q", bOut.String())
	}
}

func TestEmote_DefangsCfmtPayload(t *testing.T) {
	sessions, alice, _, _, bOut := commPair(t)
	em := NewEmote(sessions)

	runCmd(t, em, alice, "{{whispers}}::red")

	// Payload's `{{`/`}}`/`::` should be split by sanitizeChat so the
	// outer purple tag is the only style applied. Bob's line must
	// NOT contain a literal `}}::red` that would close + restyle.
	if strings.Contains(bOut.String(), "}}::red") {
		t.Fatalf("cfmt payload not defused: %q", bOut.String())
	}
	// Outer purple tag still present.
	if !strings.Contains(bOut.String(), "\x1b[35m") {
		t.Fatalf("purple style missing: %q", bOut.String())
	}
}

func TestEmote_HasColonAlias(t *testing.T) {
	em := NewEmote(nil)
	found := false
	for _, a := range em.Aliases {
		if a == ":" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ':' alias on emote, got %v", em.Aliases)
	}
	if em.Auth != telnet.AuthPlayer {
		t.Fatalf("emote should require AuthPlayer, got %v", em.Auth)
	}
}
