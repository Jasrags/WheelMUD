package group

import (
	"sync"
	"time"

	"github.com/Jasrags/WheelMUD/internal/session"
)

// Group is a snapshot of a single party. The pointer returned by
// Manager.Of is a copy — callers may not mutate the live group via
// the returned value; mutations go through Manager methods.
type Group struct {
	Leader  int64            // CharacterID of the leader
	Members map[int64]string // charID → display name (includes leader)
	Created time.Time
}

// IsMember reports whether charID is in the group.
func (g Group) IsMember(charID int64) bool {
	_, ok := g.Members[charID]
	return ok
}

// invite is a pending-invite record. Stored in the manager keyed by
// invitee CharacterID.
type invite struct {
	leaderID   int64
	leaderName string
	at         time.Time
}

// Manager is the process-level group registry. Concurrent-safe.
// Mirrors internal/combat.Manager's mu+map shape.
type Manager struct {
	mu          sync.Mutex
	groups      map[int64]*Group // keyed by leader CharacterID
	byCharacter map[int64]int64  // charID → leader CharacterID
	invites     map[int64]invite // invitee CharacterID → pending invite
	now         func() time.Time
}

// New constructs an empty Manager.
func New() *Manager {
	return &Manager{
		groups:      make(map[int64]*Group),
		byCharacter: make(map[int64]int64),
		invites:     make(map[int64]invite),
		now:         time.Now,
	}
}

// Of returns a snapshot of the group containing charID, or nil. The
// returned *Group is a copy; mutations on it do not affect the
// registry.
func (m *Manager) Of(charID int64) *Group {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.copyByMember(charID)
}

