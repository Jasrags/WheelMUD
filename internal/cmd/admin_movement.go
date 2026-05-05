package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewGoto builds the `goto` admin verb. Resolves a single argument as
// either an online character name (via the session registry) or a
// room (numeric id or external id), then teleports the caller into
// the resolved room. Player-name lookup wins on conflict so an admin
// hunting a player by name doesn't accidentally land in a similarly
// named room.
//
// Auth: AuthAdmin. The dispatcher rejects lower tiers silently
// (telnet/command.go), so the command does not enumerate.
func NewGoto(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, sessions *session.Registry, clock *world.Clock) *telnet.Command {
	return &telnet.Command{
		Name: "goto",
		Help: "goto <player|room> — teleport yourself to a player or room",
		Long: "Usage: goto <player>\n" +
			"       goto <room-id>\n\n" +
			"<player> matches an online character name (case-insensitive).\n" +
			"<room>   is a numeric room id (e.g. 1) or a stable external\n" +
			"         id (e.g. tr.emonds_field). Player-name lookup wins\n" +
			"         on conflict.",
		Auth:      telnet.AuthAdmin,
		MinArgs:   1,
		Completer: completeTeleport(rooms, sessions),
		Run: func(c *telnet.Context) error {
			arg := c.Args[0]
			// Player-first lookup. A hit short-circuits the room lookup
			// so an offline-name miss doesn't probe the DB twice.
			if peer := lookupByCharacter(sessions, arg); peer != nil {
				if peer == c.Session {
					return c.Session.WriteString("{{You are already there.}}::yellow\r\n")
				}
				if peer.CurrentRoomID == 0 {
					return c.Session.WriteString("{{" + sanitizeArg(peer.CharacterName) + " is not in the world yet.}}::yellow\r\n")
				}
				return gotoRoomID(c, peer.CurrentRoomID, rooms, exits, items, mobs, characters, clock)
			}
			return tpSelf(c, arg, rooms, exits, items, mobs, characters, clock)
		},
	}
}

// gotoRoomID is the room-id-known branch of `goto`. Mirrors tpSelf's
// no-teleport gate + relocate + render but skips the room arg parsing
// since the id is already in hand.
func gotoRoomID(c *telnet.Context, roomID int64, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, clock *world.Clock) error {
	room, err := rooms.FindByID(c.Ctx, roomID)
	if errors.Is(err, repo.ErrRoomNotFound) {
		return c.Session.WriteString("{{That room no longer exists.}}::red\r\n")
	}
	if err != nil {
		slog.Warn("goto: room lookup failed", "char", c.Session.CharacterID, "room", roomID, "error", err)
		return c.Session.WriteString("{{Could not teleport.}}::red\r\n")
	}
	if room.Flags.NoTeleport {
		return c.Session.WriteString("{{The Pattern resists — that destination cannot be reached by weave.}}::red\r\n")
	}
	relocate(c.Ctx, c.Session, room.ID, characters)
	return RenderRoom(c.Ctx, c.Session, rooms, exits, items, mobs, clock)
}

