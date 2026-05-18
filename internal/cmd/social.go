package cmd

import (
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/emote"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/visibility"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewSocials returns one telnet.Command per social in the catalog,
// ready to register through telnet.Registry.Register. Each command
// participates in the same broadcast/visibility rules as say:
//
//   - actor self-line always rendered locally
//   - other-line broadcast to peers sharing CurrentRoomID
//   - visibility.CanSee filters wizinvis actors out for non-admin peers
//   - room.Silent does NOT gag socials (physical pantomime, not speech)
//
// Targeted forms only run if Social.Targetable() is true. A targeted
// invocation that resolves a target in the same room produces three
// distinct lines (actor / target / others); an unknown name or a
// not-in-room target falls back to an error message and no
// broadcast occurs.
//
// Returns nil for a nil/empty catalog so callers don't have to
// special-case the empty-fs case during boot.
func NewSocials(cat *emote.Catalog, sessions *session.Registry) []*telnet.Command {
	if cat == nil {
		return nil
	}
	socials := cat.All()
	if len(socials) == 0 {
		return nil
	}
	out := make([]*telnet.Command, 0, len(socials))
	for _, s := range socials {
		out = append(out, buildSocialCommand(s, sessions))
	}
	return out
}

func buildSocialCommand(s emote.Social, sessions *session.Registry) *telnet.Command {
	c := &telnet.Command{
		Name:    s.ID,
		Aliases: append([]string(nil), s.Aliases...),
		Help:    s.Help,
		Auth:    telnet.AuthPlayer,
		MinArgs: 0,
		Run: func(c *telnet.Context) error {
			return runSocial(s, sessions, c)
		},
	}
	if s.Targetable() {
		c.Completer = func(self *telnet.Session, args string) []telnet.Candidate {
			slot, partial := completerSlot(args)
			if slot != 0 {
				return nil
			}
			return roomNameCandidates(self, sessions, partial)
		}
	}
	return c
}

func runSocial(s emote.Social, sessions *session.Registry, c *telnet.Context) error {
	actor := c.Session.CharacterName
	if actor == "" {
		actor = "Someone"
	}
	actor = defangSocialName(actor)

	targetArg := strings.TrimSpace(c.Raw)
	if targetArg != "" {
		if !s.Targetable() {
			return c.Session.WriteString("{{You can't " + s.ID + " at someone.}}::yellow\r\n")
		}
		return runTargetedSocial(s, sessions, c, actor, targetArg)
	}

	if c.Session.CurrentRoomID == 0 {
		// No room — only the actor sees their line.
		return c.Session.WriteString(s.RenderSelf(actor))
	}
	otherMsg := s.RenderOther(actor)
	broadcastSocial(sessions, c.Session, otherMsg)
	return c.Session.WriteString(s.RenderSelf(actor))
}

func runTargetedSocial(s emote.Social, sessions *session.Registry, c *telnet.Context, actor, targetArg string) error {
	if sessions == nil {
		return c.Session.WriteString("{{No one by that name is here.}}::yellow\r\n")
	}
	peer := sessions.FindByCharacterName(targetArg)
	if peer == nil {
		return c.Session.WriteString("{{No one by that name is here.}}::yellow\r\n")
	}
	if peer == c.Session {
		// Targeting yourself with a targetable social falls back to
		// the untargeted forms — most MUDs treat `smile self` as
		// equivalent to `smile`, and the alternative ("You smile at
		// yourself.") is rarely what the player meant.
		broadcastSocial(sessions, c.Session, s.RenderOther(actor))
		return c.Session.WriteString(s.RenderSelf(actor))
	}
	_, peerName, peerRoom := peer.InWorld()
	if peerRoom != c.Session.CurrentRoomID {
		return c.Session.WriteString("{{They aren't here.}}::yellow\r\n")
	}
	// wizinvis: a non-admin can't target a hidden admin — pretend
	// they don't exist (same anti-enumeration policy as `tell`).
	if !visibility.CanSee(c.Session, peer) {
		return c.Session.WriteString("{{No one by that name is here.}}::yellow\r\n")
	}
	target := defangSocialName(peerName)
	selfMsg := s.RenderTargetSelf(actor, target)
	viewMsg := s.RenderTargetView(actor, target)
	otherMsg := s.RenderTargetOther(actor, target)

	for _, room := range sessions.Snapshot() {
		if room == c.Session {
			continue
		}
		_, _, rRoom := room.InWorld()
		if rRoom != c.Session.CurrentRoomID {
			continue
		}
		if !visibility.CanSee(room, c.Session) {
			continue
		}
		msg := otherMsg
		if room == peer {
			msg = viewMsg
		}
		if err := room.WriteAsync(msg); err != nil {
			slog.Debug("social: peer write failed", "to", room.CharacterName, "error", err)
		}
	}
	return c.Session.WriteString(selfMsg)
}

// broadcastSocial sends `msg` to every peer in actor's room except
// actor, applying the wizinvis filter. Mirrors the comm.go say loop.
func broadcastSocial(sessions *session.Registry, actor *telnet.Session, msg string) {
	if sessions == nil {
		return
	}
	for _, peer := range sessions.Snapshot() {
		if peer == actor {
			continue
		}
		_, peerName, peerRoom := peer.InWorld()
		if peerRoom != actor.CurrentRoomID {
			continue
		}
		if !visibility.CanSee(peer, actor) {
			continue
		}
		if err := peer.WriteAsync(msg); err != nil {
			slog.Debug("social: peer write failed", "to", peerName, "error", err)
		}
	}
}

// roomNameCandidates returns one Candidate per visible online peer
// sharing the caller's room. Used as the targeted-social Completer.
func roomNameCandidates(self *telnet.Session, sessions *session.Registry, partial string) []telnet.Candidate {
	if sessions == nil || self == nil {
		return nil
	}
	lower := strings.ToLower(partial)
	out := make([]telnet.Candidate, 0, 8)
	for _, peer := range sessions.Snapshot() {
		if peer == self {
			continue
		}
		_, name, room := peer.InWorld()
		if room != self.CurrentRoomID {
			continue
		}
		if name == "" {
			continue
		}
		if peer.AuthLevel > self.AuthLevel {
			continue
		}
		if !visibility.CanSee(self, peer) {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(name), lower) {
			continue
		}
		out = append(out, telnet.Candidate{Text: name, Help: "in room"})
	}
	return out
}

// defangSocialName escapes cfmt template syntax in a name so it can
// be embedded inside `{{...}}::style` without closing the surrounding
// tag or injecting a new style. The zones.go defangCfmt covers
// `{{`/`}}` only — social rendering also has to defuse `::` so a
// character name like `Foo::red` cannot wrest the colour back from
// `purple`. This is belt-and-suspenders today (chargen constrains
// names tightly), but pinning it here makes the threat model
// reviewable without auditing every name producer.
func defangSocialName(s string) string {
	s = strings.ReplaceAll(s, "{{", "{ {")
	s = strings.ReplaceAll(s, "}}", "} }")
	s = strings.ReplaceAll(s, "::", ": :")
	return s
}
