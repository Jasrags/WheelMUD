package repo

import (
	"context"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// MobTrailCap is sized for `track`'s expected horizon (a handful of
// rooms back) and keeps mob_trails bounded without a periodic prune
// task. Raise it if §12 staleness ends up wanting deeper history.
const MobTrailCap = 16

// MobTrail is one entry in a mob's movement history. Recorded by
// UpdateRoom whenever a mob moves to a non-zero room. The §10
// `track` verb reads these to report a target's most recent
// known location and first-step direction.
type MobTrail struct {
	MobID  int64
	RoomID int64
	At     time.Time
}

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
	// ListSpawned returns every mob with a non-zero room_id, ordered
	// by id ASC, capped at limit. limit <= 0 returns nothing. The
	// wander tick uses this to enumerate eligible movers each pulse
	// without walking every room.
	ListSpawned(ctx context.Context, limit int) ([]creature.MobInstance, error)
	// UpdateLive persists the four mutable runtime fields. Combat
	// hits, condition application, and stance changes call this.
	UpdateLive(ctx context.Context, id int64, hpCurrent, subdual int32, conditions creature.Condition, position creature.PositionFlags) error
	// UpdateRoom moves the instance to roomID (0 = removed from world
	// but not despawned). When roomID is non-zero, a MobTrail row is
	// recorded for the §10 `track` verb and rows past MobTrailCap are
	// pruned. Returns ErrInstanceNotFound if id is unknown.
	UpdateRoom(ctx context.Context, id, roomID int64) error
	// RecentTrails returns up to limit trail entries for mobID, newest
	// first. limit <= 0 returns nothing. The slice may be shorter than
	// limit (or empty) if the mob has fewer recorded movements.
	RecentTrails(ctx context.Context, mobID int64, limit int) ([]MobTrail, error)
	// Delete removes the instance row. Returns ErrInstanceNotFound
	// if id is unknown. Trail rows for the mob are removed alongside
	// (sqlite via FK CASCADE; memory mirrors this).
	Delete(ctx context.Context, id int64) error
}
