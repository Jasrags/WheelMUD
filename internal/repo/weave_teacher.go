package repo

import (
	"context"
	"errors"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// WeaveTeacher is one teacher-NPC's mid-game weave-learning config,
// keyed 1:1 to a MobTemplate. V1 carries the level cap and an
// affinity-filter bitmask only — no fees, no min-level, no time
// cost. The cmd-layer `learn weave` verb resolves this against the
// chargen catalog and the channeler's affinities at runtime.
//
// AffinityFilter == 0 means "teach any weave the channeler can
// learn" (no Power restriction beyond the channeler's own
// Affinities). Non-zero restricts the teacher to weaves whose Power
// is set in the bitmask.
type WeaveTeacher struct {
	ID              int64
	MobTemplateID   int64
	MaxLevelTaught  int8
	AffinityFilter  creature.PowerSet
}

// WeaveTeacherRepo persists weave-teacher config. Teachers are
// re-created from YAML by the world loader on startup; there is no
// runtime mutation path.
type WeaveTeacherRepo interface {
	// Create inserts a new teacher config. MobTemplateID must be
	// non-zero and unique. MaxLevelTaught must be in [0, 9].
	// Returns the row with its assigned ID populated.
	Create(ctx context.Context, t WeaveTeacher) (WeaveTeacher, error)
	// GetByMobTemplateID returns the teacher attached to the given
	// template id, or ErrWeaveTeacherNotFound.
	GetByMobTemplateID(ctx context.Context, mobTemplateID int64) (WeaveTeacher, error)
	// ListWeaveTeachers returns every teacher, sorted by ID.
	ListWeaveTeachers(ctx context.Context) ([]WeaveTeacher, error)
}

// ErrWeaveTeacherNotFound is returned when a WeaveTeacher lookup
// misses. Callers translate this into "this isn't a teacher" at the
// verb layer.
var ErrWeaveTeacherNotFound = errors.New("repo: weave teacher not found")
