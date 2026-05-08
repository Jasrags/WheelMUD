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
	// ListExternalIDs returns every template's external_id, sorted.
	// Used for admin tab completion of `spawn mob`. Templates are
	// write-once and small (one row per archetype), so a full scan
	// is acceptable; pagination is a follow-up if the table grows.
	ListExternalIDs(ctx context.Context) ([]string, error)
	// ListByRespawnZone returns every template whose
	// RespawnZoneResetID matches zoneID. The §9 Respawner uses this
	// to enumerate spawn anchors for a given zone on each AreaReset
	// tick. Returns an empty slice (not an error) when no templates
	// are bound to the zone.
	ListByRespawnZone(ctx context.Context, zoneID int64) ([]creature.MobTemplate, error)
	// SetSpawnAnchor stamps the (zone, room) anchor on an existing
	// template row. The world loader calls this immediately after
	// Create so YAML-seeded mobs become respawnable; manual spawns
	// via the `spawn` admin verb leave both fields at 0.
	SetSpawnAnchor(ctx context.Context, templateID, zoneID, homeRoomID int64) error
}
