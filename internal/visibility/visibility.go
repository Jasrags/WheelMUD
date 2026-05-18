// Package visibility centralizes the "can viewer see target?" rule that
// was previously reimplemented across half a dozen callsites in
// internal/cmd. The single source of truth is CanSee; callers that
// snapshot session.Snapshot() should run results through VisiblePeers
// for the filtering pass.
//
// Today's policy is the same one already enforced ad-hoc throughout
// the codebase: a session is visible to itself, every non-hidden
// session is visible to everyone, and a hidden session is visible
// only to admin viewers. Hidden = wizinvis bit on Session.
//
// Centralizing the rule does two things: it closes coverage gaps in
// broadcast paths (combat / shout / yell / movement previously
// rendered to all peers regardless of wizinvis), and it gives a
// single hook for future tightening — e.g. per-zone invisibility,
// PvP-flagged players hidden from each other in safe rooms,
// builder-test invisibility.
package visibility

import "github.com/Jasrags/WheelMUD/telnet"

// CanSee reports whether viewer can see target.
//
// Rules:
//   - target == viewer: always true (a session always sees itself).
//   - target not hidden: always true (default visibility).
//   - target hidden + viewer.AuthLevel >= AuthAdmin: true (admins see
//     each other through wizinvis).
//   - target hidden + viewer non-admin: false.
//
// Nil viewer is treated as a non-admin observer: it sees only
// non-hidden targets. This is defensive — every real callsite has a
// non-nil viewer, but a future broadcast helper running on a
// disconnected session should still produce sensible filtering.
//
// Nil target is treated as not-visible: there is nothing to render.
func CanSee(viewer, target *telnet.Session) bool {
	if target == nil {
		return false
	}
	if target == viewer {
		return true
	}
	if !target.IsHidden() {
		return true
	}
	if viewer == nil {
		return false
	}
	return viewer.AuthLevel >= telnet.AuthAdmin
}

// VisiblePeers filters peers down to the subset viewer can see.
// Preserves order and returns a fresh slice (never aliases the
// input). nil/empty input returns nil. Use this when iterating over
// session.Snapshot() output for a broadcast — the more idiomatic
// alternative to `for ... { if visibility.CanSee(self, peer) { ... } }`
// when the caller already wants a slice.
func VisiblePeers(viewer *telnet.Session, peers []*telnet.Session) []*telnet.Session {
	if len(peers) == 0 {
		return nil
	}
	out := make([]*telnet.Session, 0, len(peers))
	for _, p := range peers {
		if CanSee(viewer, p) {
			out = append(out, p)
		}
	}
	return out
}
