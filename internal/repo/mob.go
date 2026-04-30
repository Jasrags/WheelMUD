package repo

import (
	"context"
	"time"
)

// Mob is a placeholder NPC entity. Stats, AI, dialogue, and the
// template/instance split land in §9 / §11 / §15 of the roadmap; this
// slice carries only enough to render a room's "Also here:" line.
//
// RoomID == 0 means "not currently in any room" — useful later for a
// despawn / respawn pool without a schema change. ExternalID is the
// stable identifier referenced from YAML.
type Mob struct {
	ID         int64
	ExternalID string
	Name       string
	NameLower  string
	ShortDesc  string
	RoomID     int64
	CreatedAt  time.Time
}

// MobRepo is the persistence boundary for mobs.
type MobRepo interface {
	// ListInRoom returns every mob whose room_id equals the given id,
	// sorted by name. An empty result is not an error.
	ListInRoom(ctx context.Context, roomID int64) ([]Mob, error)
	// Create inserts a new mob. ExternalID must be non-empty.
	Create(ctx context.Context, m Mob) (Mob, error)
}