// NewTransfer builds the `transfer` admin verb. Pulls another online
// player to the caller's current room (1-arg form) or to an arbitrary
// room (2-arg form, equivalent to `tp <user> <room>` but kept as its
// own verb for ergonomics + audit clarity).
func NewTransfer(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, sessions *session.Registry, clock *world.Clock) *telnet.Command {
	return &telnet.Command{
		Name: "transfer",
		Help: "transfer <player> [<room>] — pull a player to you (or to a room)",
		Long: "Usage: transfer <player>          - pull <player> to your room\n" +
			"       transfer <player> <room>   - send <player> to <room>\n\n" +
			"<player> matches an online character name (case-insensitive).\n" +
			"<room>   is a numeric room id or stable external id.",
		Auth:    telnet.AuthAdmin,
		MinArgs: 1,
		Completer: func(s *telnet.Session, args string) []telnet.Candidate {
			slot, partial := completerSlot(args)
			if slot < 0 || slot > 1 {
				return nil
			}
			if slot == 0 {
				return onlineNameCandidates(s, sessions, partial)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return roomIDCandidates(ctx, rooms, partial)
		},
		Run: func(c *telnet.Context) error {
			switch len(c.Args) {
			case 1:
				return transferToCaller(c, c.Args[0], rooms, exits, items, mobs, characters, sessions, clock)
			case 2:
				return tpOther(c, c.Args[0], c.Args[1], rooms, exits, items, mobs, characters, sessions, clock)
			default:
				return c.Session.WriteRaw([]byte("Usage: transfer <player> [<room>]\r\n"))
			}
		},
	}
}

// NewSummon builds the `summon` admin verb — strictly the 1-arg
// "pull player to me" form. Kept distinct from `transfer` so audit
// logs (Phase A 5) record the intent unambiguously.
func NewSummon(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, sessions *session.Registry, clock *world.Clock) *telnet.Command {
	return &telnet.Command{
		Name:    "summon",
		Help:    "summon <player> — pull a player to your current room",
		Long:    "Usage: summon <player>\n\n<player> matches an online character name (case-insensitive).",
		Auth:    telnet.AuthAdmin,
		MinArgs: 1,
		Completer: func(s *telnet.Session, args string) []telnet.Candidate {
			slot, partial := completerSlot(args)
			if slot != 0 {
				return nil
			}
			return onlineNameCandidates(s, sessions, partial)
		},
		Run: func(c *telnet.Context) error {
			return transferToCaller(c, c.Args[0], rooms, exits, items, mobs, characters, sessions, clock)
		},
	}
}

// transferToCaller is the shared body of `summon` and the 1-arg
// `transfer` form. Resolves the named player, sanity-checks self-
// targeting and the caller's own location, validates NoTeleport on
// the destination, then mirrors tpOther's notify pattern (caller ack
// + target async ripple + detached-context render).
func transferToCaller(c *telnet.Context, username string, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, sessions *session.Registry, clock *world.Clock) error {
	target := lookupByCharacter(sessions, username)
	if target == nil {
		return c.Session.WriteString("{{No such player online: " + sanitizeArg(username) + "}}::red\r\n")
	}
	if target == c.Session {
		return c.Session.WriteString("{{You are already here.}}::yellow\r\n")
	}
	if c.Session.CurrentRoomID == 0 {
		return c.Session.WriteString("{{You are nowhere — there is nowhere to summon them to.}}::yellow\r\n")
	}
	room, err := rooms.FindByID(c.Ctx, c.Session.CurrentRoomID)
	if errors.Is(err, repo.ErrRoomNotFound) {
		return c.Session.WriteString("{{Your current room no longer exists.}}::red\r\n")
	}
	if err != nil {
		slog.Warn("transfer: room lookup failed", "caller", c.Session.CharacterID, "room", c.Session.CurrentRoomID, "error", err)
		return c.Session.WriteString("{{Could not transfer " + sanitizeArg(username) + ".}}::red\r\n")
	}
	if room.Flags.NoTeleport {
		return c.Session.WriteString("{{The Pattern resists — none may be drawn to this place.}}::red\r\n")
	}
	relocate(c.Ctx, target, room.ID, characters)
	// Defang both spliced fields: CharacterName is player-supplied at
	// character-create, and room.Name is builder-authored — neither is
	// safe to splice raw into a cfmt template.
	if err := c.Session.WriteString("{{Transferred " + defangWorldField(target.CharacterName) + " to " + defangWorldField(room.Name) + ".}}::green\r\n"); err != nil {
		return fmt.Errorf("write caller ack: %w", err)
	}
	if err := target.WriteAsync("{{The world ripples; you are somewhere else.}}::magenta"); err != nil {
		slog.Debug("transfer: target notify failed", "target", target.CharacterID, "error", err)
	}
	// Detached context for the same reason tpOther uses one: the
	// caller's ctx dies with the caller's dispatch, but the target's
	// render is independent.
	return RenderRoom(context.Background(), target, rooms, exits, items, mobs, clock)
}
