// Package group manages player parties — invite / accept / kick /
// leave plumbing plus shared lookups used by combat XP split, the
// `follow` verb, and the same-group PvP guard. State is process-
// level and in-memory; a server restart drops every group, the same
// trade-off as in-flight Fights in internal/combat.
package group

import "errors"

var (
	// ErrAlreadyGrouped is returned when a player is invited to or
	// tries to accept an invite while already in a group.
	ErrAlreadyGrouped = errors.New("group: character already in a group")

	// ErrFull is returned when a group is already at MaxGroupSize.
	ErrFull = errors.New("group: at capacity")

	// ErrNotInGroup is returned by Leave / Kick when the target is
	// not a member of any group (or not the named group).
	ErrNotInGroup = errors.New("group: character is not in a group")

	// ErrNotLeader is returned by Kick / Disband when the caller
	// is not the group's leader.
	ErrNotLeader = errors.New("group: only the leader may do that")

	// ErrNoInvite is returned by Accept / Decline when no pending
	// invite exists for the invitee.
	ErrNoInvite = errors.New("group: no pending invite")

	// ErrSelfInvite is returned by Invite when the leader and
	// invitee are the same character.
	ErrSelfInvite = errors.New("group: cannot invite yourself")

	// ErrInviteeOffline is returned by Invite when the invitee is
	// already grouped or otherwise can't receive — currently only
	// the "already in a group" branch surfaces this; reserved for
	// a presence check follow-up.
	ErrInviteeBusy = errors.New("group: invitee already has a pending invite")
)

// MaxGroupSize caps how many characters a single group can hold,
// leader inclusive. D&D-MUD convention; configurable later.
const MaxGroupSize = 6
