package cmd

import (
	"errors"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewMoveFamily builds the six direction commands (north, south, east,
// west, up, down) plus their single-letter aliases. Each command runs the
// shared moveDir helper, which resolves the exit, updates the session,
// persists the new location via CharacterRepo, and re-renders the room.
//
// All four world repos plus characters are required so a successful move
// can immediately call into RenderRoom.
func NewMoveFamily(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobRepo, characters repo.CharacterRepo) []*telnet.Command {
	build := func(name, dir string) *telnet.Command {
		return &telnet.Command{
			Name:    name,
			Aliases: []string{dir}, // single-letter alias matches the DB code
			Help:    "Move " + name,
			Auth:    telnet.AuthPlayer,
			Run: func(c *telnet.Context) error {
				return moveDir(c, dir, rooms, exits, items, mobs, characters)
			},
		}
	}
	return []*telnet.Command{
		build("north", repo.DirNorth),
		build("south", repo.DirSouth),
		build("east", repo.DirEast),
		build("west", repo.DirWest),
		build("up", repo.DirUp),
		build("down", repo.DirDown),
	}
}

// moveDir is the work behind every direction command. Pulled out as a
// helper so each direction's Run is a one-liner and the move semantics
// live in one place.
func moveDir(c *telnet.Context, dir string, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobRepo, characters repo.CharacterRepo) error {
	s := c.Session
	if s.CurrentRoomID == 0 {
		return s.WriteRaw([]byte("You are nowhere — cannot move.\r\n"))
	}

	exit, err := exits.FindByDirection(c.Ctx, s.CurrentRoomID, dir)
	if err != nil {
		if errors.Is(err, repo.ErrExitNotFound) {
			return s.WriteRaw([]byte("You can't go that way.\r\n"))
		}
		slog.Warn("move: exit lookup failed", "char", s.CharacterID, "from", s.CurrentRoomID, "dir", dir, "error", err)
		return s.WriteRaw([]byte("Could not move right now.\r\n"))
	}

	s.CurrentRoomID = exit.ToRoomID
	if s.CharacterID != 0 {
		// Best-effort persistence: if it fails we still let the player
		// move within the current process so the experience isn't broken
		// by a transient DB hiccup. The slog warning is the audit trail.
		if err := characters.RecordRoom(c.Ctx, s.CharacterID, exit.ToRoomID); err != nil {
			slog.Warn("RecordRoom failed", "char", s.CharacterID, "to", exit.ToRoomID, "error", err)
		}
	}
	return RenderRoom(c.Ctx, s, rooms, exits, items, mobs)
}
