package cmd

import (
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/group"
)

func TestGroup_InviteAcceptRoundTrip(t *testing.T) {
	sessions, alice, bob, aOut, bOut := commPair(t)
	groups := group.New()
	c := NewGroup(groups, sessions)

	runCmd(t, c, alice, "invite bob")
	if got := aOut.String(); !strings.Contains(got, "invite Bob to your party") {
		t.Fatalf("alice self echo missing: %q", got)
	}
	if got := bOut.String(); !strings.Contains(got, "Alice invites you") {
		t.Fatalf("bob notify missing: %q", got)
	}

	bOut.Reset()
	runCmd(t, c, bob, "accept")
	if got := bOut.String(); !strings.Contains(got, "You join") {
		t.Fatalf("bob accept echo missing: %q", got)
	}
	if !groups.SameGroup(alice.CharacterID, bob.CharacterID) {
		t.Fatal("alice + bob should share a group post-accept")
	}
}

func TestGroup_InviteSelfRefused(t *testing.T) {
	// Self-resolution is short-circuited by MatchPlayer's self-skip
	// (keyword.go), so a self-invite surfaces as the "don't see them
	// here" branch rather than ErrSelfInvite. The manager's own
	// self-invite guard is covered in internal/group/manager_test.go.
	sessions, alice, _, aOut, _ := commPair(t)
	groups := group.New()
	c := NewGroup(groups, sessions)

	runCmd(t, c, alice, "invite alice")
	if got := aOut.String(); !strings.Contains(got, "don't see them here") {
		t.Fatalf("self-invite should fall through to no-target: %q", got)
	}
}

func TestGroup_InviteUnknownPlayer(t *testing.T) {
	sessions, alice, _, aOut, _ := commPair(t)
	groups := group.New()
	c := NewGroup(groups, sessions)

	runCmd(t, c, alice, "invite ghostly")
	if got := aOut.String(); !strings.Contains(got, "don't see them here") {
		t.Fatalf("missing target refusal missing: %q", got)
	}
}

func TestGroup_AcceptWithoutInvite(t *testing.T) {
	sessions, _, bob, _, bOut := commPair(t)
	groups := group.New()
	c := NewGroup(groups, sessions)

	runCmd(t, c, bob, "accept")
	if got := bOut.String(); !strings.Contains(got, "no pending party invitation") {
		t.Fatalf("no-invite refusal missing: %q", got)
	}
}

func TestGroup_DeclineClearsInvite(t *testing.T) {
	sessions, alice, bob, _, bOut := commPair(t)
	groups := group.New()
	c := NewGroup(groups, sessions)

	runCmd(t, c, alice, "invite bob")
	bOut.Reset()
	runCmd(t, c, bob, "decline")
	if got := bOut.String(); !strings.Contains(got, "decline the party invitation") {
		t.Fatalf("decline echo missing: %q", got)
	}
	bOut.Reset()
	runCmd(t, c, bob, "accept")
	if got := bOut.String(); !strings.Contains(got, "no pending party invitation") {
		t.Fatalf("post-decline accept should refuse: %q", got)
	}
}

func TestGroup_LeaderLeavesDisbands(t *testing.T) {
	sessions, alice, bob, aOut, bOut := commPair(t)
	groups := group.New()
	c := NewGroup(groups, sessions)

	runCmd(t, c, alice, "invite bob")
	runCmd(t, c, bob, "accept")
	aOut.Reset()
	bOut.Reset()
	runCmd(t, c, alice, "leave")
	if got := aOut.String(); !strings.Contains(got, "disband the party") {
		t.Fatalf("leader disband self echo missing: %q", got)
	}
	if got := bOut.String(); !strings.Contains(got, "Alice disbands the party") {
		t.Fatalf("bob disband notice missing: %q", got)
	}
	if groups.Of(alice.CharacterID) != nil {
		t.Fatal("group should be gone")
	}
}

