package repo

import (
	"context"
	"time"
)

// Item is a placeholder world object. The schema deliberately captures
// only enough to render a room's "You see:" line; weight, value, wear
// flags, container semantics, and inventory ownership land alongside
// §14 of the roadmap.
//
// RoomID == 0 means "not currently in a room" — a future inventory or
// respawn-pool slice will populate that state without a schema change.
// ExternalID is the stable identifier referenced from YAML.
type Item struct {
	ID         int64
	ExternalID string
	Name       string
	NameLower  string
	ShortDesc  string
	RoomID     int64
	CreatedAt  time.Time
}

// ItemRepo is the persistence boundary for items.
type ItemRepo interface {
	// ListInRoom returns every item whose room_id equals the given id,
	// sorted by name. An empty result is not an error.
	ListInRoom(ctx context.Context, roomID int64) ([]Item, error)
	// Create inserts a new item. ExternalID must be non-empty.
	Create(ctx context.Context, i Item) (Item, error)
}
