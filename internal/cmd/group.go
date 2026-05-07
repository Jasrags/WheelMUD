package cmd

import (
	"errors"
	"log/slog"
	"sort"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/group"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewGroup builds the `group <subverb>` command. Subverbs:
//
//	group invite <player>   leader-side invite
//	group accept            invitee accepts a pending invite
//	group decline           invitee drops a pending invite
//	group leave             member leaves; leader-leaves disbands
//	group kick <player>     leader removes a member
//	group disband           leader-only end-the-group
//	group                   render the current roster
//
// Slice 1 of Phase D #22. State is in-memory; restart drops every
// group.
func NewGroup(groups *group.Manager, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name: "group",
		Help: "group <invite|accept|decline|leave|kick|disband>",
		Long: "Usage: group invite <player>\n" +
			"       group accept | group decline\n" +
			"       group leave | group disband\n" +
			"       group kick <player>\n" +
			"       group (no args) — show your party roster\n",
		Auth: telnet.AuthPlayer,
		Completer: func(s *telnet.Session, args string) []telnet.Candidate {
			slot, partial := completerSlot(args)
			switch slot {
			case 0:
				return staticCandidates(partial,
					"invite", "accept", "decline", "leave", "kick", "disband")
			case 1:
				toks := strings.Fields(args)
				if len(toks) >= 1 {
					sub := strings.ToLower(toks[0])
					if sub == "invite" || sub == "kick" {
						return onlineNameCandidates(s, sessions, partial)
					}
				}
			}
			return nil
		},
		Run: func(c *telnet.Context) error {
			s := c.Session
			if len(c.Args) == 0 {
				return renderGroupRoster(s, groups, sessions)
			}
			sub := strings.ToLower(c.Args[0])
			rest := c.Args[1:]
			switch sub {
			case "invite":
				return groupInvite(c, groups, sessions, rest)
			case "accept":
				return groupAccept(c, groups, sessions)
			case "decline":
				return groupDecline(c, groups)
			case "leave":
				return groupLeave(c, groups, sessions)
			case "kick":
				return groupKick(c, groups, sessions, rest)
			case "disband":
				return groupDisband(c, groups, sessions)
			default:
				return s.WriteString("{{Unknown group subverb. See `help group`.}}::yellow\r\n")
			}
		},
	}
}

func groupInvite(c *telnet.Context, groups *group.Manager, sessions *session.Registry, rest []string) error {
	s := c.Session
	if len(rest) == 0 {
		return s.WriteString("{{Invite who?}}::yellow\r\n")
	}
	target := strings.Join(rest, " ")
	peer, ok := MatchPlayer(target, sessions, s)
	if !ok || peer == nil {
		return s.WriteString("{{You don't see them here.}}::yellow\r\n")
	}
	leaderName := safeActor(s)
	inviteeName := safePeer(peer)
	if err := groups.Invite(s.CharacterID, s.CharacterName, peer.CharacterID, peer.CharacterName); err != nil {
		return s.WriteString(groupErrLine(err, inviteeName))
	}
	if err := peer.WriteAsync(
		"{{" + leaderName + " invites you to join their party. Type `group accept` or `group decline`.}}::cyan\r\n",
	); err != nil {
		slog.Debug("group: invitee notify failed", "to", peer.CharacterName, "error", err)
	}
	return s.WriteString("{{You invite " + inviteeName + " to your party.}}::cyan\r\n")
}

func groupAccept(c *telnet.Context, groups *group.Manager, sessions *session.Registry) error {
	s := c.Session
	_, leaderName, ok := groups.PendingInvite(s.CharacterID)
	if !ok {
		return s.WriteString("{{You have no pending party invitation.}}::yellow\r\n")
	}
	g, err := groups.Accept(s.CharacterID, s.CharacterName)
	if err != nil {
		return s.WriteString(groupErrLine(err, leaderName))
	}
	memberName := safeActor(s)
	notifyMembers(sessions, g, s.CharacterID,
		"{{"+memberName+" has joined the party.}}::cyan\r\n")
	return s.WriteString("{{You join " + display.Defang(leaderName, "the leader") + "'s party.}}::cyan\r\n")
}

func groupDecline(c *telnet.Context, groups *group.Manager) error {
	s := c.Session
	if err := groups.Decline(s.CharacterID); err != nil {
		return s.WriteString(groupErrLine(err, ""))
	}
	return s.WriteString("{{You decline the party invitation.}}::yellow\r\n")
}

