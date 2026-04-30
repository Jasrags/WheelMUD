package repo

import (
	"context"
	"errors"
	"time"
)

// Room is the canonical "place" in the world. Description text is rendered
// verbatim by the look command — callers are responsible for any cfmt
// styling baked into the strings.
type Room struct {
	ID        int64
	Name      string
	ShortDesc string
	LongDesc  string
	CreatedAt time.Time
}

// RoomRepo is the persistence boundary the look / move commands talk to.
// Read-only for now; world data ships via seed migration. Create / Update
// will land alongside the YAML loader.
type RoomRepo interface {
	// FindByID returns the room with the given id. Returns ErrRoomNotFound
	// when missing.
	FindByID(ctx context.Context, id int64) (Room, error)
}

// StarterRoomID is where new characters spawn and where the seed-data
// fallback room lives. Kept as a constant so character-create code and
// the seed migration agree without a runtime lookup.
const StarterRoomID int64 = 1

var ErrRoomNotFound = errors.New("repo: room not found")