// SameGroup reports whether a and b are in the same group. Distinct
// non-zero ids that share a leader return true; a==b returns true
// when both are in any group (a character is always co-grouped with
// themself for caller convenience). Zero ids return false.
func (m *Manager) SameGroup(a, b int64) bool {
	if a == 0 || b == 0 {
		return false
	}
	if a == b {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	la, oka := m.byCharacter[a]
	lb, okb := m.byCharacter[b]
	return oka && okb && la == lb
}

// Invite records a pending invite from leader to invitee. If the
// leader has no group yet, one is created with leader as the sole
// member. Returns ErrSelfInvite, ErrAlreadyGrouped (invitee already
// in a group), ErrFull (leader's group is at capacity), or
// ErrInviteeBusy (invitee already has a pending invite).
func (m *Manager) Invite(leaderID int64, leaderName string, inviteeID int64, inviteeName string) error {
	if leaderID == inviteeID {
		return ErrSelfInvite
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.byCharacter[inviteeID]; exists {
		return ErrAlreadyGrouped
	}
	if _, pending := m.invites[inviteeID]; pending {
		return ErrInviteeBusy
	}

	g := m.ensureGroup(leaderID, leaderName)
	if len(g.Members) >= MaxGroupSize {
		return ErrFull
	}

	m.invites[inviteeID] = invite{
		leaderID:   leaderID,
		leaderName: leaderName,
		at:         m.now(),
	}
	return nil
}

// PendingInvite returns the pending invite for inviteeID, if any.
func (m *Manager) PendingInvite(inviteeID int64) (leaderID int64, leaderName string, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inv, ok := m.invites[inviteeID]
	if !ok {
		return 0, "", false
	}
	return inv.leaderID, inv.leaderName, true
}

// Accept moves the invitee into the leader's group and clears the
// pending invite. Returns ErrNoInvite if no invite exists,
// ErrAlreadyGrouped if the invitee somehow joined another group
// since the invite, or ErrFull if the leader's group filled up
// before the accept landed.
func (m *Manager) Accept(inviteeID int64, inviteeName string) (Group, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inv, ok := m.invites[inviteeID]
	if !ok {
		return Group{}, ErrNoInvite
	}
	if _, exists := m.byCharacter[inviteeID]; exists {
		delete(m.invites, inviteeID)
		return Group{}, ErrAlreadyGrouped
	}
	g, ok := m.groups[inv.leaderID]
	if !ok {
		// Leader disbanded between invite and accept — treat as
		// no-invite.
		delete(m.invites, inviteeID)
		return Group{}, ErrNoInvite
	}
	if len(g.Members) >= MaxGroupSize {
		return Group{}, ErrFull
	}
	g.Members[inviteeID] = inviteeName
	m.byCharacter[inviteeID] = inv.leaderID
	delete(m.invites, inviteeID)
	return *m.copyGroup(g), nil
}

// Decline drops a pending invite without joining. Returns
// ErrNoInvite when no invite exists.
func (m *Manager) Decline(inviteeID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.invites[inviteeID]; !ok {
		return ErrNoInvite
	}
	delete(m.invites, inviteeID)
	return nil
}

// Leave removes member from their group. If the leader leaves, the
// group disbands and disbanded=true is returned. Returns
// ErrNotInGroup when member is not in any group.
func (m *Manager) Leave(memberID int64) (disbanded bool, members []int64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	leaderID, ok := m.byCharacter[memberID]
	if !ok {
		return false, nil, ErrNotInGroup
	}
	g := m.groups[leaderID]
	if memberID == leaderID {
		members = m.disbandLocked(leaderID)
		return true, members, nil
	}
	delete(g.Members, memberID)
	delete(m.byCharacter, memberID)
	return false, m.memberIDsLocked(g), nil
}

// Kick removes target from leader's group. Returns ErrNotLeader if
// caller is not the group's leader and ErrNotInGroup if target is
// not a member.
func (m *Manager) Kick(leaderID, targetID int64) error {
	if leaderID == targetID {
		return ErrNotInGroup
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.groups[leaderID]
	if !ok {
		return ErrNotLeader
	}
	if _, ok := g.Members[targetID]; !ok {
		return ErrNotInGroup
	}
	delete(g.Members, targetID)
	delete(m.byCharacter, targetID)
	return nil
}

// Disband ends a group. Returns ErrNotLeader if leaderID is not
// actually the leader of any group.
func (m *Manager) Disband(leaderID int64) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.groups[leaderID]; !ok {
		return nil, ErrNotLeader
	}
	return m.disbandLocked(leaderID), nil
}

// MembersInRoom returns the CharacterIDs of every group member who
// is currently in roomID, including charID themselves if present.
// A solo character returns just [charID]. Returns nil if charID is
// 0 or sessions is nil.
func (m *Manager) MembersInRoom(charID int64, roomID int64, sessions *session.Registry) []int64 {
	if charID == 0 {
		return nil
	}
	m.mu.Lock()
	leaderID, grouped := m.byCharacter[charID]
	var memberIDs map[int64]struct{}
	if grouped {
		g := m.groups[leaderID]
		memberIDs = make(map[int64]struct{}, len(g.Members))
		for id := range g.Members {
			memberIDs[id] = struct{}{}
		}
	}
	m.mu.Unlock()

	if !grouped {
		return []int64{charID}
	}
	if sessions == nil {
		// Without a session registry we can't enforce in-room
		// filtering; fall back to "just the asker".
		return []int64{charID}
	}
	out := make([]int64, 0, len(memberIDs))
	for _, sess := range sessions.Snapshot() {
		if sess.CurrentRoomID != roomID {
			continue
		}
		if _, ok := memberIDs[sess.CharacterID]; !ok {
			continue
		}
		out = append(out, sess.CharacterID)
	}
	if len(out) == 0 {
		// Asker themselves wasn't in sessions yet (mid-promote);
		// at minimum return the asker so callers don't get an
		// empty split that would award no XP.
		out = []int64{charID}
	}
	return out
}

// ClearForCharacter handles logout / character delete: drops any
// pending invite for charID, removes charID from any group (with
// leader-leaves semantics), and drops invites the character
// authored as leader by disbanding their group. Safe to call for a
// character with no group state.
func (m *Manager) ClearForCharacter(charID int64) {
	if charID == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.invites, charID)
	leaderID, ok := m.byCharacter[charID]
	if !ok {
		return
	}
	if leaderID == charID {
		m.disbandLocked(leaderID)
		return
	}
	g := m.groups[leaderID]
	delete(g.Members, charID)
	delete(m.byCharacter, charID)
}

// --- internal helpers (lock held) ---

func (m *Manager) ensureGroup(leaderID int64, leaderName string) *Group {
	if g, ok := m.groups[leaderID]; ok {
		return g
	}
	g := &Group{
		Leader:  leaderID,
		Members: map[int64]string{leaderID: leaderName},
		Created: m.now(),
	}
	m.groups[leaderID] = g
	m.byCharacter[leaderID] = leaderID
	return g
}

func (m *Manager) disbandLocked(leaderID int64) []int64 {
	g, ok := m.groups[leaderID]
	if !ok {
		return nil
	}
	out := make([]int64, 0, len(g.Members))
	for id := range g.Members {
		out = append(out, id)
		delete(m.byCharacter, id)
	}
	delete(m.groups, leaderID)
	// Also drop any pending invites authored by this leader.
	for invitee, inv := range m.invites {
		if inv.leaderID == leaderID {
			delete(m.invites, invitee)
		}
	}
	return out
}

func (m *Manager) copyByMember(charID int64) *Group {
	leaderID, ok := m.byCharacter[charID]
	if !ok {
		return nil
	}
	g := m.groups[leaderID]
	if g == nil {
		return nil
	}
	return m.copyGroup(g)
}

func (m *Manager) copyGroup(g *Group) *Group {
	out := &Group{
		Leader:  g.Leader,
		Members: make(map[int64]string, len(g.Members)),
		Created: g.Created,
	}
	for id, name := range g.Members {
		out.Members[id] = name
	}
	return out
}

func (m *Manager) memberIDsLocked(g *Group) []int64 {
	out := make([]int64, 0, len(g.Members))
	for id := range g.Members {
		out = append(out, id)
	}
	return out
}