func groupLeave(c *telnet.Context, groups *group.Manager, sessions *session.Registry) error {
	s := c.Session
	disbanded, members, err := groups.Leave(s.CharacterID)
	if err != nil {
		return s.WriteString(groupErrLine(err, ""))
	}
	leaver := safeActor(s)
	if disbanded {
		notifyIDs(sessions, members, s.CharacterID,
			"{{"+leaver+" disbands the party.}}::yellow\r\n")
		return s.WriteString("{{You disband the party.}}::yellow\r\n")
	}
	notifyIDs(sessions, members, s.CharacterID,
		"{{"+leaver+" leaves the party.}}::yellow\r\n")
	return s.WriteString("{{You leave the party.}}::yellow\r\n")
}

func groupKick(c *telnet.Context, groups *group.Manager, sessions *session.Registry, rest []string) error {
	s := c.Session
	if len(rest) == 0 {
		return s.WriteString("{{Kick who?}}::yellow\r\n")
	}
	target := strings.Join(rest, " ")
	g := groups.Of(s.CharacterID)
	if g == nil {
		return s.WriteString(groupErrLine(group.ErrNotInGroup, ""))
	}
	if g.Leader != s.CharacterID {
		return s.WriteString(groupErrLine(group.ErrNotLeader, ""))
	}
	// Resolve target by matching against current members.
	lower := strings.ToLower(target)
	var (
		victimID   int64
		victimName string
	)
	for id, name := range g.Members {
		if id == s.CharacterID {
			continue
		}
		if strings.HasPrefix(strings.ToLower(name), lower) {
			victimID = id
			victimName = name
			break
		}
	}
	if victimID == 0 {
		return s.WriteString("{{No one in your party by that name.}}::yellow\r\n")
	}
	if err := groups.Kick(s.CharacterID, victimID); err != nil {
		return s.WriteString(groupErrLine(err, victimName))
	}
	safeVictim := display.Defang(victimName, "Someone")
	leaverName := safeActor(s)
	// Notify the kicked player.
	if peer := sessions.FindByCharacterName(victimName); peer != nil {
		_ = peer.WriteAsync("{{" + leaverName + " kicks you from the party.}}::yellow\r\n")
	}
	// Notify remaining members.
	if remaining := groups.Of(s.CharacterID); remaining != nil {
		notifyMembers(sessions, *remaining, s.CharacterID,
			"{{"+leaverName+" kicks "+safeVictim+" from the party.}}::yellow\r\n")
	}
	return s.WriteString("{{You kick " + safeVictim + " from the party.}}::yellow\r\n")
}

func groupDisband(c *telnet.Context, groups *group.Manager, sessions *session.Registry) error {
	s := c.Session
	members, err := groups.Disband(s.CharacterID)
	if err != nil {
		return s.WriteString(groupErrLine(err, ""))
	}
	leaver := safeActor(s)
	notifyIDs(sessions, members, s.CharacterID,
		"{{"+leaver+" disbands the party.}}::yellow\r\n")
	return s.WriteString("{{You disband the party.}}::yellow\r\n")
}

func renderGroupRoster(s *telnet.Session, groups *group.Manager, sessions *session.Registry) error {
	g := groups.Of(s.CharacterID)
	if g == nil {
		if leaderID, leaderName, ok := groups.PendingInvite(s.CharacterID); ok && leaderID != 0 {
			return s.WriteString("{{You have a pending invite from " +
				display.Defang(leaderName, "someone") +
				". Type `group accept` or `group decline`.}}::cyan\r\n")
		}
		return s.WriteString("{{You are not in a party.}}::yellow\r\n")
	}
	type row struct {
		id     int64
		name   string
		role   string
		online bool
	}
	rows := make([]row, 0, len(g.Members))
	online := liveCharacterIDs(sessions)
	for id, name := range g.Members {
		role := "member"
		if id == g.Leader {
			role = "leader"
		}
		_, isOnline := online[id]
		rows = append(rows, row{id: id, name: name, role: role, online: isOnline})
	}
	sort.Slice(rows, func(i, j int) bool {
		// leader first, then alpha
		if rows[i].role != rows[j].role {
			return rows[i].role == "leader"
		}
		return strings.ToLower(rows[i].name) < strings.ToLower(rows[j].name)
	})
	var b strings.Builder
	b.WriteString("{{Party roster:}}::cyan\r\n")
	for _, r := range rows {
		marker := "  "
		if r.role == "leader" {
			marker = "* "
		}
		status := "online"
		if !r.online {
			status = "offline"
		}
		b.WriteString("{{")
		b.WriteString(marker)
		b.WriteString(display.Defang(r.name, "Someone"))
		b.WriteString(" (")
		b.WriteString(status)
		b.WriteString(")}}::white\r\n")
	}
	return s.WriteString(b.String())
}

