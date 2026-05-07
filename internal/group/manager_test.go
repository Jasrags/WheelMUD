package group

import (
	"errors"
	"sort"
	"testing"
)

func TestManager_InviteAcceptRoundTrip(t *testing.T) {
	m := New()
	if err := m.Invite(1, "Alice", 2, "Bob"); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if !m.SameGroup(1, 1) || m.SameGroup(1, 2) {
		t.Fatal("Bob should not be in-group before accept")
	}
	leaderID, leaderName, ok := m.PendingInvite(2)
	if !ok || leaderID != 1 || leaderName != "Alice" {
		t.Fatalf("PendingInvite = (%d,%q,%v), want (1,Alice,true)", leaderID, leaderName, ok)
	}
	g, err := m.Accept(2, "Bob")
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if g.Leader != 1 || len(g.Members) != 2 {
		t.Fatalf("group = %+v, want leader=1 size=2", g)
	}
	if !m.SameGroup(1, 2) || !m.SameGroup(2, 1) {
		t.Fatal("Alice + Bob should share a group post-accept")
	}
	if _, _, still := m.PendingInvite(2); still {
		t.Fatal("invite should be cleared after accept")
	}
}

func TestManager_DoubleInviteSameTarget(t *testing.T) {
	m := New()
	if err := m.Invite(1, "Alice", 2, "Bob"); err != nil {
		t.Fatalf("first Invite: %v", err)
	}
	err := m.Invite(1, "Alice", 2, "Bob")
	if !errors.Is(err, ErrInviteeBusy) {
		t.Fatalf("second Invite err = %v, want ErrInviteeBusy", err)
	}
}

func TestManager_InviteAlreadyGrouped(t *testing.T) {
	m := New()
	mustInviteAccept(t, m, 1, "Alice", 2, "Bob")
	err := m.Invite(3, "Cara", 2, "Bob")
	if !errors.Is(err, ErrAlreadyGrouped) {
		t.Fatalf("Invite already-grouped err = %v, want ErrAlreadyGrouped", err)
	}
}

func TestManager_FullGroupRefuses(t *testing.T) {
	m := New()
	for i := int64(2); i <= MaxGroupSize; i++ {
		mustInviteAccept(t, m, 1, "Alice", i, "P"+itoa(i))
	}
	err := m.Invite(1, "Alice", 99, "Late")
	if !errors.Is(err, ErrFull) {
		t.Fatalf("Invite #7 err = %v, want ErrFull", err)
	}
}

func TestManager_SelfInvite(t *testing.T) {
	m := New()
	if err := m.Invite(1, "Alice", 1, "Alice"); !errors.Is(err, ErrSelfInvite) {
		t.Fatalf("self-invite err = %v, want ErrSelfInvite", err)
	}
}

func TestManager_LeaderLeavesDisbands(t *testing.T) {
	m := New()
	mustInviteAccept(t, m, 1, "Alice", 2, "Bob")
	mustInviteAccept(t, m, 1, "Alice", 3, "Cara")
	disbanded, members, err := m.Leave(1)
	if err != nil {
		t.Fatalf("Leave: %v", err)
	}
	if !disbanded {
		t.Fatal("leader leaving must disband")
	}
	sort.Slice(members, func(i, j int) bool { return members[i] < members[j] })
	if len(members) != 3 || members[0] != 1 || members[1] != 2 || members[2] != 3 {
		t.Fatalf("disband members = %v, want [1 2 3]", members)
	}
	if m.Of(1) != nil || m.Of(2) != nil || m.Of(3) != nil {
		t.Fatal("post-disband Of should be nil")
	}
}

func TestManager_MemberLeavesShrinks(t *testing.T) {
	m := New()
	mustInviteAccept(t, m, 1, "Alice", 2, "Bob")
	mustInviteAccept(t, m, 1, "Alice", 3, "Cara")
	disbanded, _, err := m.Leave(2)
	if err != nil {
		t.Fatalf("Leave: %v", err)
	}
	if disbanded {
		t.Fatal("member leaving must not disband")
	}
	g := m.Of(1)
	if g == nil || len(g.Members) != 2 || g.IsMember(2) {
		t.Fatalf("post-leave group = %+v, want size=2 without Bob", g)
	}
}

