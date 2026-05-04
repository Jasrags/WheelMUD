package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// zoneTrio extends commPair with a third session in a different zone.
// alice (room 1, zone 1), bob (room 2, zone 1 — different room, same
// zone), carol (room 3, zone 2 — different zone). The room repo is
// pre-seeded so peers resolve through FindByID without hitting
// ErrRoomNotFound.
func zoneTrio(t *testing.T) (sessions *session.Registry, rooms *repo.MemoryRoomRepo, alice, bob, carol *telnet.Session, aOut, bOut, cOut *bufConn) {
	t.Helper()
	sessions, alice, bob, aOut, bOut = commPair(t)
	bob.CurrentRoomID = 2

	carol, cOut = bufSession(t)
	carol.AccountID = 300
	carol.AuthLevel = telnet.AuthPlayer
	carol.CharacterID = 3
	carol.CharacterName = "Carol"
	carol.CurrentRoomID = 3
	sessions.Bind(carol.AccountID, carol)

	rooms = repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, Name: "Town Square", ZoneID: 1})
	rooms.Insert(repo.Room{ID: 2, Name: "Market Street", ZoneID: 1})
	rooms.Insert(repo.Room{ID: 3, Name: "Distant Forest", ZoneID: 2})
	return
}

func TestShout_ReachesSameZonePeer(t *testing.T) {
	sessions, rooms, alice, _, _, _, bOut, cOut := zoneTrio(t)
	shout := NewShout(sessions, rooms)

	runCmd(t, shout, alice, "the gates are open!")

	if !strings.Contains(bOut.String(), "Alice shouts,") {
		t.Fatalf("bob: missing speaker line; got %q", bOut.String())
	}
	if !strings.Contains(bOut.String(), "the gates are open!") {
		t.Fatalf("bob: missing payload; got %q", bOut.String())
	}
	if strings.Contains(cOut.String(), "the gates are open") {
		t.Fatalf("carol heard a cross-zone shout: %q", cOut.String())
	}
}

func TestShout_EchoesToSender(t *testing.T) {
	sessions, rooms, alice, _, _, aOut, _, _ := zoneTrio(t)
	shout := NewShout(sessions, rooms)

	runCmd(t, shout, alice, "hello zone")

	if !strings.Contains(aOut.String(), "You shout,") {
		t.Fatalf("alice: missing self echo; got %q", aOut.String())
	}
	if !strings.Contains(aOut.String(), "hello zone") {
		t.Fatalf("alice: missing payload; got %q", aOut.String())
	}
}

func TestYell_ReachesSameZoneWithRedFlavor(t *testing.T) {
	sessions, rooms, alice, _, _, aOut, bOut, _ := zoneTrio(t)
	yell := NewYell(sessions, rooms)

	runCmd(t, yell, alice, "fire!")

	if !strings.Contains(aOut.String(), "You yell,") {
		t.Fatalf("alice: missing self echo; got %q", aOut.String())
	}
	if !strings.Contains(bOut.String(), "Alice yells,") {
		t.Fatalf("bob: missing speaker line; got %q", bOut.String())
	}
	if !strings.Contains(bOut.String(), "fire!") {
		t.Fatalf("bob: missing payload; got %q", bOut.String())
	}
}

func TestShout_EmptyMessageReturnsPrompt(t *testing.T) {
	sessions, rooms, alice, _, _, aOut, bOut, _ := zoneTrio(t)
	shout := NewShout(sessions, rooms)

	// Drive the empty branch directly: dispatcher would block on
	// MinArgs, but the in-handler sanitizeChat guard is the safety
	// net for whitespace-only payloads.
	ctx := &telnet.Context{
		Ctx:     context.Background(),
		Session: alice,
		Name:    shout.Name,
		Args:    []string{},
		Raw:     "   ",
	}
	if err := shout.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(aOut.String(), "Shout what?") {
		t.Fatalf("alice: missing prompt; got %q", aOut.String())
	}
	if bOut.String() != "" {
		t.Fatalf("bob received output for empty shout: %q", bOut.String())
	}
}

func TestShout_UnscopedRoomRefuses(t *testing.T) {
	sessions, rooms, alice, _, _, aOut, bOut, _ := zoneTrio(t)
	rooms.Insert(repo.Room{ID: 1, Name: "Limbo", ZoneID: 0}) // overwrite zone
	shout := NewShout(sessions, rooms)

	runCmd(t, shout, alice, "anyone?")

	if !strings.Contains(aOut.String(), "could carry") {
		t.Fatalf("alice: missing unscoped error; got %q", aOut.String())
	}
	if strings.Contains(bOut.String(), "anyone?") {
		t.Fatalf("bob heard an unscoped shout: %q", bOut.String())
	}
}

func TestShout_SilentRoomMuffles(t *testing.T) {
	sessions, rooms, alice, _, _, aOut, bOut, _ := zoneTrio(t)
	rooms.Insert(repo.Room{ID: 1, Name: "Hush Chapel", ZoneID: 1, Flags: repo.RoomFlags{Silent: true}})
	shout := NewShout(sessions, rooms)

	runCmd(t, shout, alice, "anyone?")

	if !strings.Contains(aOut.String(), "smothers your voice") {
		t.Fatalf("alice: missing silent message; got %q", aOut.String())
	}
	if strings.Contains(bOut.String(), "anyone?") {
		t.Fatalf("bob heard a shout from a silent room: %q", bOut.String())
	}
}

func TestShout_NoCurrentRoomIsFriendlyError(t *testing.T) {
	sessions, rooms, alice, _, _, aOut, bOut, _ := zoneTrio(t)
	alice.CurrentRoomID = 0
	shout := NewShout(sessions, rooms)

	runCmd(t, shout, alice, "anyone there?")

	if !strings.Contains(aOut.String(), "voice goes nowhere") {
		t.Fatalf("alice: missing no-room error; got %q", aOut.String())
	}
	if bOut.String() != "" {
		t.Fatalf("bob received output for nowhere shout: %q", bOut.String())
	}
}

func TestShout_PeerReceivesOtherFormNotSelfForm(t *testing.T) {
	sessions, rooms, alice, _, _, _, bOut, _ := zoneTrio(t)
	shout := NewShout(sessions, rooms)

	runCmd(t, shout, alice, "across the way")

	if strings.Contains(bOut.String(), "You shout,") {
		t.Fatalf("bob received self-form line: %q", bOut.String())
	}
	if !strings.Contains(bOut.String(), "Alice shouts,") {
		t.Fatalf("bob missing other-form line: %q", bOut.String())
	}
}

func TestShout_SkipsPeersWithoutCharacter(t *testing.T) {
	sessions, rooms, alice, _, _, _, _, _ := zoneTrio(t)
	// Add a pre-login peer (no character bound). Snapshot must
	// include it without panicking the broadcast.
	ghost, _ := bufSession(t)
	ghost.AccountID = 400
	ghost.AuthLevel = telnet.AuthGuest
	sessions.Bind(ghost.AccountID, ghost)

	shout := NewShout(sessions, rooms)
	runCmd(t, shout, alice, "ping")
	// no assertion needed beyond no-panic; ghost would have nil-ish
	// CurrentRoomID and the fanout must skip it cleanly.
}
