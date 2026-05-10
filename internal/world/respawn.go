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
// ZoneResetter stay testable without dragging the telnet package in.
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

// ZoneResetter walks every zone on each AreaReset tick and runs three
// reset steps in order — mob respawn, door state restoration, item
// respawn. One per-zone gate (LastResetTs + ResetIntervalS +
// ResetMode + occupancy) covers all three steps so reset semantics
// stay coherent: either the zone reset fires this tick (and all
// three steps run + the timestamp stamps), or it doesn't (and none
// of them do).
//
// Wired alongside Restocker into tick.Buckets.AreaReset so refills
// share the 5-minute default cadence.
type ZoneResetter struct {
	Zones     repo.ZoneRepo
	Templates repo.MobTemplateRepo
	Mobs      repo.MobInstanceRepo
	Rooms     repo.RoomRepo
	Exits     repo.ExitRepo
	Items     repo.ItemRepo
	// ItemSpecsByZone is keyed by zone external_id (the loader's
	// authoring identifier). nil is treated as "no items to respawn"
	// — fine for tests that only exercise the mob/door paths.
	ItemSpecsByZone map[string][]ItemSpec
	// Occupancy is consulted when a zone's ResetMode is "empty". Nil
	// is treated as "always unoccupied" — fine for tests that don't
	// care about the gate.
	Occupancy OccupancyChecker
	// Now overrides time.Now for deterministic tests. Production
	// leaves it nil and falls back to time.Now.
	Now func() time.Time
}

// NewZoneResetter builds a ZoneResetter with the given dependencies.
// rooms / exits / items / itemSpecs may be nil for narrow test
// fixtures; production wiring passes the real repos and the loader's
// item-spec map.
func NewZoneResetter(
	zones repo.ZoneRepo,
	templates repo.MobTemplateRepo,
	mobs repo.MobInstanceRepo,
	rooms repo.RoomRepo,
	exits repo.ExitRepo,
	items repo.ItemRepo,
	itemSpecs map[string][]ItemSpec,
	occ OccupancyChecker,
) *ZoneResetter {
	return &ZoneResetter{
		Zones:           zones,
		Templates:       templates,
		Mobs:            mobs,
		Rooms:           rooms,
		Exits:           exits,
		Items:           items,
		ItemSpecsByZone: itemSpecs,
		Occupancy:       occ,
	}
}

// Tick is the bucket subscription. Errors are logged and swallowed —
// one zone's broken state must not stop the rest from resetting.
func (r *ZoneResetter) Tick(ctx context.Context) {
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
		slog.Warn("zoneresetter: list zones failed", "error", err)
		return
	}
	for _, z := range zones {
		if z.ResetMode == repo.ZoneResetNever || z.ResetIntervalS <= 0 {
			continue
		}
		last, err := r.Zones.LastResetTs(ctx, z.ID)
		if err != nil {
			slog.Warn("zoneresetter: last reset ts", "zone", z.ID, "error", err)
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

		mobsSpawned := r.respawnMobs(ctx, z)
		doorsRestored := r.restoreDoors(ctx, z)
		itemsCreated := r.respawnItems(ctx, z)

		if err := r.Zones.RecordLastResetTs(ctx, z.ID, nowSec); err != nil {
			slog.Warn("zoneresetter: record reset ts", "zone", z.ID, "error", err)
		}
		// Telemetry only — emit even when all three counts are zero
		// so a quiet zone reset still leaves a breadcrumb.
		slog.Info("zoneresetter: zone reset",
			"zone", z.ID, "external_id", z.ExternalID,
			"mobs_spawned", mobsSpawned,
			"doors_restored", doorsRestored,
			"items_created", itemsCreated)
	}
}

