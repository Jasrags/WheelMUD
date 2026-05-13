package repo

import (
	"context"
	"errors"
	"time"
)

// BuilderZone is one (character, zone) builder-grant row. AuthAdmin
// bypasses this table; AuthPlayer with a matching row gains OLC edit
// rights scoped to the listed zone only. Phase G #33.
type BuilderZone struct {
	CharacterID int64
	ZoneID      int64
	// GrantedBy snapshots the actor character_id at write time so a
	// later revoke or audit walk has provenance even if the granter
	// has since lost AuthAdmin. Zero for system-issued rows (none
	// today; reserved for future world-loader seeds).
	GrantedBy int64
	GrantedAt time.Time
}

// BuilderZoneRepo is the persistence boundary for per-zone builder
// grants. The verb layer consults Has at OLC dispatch time;
// Session.IsBuilderFor caches a snapshot loaded at promoteToGame so
// the steady-state hit is a map lookup, not a repo call.
type BuilderZoneRepo interface {
	// Grant upserts a (characterID, zoneID) row. Idempotent: re-issuing
	// a grant refreshes grantedBy / grantedAt without erroring.
	Grant(ctx context.Context, characterID, zoneID, grantedBy int64, grantedAt time.Time) error

	// Revoke removes a (characterID, zoneID) row. Returns
	// ErrBuilderZoneNotFound when no row matches so the verb layer can
	// emit a meaningful refusal; callers wanting idempotency should
	// errors.Is-check.
	Revoke(ctx context.Context, characterID, zoneID int64) error

	// Has reports whether characterID is granted on zoneID. Fast path
	// for the per-verb permission gate; cache misses fall back to this.
	Has(ctx context.Context, characterID, zoneID int64) (bool, error)

	// ListForCharacter returns every grant held by characterID, sorted
	// by zone_id ascending for deterministic admin output. Called at
	// promoteToGame to populate Session.BuilderZones.
	ListForCharacter(ctx context.Context, characterID int64) ([]BuilderZone, error)

	// ListForZone returns every grant on zoneID, sorted by character_id
	// ascending. Powers the admin "who can edit this zone?" view.
	ListForZone(ctx context.Context, zoneID int64) ([]BuilderZone, error)
}

// ErrBuilderZoneNotFound is returned by Revoke when the requested
// grant doesn't exist. Has returns (false, nil) in the same case —
// only the destructive verb distinguishes "already gone" from "still
// there" because the admin viewer wants confirmation.
var ErrBuilderZoneNotFound = errors.New("repo: builder grant not found")
