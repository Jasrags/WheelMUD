package repo

import (
	"context"
	"errors"
)

// Trainer is one trainer-NPC's class config, keyed 1:1 to a MobTemplate.
// V1 carries the chargen class_id only — fees, min-level gating, and
// per-class skill requirements land with later #23 / #24 slices. Level
// commits move state through CharacterRepo (ClassLevels + Core stat
// fields); the trainer row is read-only after the world loader inserts
// it.
type Trainer struct {
	ID            int64
	MobTemplateID int64
	// ClassID is a chargen catalog id (e.g. "armsman"). The `train`
	// verb resolves it against the live *chargen.Catalog at runtime
	// because the catalog is content, not state — a content swap
	// must not require a DB migration.
	ClassID string
}

// TrainerRepo persists trainer config. Trainers are re-created from
// YAML by the world loader on startup; there is no runtime mutation
// path.
type TrainerRepo interface {
	// Create inserts a new trainer config. MobTemplateID must be
	// non-zero and unique. ClassID must be non-empty. Returns the
	// row with its assigned ID populated.
	Create(ctx context.Context, t Trainer) (Trainer, error)
	// GetByMobTemplateID returns the trainer attached to the given
	// template id, or ErrTrainerNotFound.
	GetByMobTemplateID(ctx context.Context, mobTemplateID int64) (Trainer, error)
	// ListTrainers returns every trainer, sorted by ID.
	ListTrainers(ctx context.Context) ([]Trainer, error)
}

// ErrTrainerNotFound is returned when a Trainer lookup misses.
// Callers translate this into "this isn't a trainer" at the verb layer.
var ErrTrainerNotFound = errors.New("repo: trainer not found")
