package cmd

import (
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/group"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// followFixture seeds two rooms (1 ↔ 2) with no flags, plus the
// commPair pair Alice (1) and Bob (2). Returns everything the
// follow + move tests need.
func followFixture(t *testing.T) (rooms *repo.MemoryRoomRepo, exits *repo.MemoryExitRepo, items *repo.MemoryItemRepo, mobs *repo.MemoryMobInstanceRepo, chars *repo.MemoryCharacterRepo) {
	t.Helper()
	r, e, i, m := seedWorld(t)
	return r, e, i, m, repo.NewMemoryCharacterRepo()
}

func TestFollow_RequiresSameGroup(t *testing.T) {
	sessions, alice, _, aOut, _ := commPair(t)
	groups := group.New()
	c := NewFollow(groups, sessions)
	runCmd(t, c, alice, "bob")
	if got := aOut.String(); !strings.Contains(got, "members of your own party") {
		t.Fatalf("expected same-party refusal, got %q", got)
	}
	if alice.Following() != 0 {
		t.Fatal("Following must remain 0 after refusal")
	}
}

func TestFollow_StartsAfterGroupJoin(t *testing.T) {
	sessions, alice, bob, _, bOut := commPair(t)
	groups := group.New()
	mustJoinGroup(t, groups, alice.CharacterID, alice.CharacterName, bob.CharacterID, bob.CharacterName)
	c := NewFollow(groups, sessions)
	runCmd(t, c, bob, "alice")
	if bob.Following() != alice.CharacterID {
		t.Fatalf("bob.Following = %d, want alice.id %d", bob.Following(), alice.CharacterID)
	}
	if got := bOut.String(); !strings.Contains(got, "start following Alice") {
		t.Fatalf("missing self echo: %q", got)
	}
}

func TestFollow_NoArgsClears(t *testing.T) {
	sessions, alice, _, aOut, _ := commPair(t)
	alice.SetFollowing(99)
	c := NewFollow(group.New(), sessions)
	runCmd(t, c, alice, "")
	if alice.Following() != 0 {
		t.Fatal("Following must be cleared on bare follow")
	}
	if got := aOut.String(); !strings.Contains(got, "stop following") {
		t.Fatalf("missing stop echo: %q", got)
	}
}

func TestUnfollow_StopsTrailing(t *testing.T) {
	_, alice, _, aOut, _ := commPair(t)
	alice.SetFollowing(99)
	c := NewUnfollow()
	runCmd(t, c, alice, "")
	if alice.Following() != 0 {
		t.Fatal("Following must be 0 after unfollow")
	}
	if got := aOut.String(); !strings.Contains(got, "stop following") {
		t.Fatalf("missing stop echo: %q", got)
	}
}

func TestFollow_RefusesCycle(t *testing.T) {
	sessions, alice, bob, _, bOut := commPair(t)
	groups := group.New()
	mustJoinGroup(t, groups, alice.CharacterID, alice.CharacterName, bob.CharacterID, bob.CharacterName)
	// Alice already follows Bob — Bob asking to follow Alice would
	// close the loop.
	alice.SetFollowing(bob.CharacterID)
	c := NewFollow(groups, sessions)
	runCmd(t, c, bob, "alice")
	if got := bOut.String(); !strings.Contains(got, "tangle") {
		t.Fatalf("missing cycle refusal: %q", got)
	}
	if bob.Following() != 0 {
		t.Fatal("cycle should not have set Following")
	}
}

// TestMove_FollowerChainsThroughExit — the heart of slice 3. Alice
// (leader) and Bob (follower) start in the same room; Bob follows
// Alice; Alice moves north; Bob auto-moves north too.
func TestMove_FollowerChainsThroughExit(t *testing.T) {
	rooms, exits, items, mobs, chars := followFixture(t)
	sessions, alice, bob, _, bOut := commPair(t)
	groups := group.New()
	mustJoinGroup(t, groups, alice.CharacterID, alice.CharacterName, bob.CharacterID, bob.CharacterName)
	bob.SetFollowing(alice.CharacterID)

	family := NewMoveFamily(rooms, exits, items, mobs, chars, nil, noonClock(t), sessions)
	north := findCmd(t, family, "north")
	runCmd(t, north, alice, "")

	if alice.CurrentRoomID != 2 {
		t.Fatalf("alice room = %d, want 2", alice.CurrentRoomID)
	}
	if bob.CurrentRoomID != 2 {
		t.Fatalf("bob room = %d, want 2 (chain failed)", bob.CurrentRoomID)
	}
	// Bob should have received a room render after the chained move.
	if got := bOut.String(); !strings.Contains(got, "North Road") {
		t.Fatalf("bob did not see destination room: %q", got)
	}
}

func TestMove_FollowerNotInSameRoomDoesNotChain(t *testing.T) {
	rooms, exits, items, mobs, chars := followFixture(t)
	sessions, alice, bob, _, _ := commPair(t)
	bob.SetFollowing(alice.CharacterID)
	bob.CurrentRoomID = 2 // pre-located elsewhere

	family := NewMoveFamily(rooms, exits, items, mobs, chars, nil, noonClock(t), sessions)
	north := findCmd(t, family, "north")
	runCmd(t, north, alice, "")

	if alice.CurrentRoomID != 2 {
		t.Fatalf("alice room = %d, want 2", alice.CurrentRoomID)
	}
	// Bob was already in room 2 (where alice ends up); chain shouldn't
	// have moved him further. The relationship should also be intact.
	if bob.CurrentRoomID != 2 {
		t.Fatalf("bob room drifted = %d, want 2", bob.CurrentRoomID)
	}
	if bob.Following() != alice.CharacterID {
		t.Fatal("relationship should persist when follower wasn't co-located")
	}
}

func mustJoinGroup(t *testing.T, m *group.Manager, leaderID int64, leaderName string, inviteeID int64, inviteeName string) {
	t.Helper()
	if err := m.Invite(leaderID, leaderName, inviteeID, inviteeName); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if _, err := m.Accept(inviteeID, inviteeName); err != nil {
		t.Fatalf("Accept: %v", err)
	}
}
