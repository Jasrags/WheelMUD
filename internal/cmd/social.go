package cmd

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/emote"
	"github.com/Jasrags/WheelMUD/internal/repo"
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
func NewSocials(cat *emote.Catalog, sessions *session.Registry, mobs repo.MobInstanceRepo) []*telnet.Command {
	if cat == nil {
		return nil
	}
	socials := cat.All()
	if len(socials) == 0 {
		return nil
	}
	out := make([]*telnet.Command, 0, len(socials))
	for _, s := range socials {
		out = append(out, buildSocialCommand(s, sessions, mobs))
	}
	return out
}

func buildSocialCommand(s emote.Social, sessions *session.Registry, mobs repo.MobInstanceRepo) *telnet.Command {
	c := &telnet.Command{
		Name:    s.ID,
		Aliases: append([]string(nil), s.Aliases...),
		Help:    s.Help,
		Auth:    telnet.AuthPlayer,
		MinArgs: 0,
		Run: func(c *telnet.Context) error {
			return runSocial(s, sessions, mobs, c)
		},
	}
	if s.Targetable() {
		c.Completer = func(self *telnet.Session, args string) []telnet.Candidate {
			slot, partial := completerSlot(args)
			if slot != 0 {
				return nil
			}
			return socialTargetCandidates(self, sessions, mobs, partial)
		}
	}
	return c
}

func runSocial(s emote.Social, sessions *session.Registry, mobs repo.MobInstanceRepo, c *telnet.Context) error {
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
		return runTargetedSocial(s, sessions, mobs, c, actor, targetArg)
	}

	if c.Session.CurrentRoomID == 0 {
		// No room — only the actor sees their line.
		return c.Session.WriteString(s.RenderSelf(actor))
	}
	otherMsg := s.RenderOther(actor)
	broadcastSocial(sessions, c.Session, otherMsg)
	return c.Session.WriteString(s.RenderSelf(actor))
}

func runTargetedSocial(s emote.Social, sessions *session.Registry, mobs repo.MobInstanceRepo, c *telnet.Context, actor, targetArg string) error {
	// Mob match first, mirroring `attack`'s precedence so a player
	// named after a mob in the same room doesn't shadow the obvious
	// target. Mobs are looked up by name token / ordinal via the
	// shared MatchMob helper (room-scoped).
	if mobs != nil && c.Session.CurrentRoomID != 0 {
		ctx, cancel := context.WithTimeout(c.Ctx, 2*time.Second)
		roomMobs, err := mobs.ListInRoom(ctx, c.Session.CurrentRoomID)
		cancel()
		if err != nil {
			slog.Debug("social: list mobs failed", "room", c.Session.CurrentRoomID, "error", err)
		} else if mob, ok := MatchMob(targetArg, roomMobs); ok {
			target := defangSocialName(mob.Core.Name)
			selfMsg := s.RenderTargetSelf(actor, target)
			otherMsg := s.RenderTargetOther(actor, target)
			broadcastSocial(sessions, c.Session, otherMsg)
			return c.Session.WriteString(selfMsg)
		}
	}
	// Self-target falls back to the untargeted broadcast. MatchPlayer
	// filters the actor out, so check here before delegating.
	if c.Session.CharacterName != "" && nameMatches(c.Session.CharacterName, strings.ToLower(strings.TrimSpace(targetArg))) {
		broadcastSocial(sessions, c.Session, s.RenderOther(actor))
		return c.Session.WriteString(s.RenderSelf(actor))
	}
	// MatchPlayer is room-scoped and wizinvis-filtered, mirroring the
	// `attack` resolver.
	peer, _ := MatchPlayer(targetArg, sessions, c.Session)
	if peer == nil {
		return c.Session.WriteString("{{No one by that name is here.}}::yellow\r\n")
	}
	target := defangSocialName(peer.CharacterName)
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

// socialTargetCandidates returns one Candidate per valid target for a
// targeted social: every mob in the caller's room plus every visible
// online peer. Order is mobs first, then players — same precedence
// runTargetedSocial uses when resolving.
func socialTargetCandidates(self *telnet.Session, sessions *session.Registry, mobs repo.MobInstanceRepo, partial string) []telnet.Candidate {
	if self == nil || self.CurrentRoomID == 0 {
		return nil
	}
	out := make([]telnet.Candidate, 0, 8)
	if mobs != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		list, err := mobs.ListInRoom(ctx, self.CurrentRoomID)
		cancel()
		if err == nil {
			out = append(out, mobKeywordCandidates(list, partial)...)
		}
	}
	if sessions != nil {
		lower := strings.ToLower(partial)
		for _, peer := range sessions.Snapshot() {
			if peer == self {
				continue
			}
			_, name, room := peer.InWorld()
			if room != self.CurrentRoomID || name == "" {
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
