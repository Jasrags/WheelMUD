package repo

import (
	"context"
	"errors"
)

// Direction constants for room exits. The DB stores the same single-byte
// strings (CHECK constraint in 0003_create_world.sql); commands accept
// long names ("north") and translate before hitting the repo.
const (
	DirNorth = "n"
	DirSouth = "s"
	DirEast  = "e"
	DirWest  = "w"
	DirUp    = "u"
	DirDown  = "d"
)

// Exit is a one-way connection between two rooms. Bidirectional travel
// is modeled as two exits (matching how MUD areas are typically
// authored), so a one-way drop is just a missing reverse exit.
type Exit struct {
	ID         int64
	FromRoomID int64
	ToRoomID   int64
	Direction  string // one of DirNorth..DirDown
}

// ExitRepo is the persistence boundary the look / move commands use to
// resolve room connectivity.
type ExitRepo interface {
	// ListFrom returns every exit leaving fromRoomID, sorted by direction.
	// An empty result is not an error.
	ListFrom(ctx context.Context, fromRoomID int64) ([]Exit, error)
	// FindByDirection resolves the exit leaving fromRoomID in the given
	// direction. Returns ErrExitNotFound when the room has no such exit.
	FindByDirection(ctx context.Context, fromRoomID int64, direction string) (Exit, error)
}

var ErrExitNotFound = errors.New("repo: exit not found")
