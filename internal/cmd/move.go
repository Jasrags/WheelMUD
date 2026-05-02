package cmd

import (
	"errors"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/creature"
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
func NewMoveFamily(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, bus *eventbus.Bus) []*telnet.Command {
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
func moveDir(c *telnet.Context, dir string, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, bus *eventbus.Bus) error {
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
	// Hidden exits behave identically to a missing exit so the same
	// flavor message lands; a player who didn't already know about
	// the passage learns nothing from probing.
	if exit.Flags.Hidden {
		return s.WriteRaw([]byte("You can't go that way.\r\n"))
	}
	if exit.Flags.NoPass {
		return s.WriteRaw([]byte("An unseen force bars your way.\r\n"))
	}
	// Locked is checked before Closed so a player learns *why* they
	// can't pass, not just that the way is blocked. By convention a
	// Locked exit is also Closed, so this catches both.
	if exit.Flags.Locked {
		return s.WriteRaw([]byte("The door is locked.\r\n"))
	}
	if exit.Flags.Closed {
		return s.WriteRaw([]byte("The door is closed.\r\n"))
	}

	// Sector gating: air requires fly, underwater requires swim. Until
	// per-character Speed (creature.Core.Speed) is plumbed through the
	// Session, neither capability is wired up — so air/underwater
	// rooms are effectively blocked for everyone. The block lives in
	// the move path so the destination's terrain has the final say.
	// On a transient lookup failure we refuse the move outright rather
	// than fall through and let a player slip into a sector they
	// shouldn't reach; ErrRoomNotFound is treated as "no sector data,
	// allow" because the exit FK already proved the room exists in the
	// happy path.
	dest, err := rooms.FindByID(c.Ctx, exit.ToRoomID)
	switch {
	case err == nil:
		if msg, blocked := sectorGate(dest.Sector, s.Speed); blocked {
			return s.WriteRaw([]byte(msg + "\r\n"))
		}
	case errors.Is(err, repo.ErrRoomNotFound):
		// Stale exit pointing at a deleted room; fall through so the
		// player still gets a coherent error from the existing path.
	default:
		slog.Warn("move: dest room lookup failed", "char", s.CharacterID, "to", exit.ToRoomID, "error", err)
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

// sectorGate returns a refusal message + true when the destination
// terrain requires a movement mode the mover doesn't have. Air needs
// Speed.FlyFt > 0; underwater needs Speed.SwimFt > 0. A zero-value
// Speed (passed when the mover is unauthenticated or the lookup
// failed) means no specialized modes — the safe default for an
// anonymous mover is "blocked".
func sectorGate(sector repo.Sector, speed creature.Speed) (string, bool) {
	switch sector {
	case repo.SectorAir:
		if speed.FlyFt > 0 {
			return "", false
		}
		return "The air offers no purchase — you cannot fly.", true
	case repo.SectorUnderwater:
		if speed.SwimFt > 0 {
			return "", false
		}
		return "The water closes over your head — you cannot swim that deep.", true
	default:
		return "", false
	}
}

