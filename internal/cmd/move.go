package cmd

import (
	"errors"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewMoveFamily builds the ten direction commands — cardinals
// (north/south/east/west), vertical (up/down), and diagonals
// (northeast/northwest/southeast/southwest) — each with its short-code
// alias (n/s/e/w/u/d/ne/nw/se/sw) matching the DB direction column.
// Each command runs the shared moveDir helper, which resolves the
// exit, updates the session, persists the new location via
// CharacterRepo, publishes PlayerLeft / PlayerEntered on bus, and
// re-renders the room.
//
// bus may be nil during tests that don't care about event emission;
// moveDir tolerates a nil bus.
func NewMoveFamily(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobRepo, characters repo.CharacterRepo, bus *eventbus.Bus) []*telnet.Command {
	build := func(name, dir string) *telnet.Command {
		return &telnet.Command{
			Name:    name,
			Aliases: []string{dir}, // single-letter alias matches the DB code
			Help:    "Move " + name,
			Auth:    telnet.AuthPlayer,
			Run: func(c *telnet.Context) error {
				return moveDir(c, dir, rooms, exits, items, mobs, characters, bus)
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
		build("northeast", repo.DirNortheast),
		build("northwest", repo.DirNorthwest),
		build("southeast", repo.DirSoutheast),
		build("southwest", repo.DirSouthwest),
	}
}

// moveDir is the work behind every direction command. Pulled out as a
// helper so each direction's Run is a one-liner and the move semantics
// live in one place.
func moveDir(c *telnet.Context, dir string, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobRepo, characters repo.CharacterRepo, bus *eventbus.Bus) error {
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

	fromRoomID := s.CurrentRoomID
	if bus != nil && s.CharacterID != 0 {
		bus.Publish(c.Ctx, world.PlayerLeft{
			CharacterID: s.CharacterID,
			FromRoomID:  fromRoomID,
			ToRoomID:    exit.ToRoomID,
		})
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

	if bus != nil && s.CharacterID != 0 {
		bus.Publish(c.Ctx, world.PlayerEntered{
			CharacterID: s.CharacterID,
			FromRoomID:  fromRoomID,
			ToRoomID:    exit.ToRoomID,
		})
	}

	return RenderRoom(c.Ctx, s, rooms, exits, items, mobs)
}
