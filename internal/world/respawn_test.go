package world

import (
	"context"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// respawnFixture builds a single zone + a single anchored mob template
// in a known room. Returned repos are populated, the helper hands back
// the zone/template/room ids the caller asserts against.
type respawnFixture struct {
	zones     *repo.MemoryZoneRepo
	templates *repo.MemoryMobTemplateRepo
	mobs      *repo.MemoryMobInstanceRepo
	zoneID    int64
	tplID     int64
	roomID    int64
}

func newRespawnFixture(t *testing.T, mode repo.ZoneResetMode, intervalS int) respawnFixture {
	t.Helper()
	ctx := context.Background()
	zones := repo.NewMemoryZoneRepo()
	templates := repo.NewMemoryMobTemplateRepo()
	mobs := repo.NewMemoryMobInstanceRepo()

	z, err := zones.Create(ctx, repo.Zone{
		ExternalID: "test-zone", Name: "Test", ResetMode: mode, ResetIntervalS: intervalS,
	})
	if err != nil {
		t.Fatalf("create zone: %v", err)
	}
	tpl, err := templates.Create(ctx, creature.MobTemplate{
		ExternalID: "rat", Core: creature.Core{Name: "rat", HPMax: 5},
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	const roomID int64 = 42
	if err := templates.SetSpawnAnchor(ctx, tpl.ID, z.ID, roomID); err != nil {
		t.Fatalf("set anchor: %v", err)
	}
	return respawnFixture{
		zones: zones, templates: templates, mobs: mobs,
		zoneID: z.ID, tplID: tpl.ID, roomID: roomID,
	}
}

func (f respawnFixture) live(t *testing.T) int {
	t.Helper()
	n, err := f.mobs.CountByTemplate(context.Background(), f.tplID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestRespawner_NeverModeSkipped(t *testing.T) {
	ctx := context.Background()
	f := newRespawnFixture(t, repo.ZoneResetNever, 60)
	r := &Respawner{Zones: f.zones, Templates: f.templates, Mobs: f.mobs,
		Now: func() time.Time { return time.Unix(10000, 0) }}

	r.Tick(ctx)

	if got := f.live(t); got != 0 {
		t.Fatalf("never-mode should not spawn, got %d", got)
	}
}

func TestRespawner_IntervalGate(t *testing.T) {
	ctx := context.Background()
	f := newRespawnFixture(t, repo.ZoneResetAlways, 60)
	now := time.Unix(1000, 0)
	r := &Respawner{Zones: f.zones, Templates: f.templates, Mobs: f.mobs,
		Now: func() time.Time { return now }}

	// First tick stamps last_reset_ts and spawns once.
	r.Tick(ctx)
	if got := f.live(t); got != 1 {
		t.Fatalf("first tick should spawn 1, got %d", got)
	}

	// 30s later: mob killed, but interval has not elapsed.
	if err := f.mobs.Delete(ctx, 1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	now = time.Unix(1030, 0)
	r.Tick(ctx)
	if got := f.live(t); got != 0 {
		t.Fatalf("interval not elapsed, should not respawn; got %d", got)
	}

	// 65s after first tick: respawn.
	now = time.Unix(1065, 0)
	r.Tick(ctx)
	if got := f.live(t); got != 1 {
		t.Fatalf("interval elapsed, should respawn; got %d", got)
	}
}

func TestRespawner_EmptyModeBlocksWhenOccupied(t *testing.T) {
	ctx := context.Background()
	f := newRespawnFixture(t, repo.ZoneResetEmpty, 60)
	now := time.Unix(1000, 0)
	occupied := true
	r := &Respawner{
		Zones: f.zones, Templates: f.templates, Mobs: f.mobs,
		Now: func() time.Time { return now },
		Occupancy: OccupancyCheckerFunc(func(_ context.Context, zid int64) bool {
			return occupied && zid == f.zoneID
		}),
	}

	r.Tick(ctx)
	if got := f.live(t); got != 0 {
		t.Fatalf("occupied empty-mode should not spawn, got %d", got)
	}
	// last_reset_ts must remain 0 so the next tick re-evaluates.
	ts, _ := f.zones.LastResetTs(ctx, f.zoneID)
	if ts != 0 {
		t.Fatalf("last_reset_ts stamped on deferred tick: %d", ts)
	}

	// Player leaves; advance a touch and tick again.
	occupied = false
	now = time.Unix(1001, 0)
	r.Tick(ctx)
	if got := f.live(t); got != 1 {
		t.Fatalf("empty zone, should spawn; got %d", got)
	}
}

func TestRespawner_DoesNotDoubleSpawn(t *testing.T) {
	ctx := context.Background()
	f := newRespawnFixture(t, repo.ZoneResetAlways, 1)
	now := time.Unix(2000, 0)
	r := &Respawner{Zones: f.zones, Templates: f.templates, Mobs: f.mobs,
		Now: func() time.Time { return now }}

	r.Tick(ctx)
	r.Tick(ctx)
	now = time.Unix(2010, 0)
	r.Tick(ctx)

	if got := f.live(t); got != 1 {
		t.Fatalf("should never exceed 1 live; got %d", got)
	}
}

func TestRespawner_SkipsTemplateWithNoHomeRoom(t *testing.T) {
	ctx := context.Background()
	zones := repo.NewMemoryZoneRepo()
	templates := repo.NewMemoryMobTemplateRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	z, _ := zones.Create(ctx, repo.Zone{
		ExternalID: "z", ResetMode: repo.ZoneResetAlways, ResetIntervalS: 1,
	})
	tpl, _ := templates.Create(ctx, creature.MobTemplate{
		ExternalID: "drift", Core: creature.Core{HPMax: 1},
	})
	// Anchor zone but leave home_room_id at 0 — admin-spawned shape.
	_ = templates.SetSpawnAnchor(ctx, tpl.ID, z.ID, 0)

	r := &Respawner{Zones: zones, Templates: templates, Mobs: mobs,
		Now: func() time.Time { return time.Unix(5000, 0) }}
	r.Tick(ctx)

	n, _ := mobs.CountByTemplate(ctx, tpl.ID)
	if n != 0 {
		t.Fatalf("template without home_room_id should not respawn; got %d", n)
	}
}