// respawnMobs enumerates anchored templates and spawns one new
// instance per template that currently has zero live rows. Per-
// template errors are logged and skipped; one bad row never blocks
// the rest of the zone. Returns the number of mob instances created.
func (r *ZoneResetter) respawnMobs(ctx context.Context, z repo.Zone) int {
	templates, err := r.Templates.ListByRespawnZone(ctx, z.ID)
	if err != nil {
		slog.Warn("zoneresetter: list templates", "zone", z.ID, "error", err)
		return 0
	}
	spawned := 0
	for _, tpl := range templates {
		if tpl.HomeRoomID == 0 {
			continue
		}
		count, err := r.Mobs.CountByTemplate(ctx, tpl.ID)
		if err != nil {
			slog.Warn("zoneresetter: count by template",
				"zone", z.ID, "template", tpl.ID, "error", err)
			continue
		}
		if count > 0 {
			continue
		}
		spawn := creature.NewInstanceFromTemplate(tpl, tpl.HomeRoomID, z.ID)
		if _, err := r.Mobs.Create(ctx, spawn); err != nil {
			if errors.Is(err, context.Canceled) {
				return spawned
			}
			slog.Warn("zoneresetter: create instance",
				"zone", z.ID, "template", tpl.ID, "error", err)
			continue
		}
		spawned++
	}
	return spawned
}

// restoreDoors snaps every exit whose from-room belongs to z back to
// its authored Closed/Locked state. Returns the number of rows that
// actually changed; an in-sync zone returns 0 with no SQL writes
// beyond the SELECT-driven UPDATE the repo issues.
func (r *ZoneResetter) restoreDoors(ctx context.Context, z repo.Zone) int {
	if r.Rooms == nil || r.Exits == nil {
		return 0
	}
	roomIDs, err := r.Rooms.ListIDsByZone(ctx, z.ID)
	if err != nil {
		slog.Warn("zoneresetter: list room ids", "zone", z.ID, "error", err)
		return 0
	}
	if len(roomIDs) == 0 {
		return 0
	}
	n, err := r.Exits.RestoreAuthored(ctx, roomIDs)
	if err != nil {
		slog.Warn("zoneresetter: restore authored exits", "zone", z.ID, "error", err)
		return 0
	}
	return n
}

// respawnItems walks the loader-supplied spec list for z and re-
// creates any item whose external_id no longer exists anywhere
// (room floor, character inventory, container) in the database.
// Items currently held by a player suppress respawn — the
// FindByExternalID check is global, not zone-scoped, so a player
// who carried the item to a different zone still keeps it from
// duplicating. Per-spec errors are logged and skipped.
func (r *ZoneResetter) respawnItems(ctx context.Context, z repo.Zone) int {
	if r.Items == nil || r.Rooms == nil || len(r.ItemSpecsByZone) == 0 {
		return 0
	}
	specs := r.ItemSpecsByZone[z.ExternalID]
	if len(specs) == 0 {
		return 0
	}
	roomIDCache := make(map[string]int64)
	created := 0
	for _, spec := range specs {
		_, err := r.Items.FindByExternalID(ctx, spec.Item.ExternalID)
		if err == nil {
			// Still exists somewhere; respect player state.
			continue
		}
		if !errors.Is(err, repo.ErrItemNotFound) {
			slog.Warn("zoneresetter: find item by external_id",
				"zone", z.ID, "external_id", spec.Item.ExternalID, "error", err)
			continue
		}
		roomID, ok := roomIDCache[spec.RoomExternalID]
		if !ok {
			room, err := r.Rooms.FindByExternalID(ctx, spec.RoomExternalID)
			if err != nil {
				slog.Warn("zoneresetter: resolve home room",
					"zone", z.ID, "room", spec.RoomExternalID,
					"external_id", spec.Item.ExternalID, "error", err)
				continue
			}
			roomID = room.ID
			roomIDCache[spec.RoomExternalID] = roomID
		}
		toCreate := spec.Item
		toCreate.RoomID = roomID
		if _, err := r.Items.Create(ctx, toCreate); err != nil {
			// A duplicate-external-id race here is benign — the item
			// reappeared between our presence check and Create, so
			// the world already has it. Demote to debug; the rest of
			// the reset proceeds.
			if errors.Is(err, repo.ErrDuplicateExternalID) {
				slog.Debug("zoneresetter: item reappeared between check and create",
					"zone", z.ID, "external_id", spec.Item.ExternalID)
				continue
			}
			slog.Warn("zoneresetter: create item",
				"zone", z.ID, "external_id", spec.Item.ExternalID,
				"error", err)
			continue
		}
		created++
	}
	return created
}