// notifyMembers writes msg via WriteAsync to every online group
// member except excludeID. Failures are debug-logged.
func notifyMembers(sessions *session.Registry, g group.Group, excludeID int64, msg string) {
	if sessions == nil {
		return
	}
	bound := sessions.Snapshot()
	for _, peer := range bound {
		if peer.CharacterID == excludeID {
			continue
		}
		if _, ok := g.Members[peer.CharacterID]; !ok {
			continue
		}
		if err := peer.WriteAsync(msg); err != nil {
			slog.Debug("group: notify failed", "to", peer.CharacterName, "error", err)
		}
	}
}

// notifyIDs is the disband variant — notify every id in members
// except excludeID. Used after Leave/Disband when the group struct
// is already torn down.
func notifyIDs(sessions *session.Registry, members []int64, excludeID int64, msg string) {
	if sessions == nil || len(members) == 0 {
		return
	}
	want := make(map[int64]struct{}, len(members))
	for _, id := range members {
		if id != excludeID {
			want[id] = struct{}{}
		}
	}
	for _, peer := range sessions.Snapshot() {
		if _, ok := want[peer.CharacterID]; !ok {
			continue
		}
		if err := peer.WriteAsync(msg); err != nil {
			slog.Debug("group: notify failed", "to", peer.CharacterName, "error", err)
		}
	}
}

// liveCharacterIDs builds a set of CharacterIDs from the registry.
func liveCharacterIDs(sessions *session.Registry) map[int64]struct{} {
	if sessions == nil {
		return nil
	}
	bound := sessions.Snapshot()
	out := make(map[int64]struct{}, len(bound))
	for _, peer := range bound {
		if peer.CharacterID != 0 {
			out[peer.CharacterID] = struct{}{}
		}
	}
	return out
}

// safePeer returns a peer's defanged display name.
func safePeer(peer *telnet.Session) string {
	if peer == nil {
		return "Someone"
	}
	return display.Defang(peer.CharacterName, "Someone")
}

// groupErrLine maps a group.* error to a player-facing refusal.
// nameContext optionally substitutes the offending party name.
func groupErrLine(err error, nameContext string) string {
	switch {
	case errors.Is(err, group.ErrSelfInvite):
		return "{{You can't invite yourself.}}::yellow\r\n"
	case errors.Is(err, group.ErrAlreadyGrouped):
		if nameContext != "" {
			return "{{" + display.Defang(nameContext, "They") + " is already in a party.}}::yellow\r\n"
		}
		return "{{Already in a party.}}::yellow\r\n"
	case errors.Is(err, group.ErrInviteeBusy):
		if nameContext != "" {
			return "{{" + display.Defang(nameContext, "They") + " has a pending invite.}}::yellow\r\n"
		}
		return "{{They have a pending invite.}}::yellow\r\n"
	case errors.Is(err, group.ErrFull):
		return "{{Your party is full.}}::yellow\r\n"
	case errors.Is(err, group.ErrNotInGroup):
		return "{{You aren't in a party.}}::yellow\r\n"
	case errors.Is(err, group.ErrNotLeader):
		return "{{Only the party leader may do that.}}::yellow\r\n"
	case errors.Is(err, group.ErrNoInvite):
		return "{{You have no pending party invitation.}}::yellow\r\n"
	default:
		return "{{Something went wrong with the party.}}::red\r\n"
	}
}

// staticCandidates filters a fixed list of subverb names by partial
// prefix.
func staticCandidates(partial string, names ...string) []telnet.Candidate {
	lower := strings.ToLower(partial)
	out := make([]telnet.Candidate, 0, len(names))
	for _, n := range names {
		if strings.HasPrefix(n, lower) {
			out = append(out, telnet.Candidate{Text: n})
		}
	}
	return out
}