func TestManager_KickRequiresLeader(t *testing.T) {
	m := New()
	mustInviteAccept(t, m, 1, "Alice", 2, "Bob")
	mustInviteAccept(t, m, 1, "Alice", 3, "Cara")
	if err := m.Kick(2, 3); !errors.Is(err, ErrNotLeader) {
		t.Fatalf("non-leader kick err = %v, want ErrNotLeader", err)
	}
	if err := m.Kick(1, 3); err != nil {
		t.Fatalf("leader Kick: %v", err)
	}
	if m.SameGroup(1, 3) {
		t.Fatal("Cara should be ungrouped after kick")
	}
}

func TestManager_KickNonMember(t *testing.T) {
	m := New()
	mustInviteAccept(t, m, 1, "Alice", 2, "Bob")
	if err := m.Kick(1, 99); !errors.Is(err, ErrNotInGroup) {
		t.Fatalf("kick stranger err = %v, want ErrNotInGroup", err)
	}
}

func TestManager_DeclineClearsInvite(t *testing.T) {
	m := New()
	if err := m.Invite(1, "Alice", 2, "Bob"); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if err := m.Decline(2); err != nil {
		t.Fatalf("Decline: %v", err)
	}
	if _, _, ok := m.PendingInvite(2); ok {
		t.Fatal("invite should be cleared after decline")
	}
	if _, err := m.Accept(2, "Bob"); !errors.Is(err, ErrNoInvite) {
		t.Fatalf("Accept after decline err = %v, want ErrNoInvite", err)
	}
}

func TestManager_DisbandRequiresLeader(t *testing.T) {
	m := New()
	mustInviteAccept(t, m, 1, "Alice", 2, "Bob")
	if _, err := m.Disband(2); !errors.Is(err, ErrNotLeader) {
		t.Fatalf("non-leader Disband err = %v, want ErrNotLeader", err)
	}
	members, err := m.Disband(1)
	if err != nil {
		t.Fatalf("leader Disband: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("Disband members = %v, want size 2", members)
	}
	if m.Of(1) != nil {
		t.Fatal("group should be gone after disband")
	}
}

func TestManager_ClearForCharacterLeader(t *testing.T) {
	m := New()
	mustInviteAccept(t, m, 1, "Alice", 2, "Bob")
	if err := m.Invite(1, "Alice", 3, "Cara"); err != nil {
		t.Fatalf("Invite Cara: %v", err)
	}
	m.ClearForCharacter(1)
	if m.Of(1) != nil || m.Of(2) != nil {
		t.Fatal("leader logout must disband")
	}
	if _, _, ok := m.PendingInvite(3); ok {
		t.Fatal("leader logout must drop their pending invites")
	}
}

func TestManager_ClearForCharacterMember(t *testing.T) {
	m := New()
	mustInviteAccept(t, m, 1, "Alice", 2, "Bob")
	mustInviteAccept(t, m, 1, "Alice", 3, "Cara")
	m.ClearForCharacter(2)
	g := m.Of(1)
	if g == nil || len(g.Members) != 2 || g.IsMember(2) {
		t.Fatalf("post-clear group = %+v, want Bob removed", g)
	}
}

func TestManager_SameGroupSelf(t *testing.T) {
	m := New()
	if !m.SameGroup(7, 7) {
		t.Fatal("self should always be co-grouped with self")
	}
	if m.SameGroup(0, 0) {
		t.Fatal("zero ids must not be co-grouped")
	}
}

func TestManager_AcceptNoInvite(t *testing.T) {
	m := New()
	if _, err := m.Accept(2, "Bob"); !errors.Is(err, ErrNoInvite) {
		t.Fatalf("Accept without invite err = %v, want ErrNoInvite", err)
	}
}

func TestManager_AcceptAfterLeaderDisbanded(t *testing.T) {
	m := New()
	if err := m.Invite(1, "Alice", 2, "Bob"); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	// Leader disbands by leaving solo group.
	if _, _, err := m.Leave(1); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	if _, err := m.Accept(2, "Bob"); !errors.Is(err, ErrNoInvite) {
		t.Fatalf("Accept after leader gone err = %v, want ErrNoInvite", err)
	}
}

// --- helpers ---

func mustInviteAccept(t *testing.T, m *Manager, leaderID int64, leaderName string, inviteeID int64, inviteeName string) {
	t.Helper()
	if err := m.Invite(leaderID, leaderName, inviteeID, inviteeName); err != nil {
		t.Fatalf("Invite %s→%s: %v", leaderName, inviteeName, err)
	}
	if _, err := m.Accept(inviteeID, inviteeName); err != nil {
		t.Fatalf("Accept %s: %v", inviteeName, err)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
