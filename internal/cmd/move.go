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
				return moveDir(c, dir, rooms, exits, items, mobs, characters, bus, clock, sessions)
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
// live in one place. Chained followers do NOT re-enter this function —
// they go through followAlong, which buffers all output for a single
// peer.WriteAsync emission so the follower's prompt cache + line-edit
// replay survive the chained move.
func moveDir(c *telnet.Context, dir string, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, bus *eventbus.Bus, clock *world.Clock, sessions *session.Registry) error {
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
	// whose Following() == s.CharacterID gets the same move attempt
	// via followAlong (which routes output through WriteAsync so the
	// follower's prompt cache + line-edit replay survive). Recursion
	// for sub-followers (C follows B follows A) is bounded by
	// maxFollowDepth inside chainFollowers.
	if sessions != nil {
		chainFollowers(c.Ctx, dir, fromRoomID, exit.ToRoomID, s, sessions,
			rooms, exits, items, mobs, characters, bus, clock, 0)
	}

	return RenderRoom(c.Ctx, s, rooms, exits, items, mobs, clock)
}

// chainFollowers drives the auto-follow per-move chain. Snapshots
// the registry, filters peers in fromRoomID following the leader,
// and runs each move through followAlong, which buffers all output
// (refusal text or destination render) into a single peer.WriteAsync
// payload so the follower's prompt cache + line-edit replay survive
// the chained move.
//
// After a follower successfully moves, recurse to chain any of
// THEIR followers — bounded by depth < maxFollowDepth so an
// unintended cycle can't wedge the dispatcher.
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

		moved := followAlong(ctx, peer, leader, dir, fromRoomID, toRoomID,
			rooms, exits, items, mobs, characters, bus, clock)
		if !moved {
			continue
		}
		// Sub-follower chain: peer (now in toRoomID) may itself have
		// followers who were also in fromRoomID. Recurse with peer as
		// the new leader.
		if depth+1 < maxFollowDepth {
			chainFollowers(ctx, dir, fromRoomID, toRoomID, peer, sessions,
				rooms, exits, items, mobs, characters, bus, clock, depth+1)
		}
	}
}

// followAlong attempts to move `peer` `dir` into `toRoomID`,
// accumulating refusal or render text into a single string and
// emitting it via peer.WriteAsync. Returns true when the move
// succeeded. On a refusal, peer.SetFollowing(0) is called so the
// leader doesn't keep dragging a stuck follower through obstacles.
func followAlong(ctx context.Context, peer, leader *telnet.Session, dir string, fromRoomID, toRoomID int64, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, bus *eventbus.Bus, clock *world.Clock) (moved bool) {
	leaderName := safeActor(leader)
	refuse := func(reason string) {
		peer.SetFollowing(0)
		msg := "{{You couldn't keep up with " + leaderName + ": " + reason + "}}::yellow\r\n"
		if err := peer.WriteAsync(msg); err != nil {
			slog.Debug("followAlong: notify failed", "follower", peer.CharacterID, "error", err)
		}
	}

	exit, err := exits.FindByDirection(ctx, fromRoomID, dir)
	if err != nil || exit.Flags.Hidden {
		refuse("you can't go that way")
		return false
	}
	if exit.Flags.NoPass {
		refuse("an unseen force bars the way")
		return false
	}
	if exit.Flags.Locked {
		refuse("the door is locked")
		return false
	}
	if exit.Flags.Closed {
		refuse("the door is closed")
		return false
	}

	// Sector gate: same per-mover capability check moveDir does.
	dest, err := rooms.FindByID(ctx, exit.ToRoomID)
	switch {
	case err == nil:
		if msg, blocked := sectorGate(dest.Sector, peer.Speed); blocked {
			refuse(msg)
			return false
		}
	case errors.Is(err, repo.ErrRoomNotFound):
		// Stale exit; let the move attempt proceed so the follower
		// still ends up co-located, matching moveDir's behaviour.
	default:
		slog.Warn("followAlong: dest room lookup failed",
			"follower", peer.CharacterID, "to", exit.ToRoomID, "error", err)
		refuse("something went wrong")
		return false
	}
	if exit.ToRoomID != toRoomID {
		// The exit in this direction goes somewhere other than where
		// the leader ended up (leader teleported, hidden one-way
		// connection, etc). Don't drag the follower into an unrelated
		// room — refuse and clear so the relationship doesn't keep
		// firing on every move.
		refuse("you lose them in the press")
		return false
	}

	if bus != nil && peer.CharacterID != 0 {
		bus.Publish(ctx, world.PlayerLeft{
			CharacterID: peer.CharacterID,
			FromRoomID:  fromRoomID,
			ToRoomID:    exit.ToRoomID,
		})
	}
	peer.SetCurrentRoom(exit.ToRoomID)
	if peer.CharacterID != 0 {
		if err := characters.RecordRoom(ctx, peer.CharacterID, exit.ToRoomID); err != nil {
			slog.Warn("followAlong: RecordRoom failed",
				"follower", peer.CharacterID, "to", exit.ToRoomID, "error", err)
		}
	}
	if bus != nil && peer.CharacterID != 0 {
		bus.Publish(ctx, world.PlayerEntered{
			CharacterID: peer.CharacterID,
			FromRoomID:  fromRoomID,
			ToRoomID:    exit.ToRoomID,
		})
	}

	rendered, renderErr := renderRoomString(ctx, peer, rooms, exits, items, mobs, clock)
	if renderErr != nil {
		slog.Warn("followAlong: render failed",
			"follower", peer.CharacterID, "to", exit.ToRoomID, "error", renderErr)
	}
	if err := peer.WriteAsync(rendered); err != nil {
		slog.Debug("followAlong: render write failed",
			"follower", peer.CharacterID, "error", err)
	}
	return true
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