func TestGroup_MemberLeavesShrinks(t *testing.T) {
	sessions, alice, bob, aOut, _ := commPair(t)
	groups := group.New()
	c := NewGroup(groups, sessions)

	runCmd(t, c, alice, "invite bob")
	runCmd(t, c, bob, "accept")
	aOut.Reset()
	runCmd(t, c, bob, "leave")
	if got := aOut.String(); !strings.Contains(got, "Bob leaves the party") {
		t.Fatalf("alice leave notice missing: %q", got)
	}
	g := groups.Of(alice.CharacterID)
	if g == nil || len(g.Members) != 1 {
		t.Fatalf("group post-leave = %+v, want size 1", g)
	}
}

func TestGroup_KickByLeader(t *testing.T) {
	sessions, alice, bob, _, bOut := commPair(t)
	groups := group.New()
	c := NewGroup(groups, sessions)

	runCmd(t, c, alice, "invite bob")
	runCmd(t, c, bob, "accept")
	bOut.Reset()
	runCmd(t, c, alice, "kick bob")
	if got := bOut.String(); !strings.Contains(got, "kicks you from the party") {
		t.Fatalf("bob kick notice missing: %q", got)
	}
	if groups.SameGroup(alice.CharacterID, bob.CharacterID) {
		t.Fatal("bob should be ungrouped after kick")
	}
}

func TestGroup_KickRequiresLeader(t *testing.T) {
	sessions, alice, bob, _, bOut := commPair(t)
	groups := group.New()
	c := NewGroup(groups, sessions)

	runCmd(t, c, alice, "invite bob")
	runCmd(t, c, bob, "accept")
	bOut.Reset()
	runCmd(t, c, bob, "kick alice")
	if got := bOut.String(); !strings.Contains(got, "Only the party leader") {
		t.Fatalf("non-leader kick refusal missing: %q", got)
	}
}

func TestGroup_DisbandLeaderOnly(t *testing.T) {
	sessions, alice, bob, _, bOut := commPair(t)
	groups := group.New()
	c := NewGroup(groups, sessions)

	runCmd(t, c, alice, "invite bob")
	runCmd(t, c, bob, "accept")
	bOut.Reset()
	runCmd(t, c, bob, "disband")
	if got := bOut.String(); !strings.Contains(got, "Only the party leader") {
		t.Fatalf("non-leader disband refusal missing: %q", got)
	}
}

func TestGroup_RosterShowsMembers(t *testing.T) {
	sessions, alice, bob, aOut, _ := commPair(t)
	groups := group.New()
	c := NewGroup(groups, sessions)

	runCmd(t, c, alice, "invite bob")
	runCmd(t, c, bob, "accept")
	aOut.Reset()
	runCmd(t, c, alice, "")
	got := aOut.String()
	if !strings.Contains(got, "Party roster") {
		t.Fatalf("roster header missing: %q", got)
	}
	if !strings.Contains(got, "Alice") || !strings.Contains(got, "Bob") {
		t.Fatalf("roster member missing: %q", got)
	}
	if !strings.Contains(got, "* Alice") {
		t.Fatalf("leader marker missing: %q", got)
	}
}

func TestGroup_RosterEmpty(t *testing.T) {
	sessions, alice, _, aOut, _ := commPair(t)
	groups := group.New()
	c := NewGroup(groups, sessions)

	runCmd(t, c, alice, "")
	if got := aOut.String(); !strings.Contains(got, "not in a party") {
		t.Fatalf("empty roster line missing: %q", got)
	}
}

func TestGroup_RosterShowsPendingInvite(t *testing.T) {
	sessions, alice, bob, _, bOut := commPair(t)
	groups := group.New()
	c := NewGroup(groups, sessions)

	runCmd(t, c, alice, "invite bob")
	bOut.Reset()
	runCmd(t, c, bob, "")
	if got := bOut.String(); !strings.Contains(got, "pending invite from Alice") {
		t.Fatalf("pending invite hint missing: %q", got)
	}
}
