package repo

import (
	"context"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// MobInstanceRepo persists live spawned mobs (mob_instances table).
// The spawn path Creates an instance from a MobTemplate; combat /
// movement / despawn mutate it through UpdateLive / UpdateRoom /
// Delete. ListInRoom feeds the look renderer once it migrates off
// the legacy flat mobs table.
//
// Stat fields beyond hp/subdual/conditions/position read from the
// parent template at spawn time; this repo only persists the live
// mutable slice. Future migrations may add an overrides table for
// instance-specific name swaps (e.g. "Halrad the Trolloc").
type MobInstanceRepo interface {
	// Create inserts a new live spawn. TemplateID and Core.HPCurrent
	// must be set. Returns the row with its assigned ID populated.
	Create(ctx context.Context, m creature.MobInstance) (creature.MobInstance, error)
	// GetByID returns the instance with the given primary key.
	// Returns ErrInstanceNotFound if no row matches.
	GetByID(ctx context.Context, id int64) (creature.MobInstance, error)
	// ListInRoom returns every live mob in the given room, sorted by
	// id (stable spawn order). An empty result is not an error.
	ListInRoom(ctx context.Context, roomID int64) ([]creature.MobInstance, error)
	// UpdateLive persists the four mutable runtime fields. Combat
	// hits, condition application, and stance changes call this.
	UpdateLive(ctx context.Context, id int64, hpCurrent, subdual int32, conditions creature.Condition, position creature.PositionFlags) error
	// UpdateRoom moves the instance to roomID (0 = removed from world
	// but not despawned). Returns ErrInstanceNotFound if id is unknown.
	UpdateRoom(ctx context.Context, id, roomID int64) error
	// Delete removes the instance row. Returns ErrInstanceNotFound
	// if id is unknown.
	Delete(ctx context.Context, id int64) error
}
