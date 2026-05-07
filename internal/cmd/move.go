package cmd

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// maxFollowDepth caps how deep the chain-on-move recursion will
// follow a follower's followers. Mirrors the segment-cap pattern
// in telnet.Registry.Dispatch — the typical depth is 1–2; the cap
// is a defence against accidental loops.
const maxFollowDepth = 16

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
func NewMoveFamily(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, bus *eventbus.Bus, clock *world.Clock, sessions *session.Registry) []*telnet.Command {
	build := func(name, dir string) *telnet.Command {
		return &telnet.Command{
			Name:    name,
			Aliases: []string{dir}, // single-letter alias matches the DB code
			Help:    "Move " + name,
			Auth:    telnet.AuthPlayer,
			Run: func(c *telnet.Context) error {
				return moveDir(c, dir, rooms, exits, items, mobs, characters, bus, clock, sessions, 0)
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
//
// followDepth tracks how many `chainFollowers` recursions led to this
// call. A direct player move is depth 0; each chained follower
// increments. Bounded by maxFollowDepth.
func moveDir(c *telnet.Context, dir string, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, bus *eventbus.Bus, clock *world.Clock, sessions *session.Registry, followDepth int) error {
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

	s.SetCurrentRoom(exit.ToRoomID)
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

	// Phase D #22 slice 3: chain followers. Any peer in fromRoomID
	// whose Following() == s.CharacterID gets the same move attempt.
	// Bounded recursion via followDepth (cap = maxFollowDepth) so an
	// undetected cycle can never wedge the dispatcher.
	if sessions != nil && followDepth < maxFollowDepth {
		chainFollowers(c.Ctx, dir, fromRoomID, exit.ToRoomID, s, sessions,
			rooms, exits, items, mobs, characters, bus, clock, followDepth)
	}

	return RenderRoom(c.Ctx, s, rooms, exits, items, mobs, clock)
}

// chainFollowers drives the auto-follow per-move chain. Snapshots
// the registry, filters peers in fromRoomID following the leader,
// and re-runs moveDir for each through the same code path so the
// follower's own followers continue the chain. A failure to keep
// up (locked door, sector gate, exit missing) emits a "couldn't
// keep up" line and clears the relationship so the leader doesn't
// drag a broken state forward.
func chainFollowers(ctx context.Context, dir string, fromRoomID, toRoomID int64, leader *telnet.Session, sessions *session.Registry, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, bus *eventbus.Bus, clock *world.Clock, depth int) {
	if leader == nil || leader.CharacterID == 0 {
		return
	}
	for _, peer := range sessions.Snapshot() {
		if peer == nil || peer == leader {
			continue
		}
		if peer.Following() != leader.CharacterID {
			continue
		}
		if peer.CurrentRoomID != fromRoomID {
			// Follower wasn't co-located when the leader left;
			// they don't auto-chase from another room. The
			// relationship stays — they may catch up later.
			continue
		}
		ctxFollower := ctx
		fc := &telnet.Context{
			Ctx:     ctxFollower,
			Session: peer,
			Name:    dir,
			Args:    nil,
			Raw:     "",
		}
		err := moveDir(fc, dir, rooms, exits, items, mobs, characters, bus, clock, sessions, depth+1)
		if err != nil {
			slog.Debug("chainFollowers: follower move failed",
				"follower", peer.CharacterID, "leader", leader.CharacterID,
				"dir", dir, "error", err)
		}
		if peer.CurrentRoomID != toRoomID {
			// Follower couldn't keep up (door locked, sector gate,
			// exit missing). Drop the relationship and tell them
			// what happened so the leader doesn't keep dragging
			// them through obstacles.
			peer.SetFollowing(0)
			leaderName := safeActor(leader)
			if err := peer.WriteAsync(
				"{{You couldn't keep up with " + leaderName + ". You stop following.}}::yellow\r\n"); err != nil {
				slog.Debug("chainFollowers: notify failed", "follower", peer.CharacterID, "error", err)
			}
		}
	}
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
