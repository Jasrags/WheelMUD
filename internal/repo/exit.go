package repo

import (
	"context"
	"errors"
)

// Direction constants for room exits. The DB stores the same short
// codes (CHECK constraint in 0003_create_world.sql, widened by
// 0007_widen_exit_directions.sql); commands accept long names
// ("north") and translate before hitting the repo.
const (
	DirNorth     = "n"
	DirSouth     = "s"
	DirEast      = "e"
	DirWest      = "w"
	DirUp        = "u"
	DirDown      = "d"
	DirNortheast = "ne"
	DirNorthwest = "nw"
	DirSoutheast = "se"
	DirSouthwest = "sw"
)

// DirLong renders a direction short code as the player-facing word
// ("n" → "north", "ne" → "northeast", "u" → "up"). Returns the input
// unchanged when no canonical mapping exists, which keeps logging
// safe for direction strings of unknown provenance.
func DirLong(code string) string {
	switch code {
	case DirNorth:
		return "north"
	case DirSouth:
		return "south"
	case DirEast:
		return "east"
	case DirWest:
		return "west"
	case DirUp:
		return "up"
	case DirDown:
		return "down"
	case DirNortheast:
		return "northeast"
	case DirNorthwest:
		return "northwest"
	case DirSoutheast:
		return "southeast"
	case DirSouthwest:
		return "southwest"
	}
	return code
}

// ExitFlags groups the door-state and gating tags on a single exit.
// Closed and locked are runtime-mutable (open/close/lock/unlock — §16);
// hidden / nopass / pickable are immutable authoring choices.
//
// AuthoredClosed / AuthoredLocked snapshot the YAML-load values of
// Closed / Locked (migration 0048). They never mutate after the
// loader writes them. ZoneResetter reads them on each AreaReset
// pass and restores the runtime columns to match.
type ExitFlags struct {
	Closed         bool
	Locked         bool
	Pickable       bool // false = lock can never be picked, only unlocked with the key
	Hidden         bool // never listed in look/exits; move treats as ErrExitNotFound
	NoPass         bool // even when "open", the way is barred (force field, etc.)
	AuthoredClosed bool // YAML-authored Closed value; never changes after insert
	AuthoredLocked bool // YAML-authored Locked value; never changes after insert
}

// Exit is a one-way connection between two rooms. Bidirectional travel
// is modeled as two exits (matching how MUD areas are typically
// authored), so a one-way drop is just a missing reverse exit.
type Exit struct {
	ID         int64
	FromRoomID int64
	ToRoomID   int64
	Direction  string // one of DirNorth..DirSouthwest
	Flags      ExitFlags
	// KeyExternalID names an item.external_id that unlocks this exit;
	// empty means no key — a locked exit must be picked or opened by
	// admin elevation.
	KeyExternalID  string
	LockDifficulty int
	Description    string
}

// ExitRepo is the persistence boundary the look / move commands and the
// YAML world loader use to resolve room connectivity.
type ExitRepo interface {
	// ListFrom returns every exit leaving fromRoomID, sorted by
	// direction (ascending lexicographic order on the short code:
	// d, e, n, ne, nw, s, se, sw, u, w). The auto-coord BFS runner
	// (internal/world/coords_derive) relies on this ordering for
	// deterministic first-arrival on contested rooms; a future
	// implementation that returns exits in insertion order would
	// silently change which path "wins" a coord conflict.
	// An empty result is not an error.
	ListFrom(ctx context.Context, fromRoomID int64) ([]Exit, error)
	// FindByDirection resolves the exit leaving fromRoomID in the given
	// direction. Returns ErrExitNotFound when the room has no such exit.
	FindByDirection(ctx context.Context, fromRoomID int64, direction string) (Exit, error)
	// Create inserts a new exit. Returns ErrDuplicateExit when
	// (from_room_id, direction) is already taken.
	Create(ctx context.Context, e Exit) (Exit, error)
	// UpdateFlags persists the runtime-mutable subset of an exit's
	// state (Closed / Locked) keyed by ID. Hidden / NoPass / Pickable /
	// AuthoredClosed / AuthoredLocked are authoring choices and are
	// not touched here. Returns ErrExitNotFound when no row matches.
	UpdateFlags(ctx context.Context, exitID int64, closed, locked bool) error
	// RestoreAuthored resets the runtime Closed / Locked columns of
	// every exit whose from_room_id is in fromRoomIDs back to their
	// authored_closed / authored_locked values. Used by ZoneResetter
	// to re-establish authored door state on each AreaReset pass.
	// Returns the number of rows changed for telemetry. Empty
	// fromRoomIDs is a no-op (returns 0, nil).
	RestoreAuthored(ctx context.Context, fromRoomIDs []int64) (int, error)
}

var (
	ErrExitNotFound  = errors.New("repo: exit not found")
	ErrDuplicateExit = errors.New("repo: exit already exists for that (room, direction)")
)
