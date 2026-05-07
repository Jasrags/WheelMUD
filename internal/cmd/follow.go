package cmd

import (
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/group"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewFollow builds the `follow <player>` verb. With no argument, or
// with a target naming the actor themself, the verb stops following.
//
// Refused unless the target shares a party with the actor (slice 3
// invariant), is in the same room, and the relationship would not
// introduce a cycle. The chain that auto-moves followers lives in
// move.go::chainFollowers; this verb only sets the relationship.
func NewFollow(groups *group.Manager, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name: "follow",
		Help: "follow <player> — auto-move when your party leader does",
		Long: "Usage: follow <player>\n" +
			"       follow (no args) — stop following\n" +
			"       Target must be a member of your party in the same room.\n",
		Auth: telnet.AuthPlayer,
		Completer: func(s *telnet.Session, args string) []telnet.Candidate {
			slot, partial := completerSlot(args)
			if slot != 0 {
				return nil
			}
			return onlineNameCandidates(s, sessions, partial)
		},
		Run: func(c *telnet.Context) error {
			s := c.Session
			if len(c.Args) == 0 {
				return clearFollow(s, "stop")
			}
			target := strings.Join(c.Args, " ")
			peer, ok := MatchPlayer(target, sessions, s)
			if !ok || peer == nil {
				return s.WriteString("{{You don't see them here.}}::yellow\r\n")
			}
			if peer.CharacterID == s.CharacterID {
				return clearFollow(s, "stop")
			}
			if groups == nil || !groups.SameGroup(s.CharacterID, peer.CharacterID) {
				return s.WriteString("{{You can only follow members of your own party.}}::yellow\r\n")
			}
			if introducesFollowCycle(s, peer, sessions) {
				return s.WriteString("{{That would tangle the line — you can't follow them.}}::yellow\r\n")
			}
			s.SetFollowing(peer.CharacterID)
			leaderName := safePeer(peer)
			if err := peer.WriteAsync(
				"{{" + safeActor(s) + " starts following you.}}::cyan\r\n"); err != nil {
				slog.Debug("follow: leader notify failed", "to", peer.CharacterName, "error", err)
			}
			return s.WriteString("{{You start following " + leaderName + ".}}::cyan\r\n")
		},
	}
}

// NewUnfollow is a tiny convenience verb so `unfollow` doesn't have
// to type-through `follow` with no args. Equivalent to bare-arg
// `follow`.
func NewUnfollow() *telnet.Command {
	return &telnet.Command{
		Name: "unfollow",
		Help: "unfollow — stop following whoever you're tailing",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			return clearFollow(c.Session, "unfollow")
		},
	}
}

func clearFollow(s *telnet.Session, _ string) error {
	if s.Following() == 0 {
		return s.WriteString("{{You aren't following anyone.}}::yellow\r\n")
	}
	s.SetFollowing(0)
	return s.WriteString("{{You stop following.}}::yellow\r\n")
}

// introducesFollowCycle walks the proposed leader's existing follow
// chain. If the chain points back at the would-be follower, the
// follow is refused. Bounded by maxFollowDepth so a pre-existing
// orphan-cycle (shouldn't be possible, defence-in-depth) terminates.
func introducesFollowCycle(self, leader *telnet.Session, sessions *session.Registry) bool {
	if sessions == nil || self == nil || leader == nil {
		return false
	}
	cur := leader
	for i := 0; i < maxFollowDepth; i++ {
		nextID := cur.Following()
		if nextID == 0 {
			return false
		}
		if nextID == self.CharacterID {
			return true
		}
		next := findSessionByCharacterID(sessions, nextID)
		if next == nil {
			return false
		}
		cur = next
	}
	return true
}

// findSessionByCharacterID is a small helper because the registry
// is keyed by AccountID and follow-chain hops need CharacterID.
func findSessionByCharacterID(sessions *session.Registry, charID int64) *telnet.Session {
	for _, peer := range sessions.Snapshot() {
		if peer != nil && peer.CharacterID == charID {
			return peer
		}
	}
	return nil
}
