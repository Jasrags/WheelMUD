package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewTeleport builds the teleport (tp) command. Two forms:
//
//	tp <room>             - teleport yourself
//	tp <user> <room>      - teleport another logged-in character
//
// <room> is parsed as a numeric DB id first, then falls back to a
// RoomRepo.FindByExternalID lookup, so `tp 1`, `tp plaza.fountain`,
// and `tp tr.emonds_field` all work. <user> matches against
// Session.CharacterName case-insensitively across the live session
// registry.
//
// Auth: gated at AuthPlayer for now so it's testable end-to-end on the
// current single-tier auth model. This is a builder/admin tool and
// should be promoted to AuthAdmin as soon as an admin role exists on
// accounts. See world_aggregates_followups.md.
func NewTeleport(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:    "teleport",
		Aliases: []string{"tp"},
		Help:    "Teleport yourself or another player to a room",
		Long: "Usage: tp <room>          - teleport yourself\n" +
			"       tp <user> <room>   - teleport another player\n\n" +
			"<room> is a numeric room id (e.g. 1) or a stable\n" +
			"external id (e.g. tr.emonds_field).",
		Auth:    telnet.AuthPlayer,
		MinArgs: 1,
		Run: func(c *telnet.Context) error {
			switch len(c.Args) {
			case 1:
				return tpSelf(c, c.Args[0], rooms, exits, items, mobs, characters)
			case 2:
				return tpOther(c, c.Args[0], c.Args[1], rooms, exits, items, mobs, characters, sessions)
			default:
				return c.Session.WriteRaw([]byte("Usage: tp <room>   or   tp <user> <room>\r\n"))
			}
		},
	}
}

// tpSelf teleports the calling session to the resolved room and
// re-renders it. Persistence and session.CurrentRoomID update mirror
// what the move family does on a successful step.
func tpSelf(c *telnet.Context, roomArg string, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo) error {
	room, err := resolveRoom(c.Ctx, rooms, roomArg)
	if errors.Is(err, repo.ErrRoomNotFound) {
		return c.Session.WriteString("{{No such room: " + sanitizeArg(roomArg) + "}}::red\r\n")
	}
	if err != nil {
		slog.Warn("tp: room lookup failed", "char", c.Session.CharacterID, "arg", roomArg, "error", err)
		return c.Session.WriteString("{{Could not teleport.}}::red\r\n")
	}
	relocate(c.Ctx, c.Session, room.ID, characters)
	return RenderRoom(c.Ctx, c.Session, rooms, exits, items, mobs)
}

// tpOther teleports another live session by character name. The caller
// gets a confirmation; the target gets a "world ripples" notice plus
// the new room rendered into their own connection.
func tpOther(c *telnet.Context, username, roomArg string, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, sessions *session.Registry) error {
	target := lookupByCharacter(sessions, username)
	if target == nil {
		return c.Session.WriteString("{{No such player online: " + sanitizeArg(username) + "}}::red\r\n")
	}
	room, err := resolveRoom(c.Ctx, rooms, roomArg)
	if errors.Is(err, repo.ErrRoomNotFound) {
		return c.Session.WriteString("{{No such room: " + sanitizeArg(roomArg) + "}}::red\r\n")
	}
	if err != nil {
		slog.Warn("tp: room lookup failed", "caller", c.Session.CharacterID, "target", target.CharacterID, "arg", roomArg, "error", err)
		return c.Session.WriteString("{{Could not teleport " + sanitizeArg(username) + ".}}::red\r\n")
	}
	relocate(c.Ctx, target, room.ID, characters)
	if err := c.Session.WriteString("{{Teleported " + target.CharacterName + " to " + room.Name + ".}}::green\r\n"); err != nil {
		return fmt.Errorf("write caller ack: %w", err)
	}
	if err := target.WriteString("{{The world ripples; you are somewhere else.}}::magenta\r\n"); err != nil {
		slog.Debug("tp: target notify failed", "target", target.CharacterID, "error", err)
	}
	// Render to the target with a detached context: the caller's ctx
	// dies when the caller's command dispatch returns / disconnects,
	// which would abort the unrelated target's render mid-flight.
	return RenderRoom(context.Background(), target, rooms, exits, items, mobs)
}

// resolveRoom turns a user-supplied room argument into an existing
// Room. Numeric → FindByID; non-numeric → FindByExternalID. Avoids a
// double DB hit by branching on the parse result.
func resolveRoom(ctx context.Context, rooms repo.RoomRepo, arg string) (repo.Room, error) {
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil {
		return rooms.FindByID(ctx, id)
	}
	return rooms.FindByExternalID(ctx, arg)
}

// relocate updates Session.CurrentRoomID and best-effort persists via
// CharacterRepo.RecordRoom. Mirrors moveDir; failure to persist is
// logged but doesn't abort the teleport in-process so a transient DB
// hiccup doesn't trap the caller.
func relocate(ctx context.Context, s *telnet.Session, roomID int64, characters repo.CharacterRepo) {
	s.CurrentRoomID = roomID
	if s.CharacterID == 0 {
		return
	}
	if err := characters.RecordRoom(ctx, s.CharacterID, roomID); err != nil {
		slog.Warn("tp: RecordRoom failed", "char", s.CharacterID, "to", roomID, "error", err)
	}
}

// lookupByCharacter walks the registry snapshot for a session whose
// CharacterName matches name (case-insensitive). Returns nil if not
// found. Linear scan; fine while server population is small. A name
// → session index can land on the registry when it matters.
func lookupByCharacter(sessions *session.Registry, name string) *telnet.Session {
	for _, s := range sessions.Snapshot() {
		if strings.EqualFold(s.CharacterName, name) {
			return s
		}
	}
	return nil
}

// sanitizeArg trims a user-supplied argument to a safe length and
// strips control characters before echoing it back into a cfmt
// template. cfmt's `{{...}}::style` syntax doesn't try to interpret
// content as markup, but a control byte or stray closing brace would
// still wreck the rendered output and the next prompt.
func sanitizeArg(s string) string {
	if len(s) > 64 {
		s = s[:64]
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || r == 0x7F {
			continue
		}
		// `}` on its own is fine, but the cfmt parser is happier
		// without raw template-looking sequences in echoed text.
		if r == '{' || r == '}' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
