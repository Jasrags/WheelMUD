package world

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// OccupancyChecker reports whether any player session is currently
// inside zoneID. Implemented by a small adapter over
// *session.Registry + RoomRepo in cmd/server; the interface lets the
// Respawner stay testable without dragging the telnet package in.
type OccupancyChecker interface {
	ZoneOccupied(ctx context.Context, zoneID int64) bool
}

// OccupancyCheckerFunc adapts a function to the OccupancyChecker
// interface so call-sites that already have a closure don't have to
// declare a wrapper type.
type OccupancyCheckerFunc func(ctx context.Context, zoneID int64) bool

func (f OccupancyCheckerFunc) ZoneOccupied(ctx context.Context, zoneID int64) bool {
	return f(ctx, zoneID)
}

// Respawner walks every zone on each AreaReset tick and tops up
// missing mob populations from their YAML-seeded spawn anchors.
//
// "Missing" today means CountByTemplate(template) == 0, since
// handleMobDeath deletes the instance row outright. When richer
// population semantics land (per-template max count, partial
// top-up), this is the place to extend.
//
// Wired alongside Restocker into tick.Buckets.AreaReset so refills
// share the 5-minute default cadence.
type Respawner struct {
	Zones     repo.ZoneRepo
	Templates repo.MobTemplateRepo
	Mobs      repo.MobInstanceRepo
	// Occupancy is consulted when a zone's ResetMode is "empty". Nil
	// is treated as "always unoccupied" — fine for tests that don't
	// care about the gate.
	Occupancy OccupancyChecker
	// Now overrides time.Now for deterministic tests. Production
	// leaves it nil and falls back to time.Now.
	Now func() time.Time
}

// NewRespawner builds a Respawner with the given dependencies. The
// occupancy checker may be nil for tests; production wiring passes a
// real session-aware adapter.
func NewRespawner(zones repo.ZoneRepo, templates repo.MobTemplateRepo, mobs repo.MobInstanceRepo, occ OccupancyChecker) *Respawner {
	return &Respawner{Zones: zones, Templates: templates, Mobs: mobs, Occupancy: occ}
}

// Tick is the bucket subscription. Errors are logged and swallowed —
// one zone's broken state must not stop the rest from respawning.
func (r *Respawner) Tick(ctx context.Context) {
	if r == nil || r.Zones == nil || r.Templates == nil || r.Mobs == nil {
		return
	}
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	nowSec := now().Unix()

	zones, err := r.Zones.List(ctx)
	if err != nil {
		slog.Warn("respawn: list zones failed", "error", err)
		return
	}
	for _, z := range zones {
		if z.ResetMode == repo.ZoneResetNever || z.ResetIntervalS <= 0 {
			continue
		}
		last, err := r.Zones.LastResetTs(ctx, z.ID)
		if err != nil {
			slog.Warn("respawn: last reset ts", "zone", z.ID, "error", err)
			continue
		}
		if nowSec-last < int64(z.ResetIntervalS) {
			continue
		}
		if z.ResetMode == repo.ZoneResetEmpty && r.Occupancy != nil &&
			r.Occupancy.ZoneOccupied(ctx, z.ID) {
			// Defer the reset until the zone empties — do not stamp
			// last_reset_ts so the next tick re-evaluates immediately.
			continue
		}

		r.respawnZone(ctx, z)

		if err := r.Zones.RecordLastResetTs(ctx, z.ID, nowSec); err != nil {
			slog.Warn("respawn: record reset ts", "zone", z.ID, "error", err)
		}
	}
}

// respawnZone enumerates anchored templates and spawns one new
// instance per template that currently has zero live rows.
// Per-template errors are logged and skipped; one bad row never
// blocks the rest of the zone.
func (r *Respawner) respawnZone(ctx context.Context, z repo.Zone) {
	templates, err := r.Templates.ListByRespawnZone(ctx, z.ID)
	if err != nil {
		slog.Warn("respawn: list templates", "zone", z.ID, "error", err)
		return
	}
	for _, tpl := range templates {
		if tpl.HomeRoomID == 0 {
			continue
		}
		count, err := r.Mobs.CountByTemplate(ctx, tpl.ID)
		if err != nil {
			slog.Warn("respawn: count by template",
				"zone", z.ID, "template", tpl.ID, "error", err)
			continue
		}
		if count > 0 {
			continue
		}
		spawn := creature.NewInstanceFromTemplate(tpl, tpl.HomeRoomID, z.ID)
		if _, err := r.Mobs.Create(ctx, spawn); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			slog.Warn("respawn: create instance",
				"zone", z.ID, "template", tpl.ID, "error", err)
			continue
		}
	}
}
