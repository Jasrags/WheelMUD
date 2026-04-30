package repo

import (
	"context"
	"errors"
	"time"
)

// Room is the canonical "place" in the world. Description text is rendered
// verbatim by the look command — callers are responsible for any cfmt
// styling baked into the strings. ExternalID is the stable identifier
// referenced from YAML; the int ID is the surrogate primary key.
type Room struct {
	ID         int64
	ExternalID string
	Name       string
	ShortDesc  string
	LongDesc   string
	CreatedAt  time.Time
}

// RoomRepo is the persistence boundary the look / move commands and the
// YAML world loader talk to.
type RoomRepo interface {
	// FindByID returns the room with the given int id. Returns
	// ErrRoomNotFound when missing.
	FindByID(ctx context.Context, id int64) (Room, error)
	// FindByExternalID resolves a room by its stable string id (e.g.
	// "plaza.fountain"). Returns ErrRoomNotFound when missing.
	FindByExternalID(ctx context.Context, externalID string) (Room, error)
	// Create inserts a new room. ExternalID must be non-empty; an empty
	// value returns ErrInvalidExternalID. A duplicate external_id returns
	// ErrDuplicateExternalID. If r.ID is non-zero the row is inserted
	// with that exact id (the loader uses this to pin the starter room
	// to id=1); otherwise SQLite assigns one.
	Create(ctx context.Context, r Room) (Room, error)
}

// StarterRoomID is where new characters spawn. The YAML loader pins the
// room flagged `starter: true` to this id so the constant stays valid
// across loads.
const StarterRoomID int64 = 1

var ErrRoomNotFound = errors.New("repo: room not found")
