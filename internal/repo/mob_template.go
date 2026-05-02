package repo

import (
	"context"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// MobTemplateRepo persists immutable mob archetypes
// (mob_templates table). Builders / the world loader Create
// templates; the spawn path reads them via GetByID / GetByExternalID
// and copies the Core stat block into a new MobInstance.
//
// Templates are intentionally write-once at this stage — there is
// no Update method until OLC editor modes (§16) need one. Existing
// rows can be deleted and recreated for now via the loader's
// transactional re-sync.
type MobTemplateRepo interface {
	// Create inserts a new template. ExternalID must be non-empty.
	// Returns the row with its assigned ID populated.
	Create(ctx context.Context, t creature.MobTemplate) (creature.MobTemplate, error)
	// GetByID returns the template with the given primary key.
	// Returns ErrTemplateNotFound if no row matches.
	GetByID(ctx context.Context, id int64) (creature.MobTemplate, error)
	// GetByExternalID returns the template with the given YAML id.
	// Returns ErrTemplateNotFound if no row matches.
	GetByExternalID(ctx context.Context, externalID string) (creature.MobTemplate, error)
}
