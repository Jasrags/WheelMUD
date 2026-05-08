package repo

import (
	"context"
	"errors"
)

// ZoneResetMode controls how the §9 areaReset bucket treats a zone
// when its reset_interval_s elapses. Persisted as a CHECK-bound TEXT
// column so an unknown mode fails at insert time rather than turning
// into "do nothing" at runtime.
type ZoneResetMode string

const (
	// ZoneResetAlways resets every interval regardless of occupancy.
	ZoneResetAlways ZoneResetMode = "always"
	// ZoneResetEmpty resets only when no players are present in the zone.
	// Default for most authored content; avoids yanking respawning mobs
	// out from under an in-progress fight.
	ZoneResetEmpty ZoneResetMode = "empty"
	// ZoneResetNever disables resets entirely. Useful for static zones
	// like player housing or museum districts.
	ZoneResetNever ZoneResetMode = "never"
)

// IsValid reports whether m is one of the known reset modes.
func (m ZoneResetMode) IsValid() bool {
	switch m {
	case ZoneResetAlways, ZoneResetEmpty, ZoneResetNever:
		return true
	}
	return false
}

// Zone is a persisted zone row. It carries every field the runtime
// can attach behavior to: builder attribution (§16 permissions),
// level range (advisory gating), reset cadence + mode (§9 areaReset
// bucket), climate (§10 ambient/weather), and a list of ambient
// strings the §10 ambient ticker rotates through. Reset rules
// themselves live in a sibling zone_resets table that lands later.
type Zone struct {
	ID             int64
	ExternalID     string
	Name           string
	Builder        string
	MinLevel       int
	MaxLevel       int
	ResetIntervalS int
	ResetMode      ZoneResetMode
	Climate        string
	Ambient        []string
}

// ZoneRepo is the persistence boundary for zone metadata. Read-only
// outside of the world loader in this slice; admin edit lands with
// §16 builder tools.
type ZoneRepo interface {
	// Create inserts a new zone. Returns ErrDuplicateZone when
	// external_id is already taken.
	Create(ctx context.Context, z Zone) (Zone, error)
	// GetByID resolves a zone by its rowid. Returns ErrZoneNotFound
	// when no row matches.
	GetByID(ctx context.Context, id int64) (Zone, error)
	// GetByExternalID resolves a zone by its YAML id. Returns
	// ErrZoneNotFound when no row matches.
	GetByExternalID(ctx context.Context, externalID string) (Zone, error)
	// List returns every zone, sorted by external_id for deterministic
	// admin output. An empty result is not an error.
	List(ctx context.Context) ([]Zone, error)
	// LastResetTs returns the unix-second timestamp of the most
	// recent successful AreaReset pass for zoneID, or 0 if the zone
	// has never been reset. Used by the §9 Respawner to gate each
	// zone's pass on reset_interval_s.
	LastResetTs(ctx context.Context, zoneID int64) (int64, error)
	// RecordLastResetTs writes ts (unix seconds) to zones.last_reset_ts.
	// Absolute write — mirrors RecordCoin / RecordXP.
	RecordLastResetTs(ctx context.Context, zoneID int64, ts int64) error
}

var (
	ErrZoneNotFound  = errors.New("repo: zone not found")
	ErrDuplicateZone = errors.New("repo: zone already exists for that external_id")
	// ErrInvalidResetMode signals that Create was handed a ResetMode
	// outside the known enum. The SQLite CHECK constraint catches this
	// on the persistence side; the memory repo enforces it explicitly
	// so behavior matches across implementations.
	ErrInvalidResetMode = errors.New("repo: invalid zone reset mode")
)
