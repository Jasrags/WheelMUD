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

// resetterFixture extends respawnFixture with rooms / exits / items
// repos so the door-restoration and item-respawn paths have real
// data to walk. The room is inserted into the in-memory RoomRepo
// with the same id the mob template anchored to.
type resetterFixture struct {
	respawnFixture
	rooms *repo.MemoryRoomRepo
	exits *repo.MemoryExitRepo
	items *repo.MemoryItemRepo
}

func newResetterFixture(t *testing.T, mode repo.ZoneResetMode, intervalS int) resetterFixture {
	t.Helper()
	base := newRespawnFixture(t, mode, intervalS)
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	rooms.Insert(repo.Room{
		ID: base.roomID, ExternalID: "test-room", Name: "Test Room", ZoneID: base.zoneID,
	})
	return resetterFixture{
		respawnFixture: base,
		rooms:          rooms,
		exits:          exits,
		items:          items,
	}
}

func (f resetterFixture) newResetter(now func() time.Time, occ OccupancyChecker, specs map[string][]ItemSpec) *ZoneResetter {
	return &ZoneResetter{
		Zones:           f.zones,
		Templates:       f.templates,
		Mobs:            f.mobs,
		Rooms:           f.rooms,
		Exits:           f.exits,
		Items:           f.items,
		ItemSpecsByZone: specs,
		Occupancy:       occ,
		Now:             now,
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

func TestZoneResetter_NeverModeSkipped(t *testing.T) {
	ctx := context.Background()
	f := newRespawnFixture(t, repo.ZoneResetNever, 60)
	r := &ZoneResetter{Zones: f.zones, Templates: f.templates, Mobs: f.mobs,
		Now: func() time.Time { return time.Unix(10000, 0) }}

	r.Tick(ctx)

	if got := f.live(t); got != 0 {
		t.Fatalf("never-mode should not spawn, got %d", got)
	}
}

func TestZoneResetter_IntervalGate(t *testing.T) {
	ctx := context.Background()
	f := newRespawnFixture(t, repo.ZoneResetAlways, 60)
	now := time.Unix(1000, 0)
	r := &ZoneResetter{Zones: f.zones, Templates: f.templates, Mobs: f.mobs,
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

func TestZoneResetter_EmptyModeBlocksWhenOccupied(t *testing.T) {
	ctx := context.Background()
	f := newRespawnFixture(t, repo.ZoneResetEmpty, 60)
	now := time.Unix(1000, 0)
	occupied := true
	r := &ZoneResetter{
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

func TestZoneResetter_DoesNotDoubleSpawn(t *testing.T) {
	ctx := context.Background()
	f := newRespawnFixture(t, repo.ZoneResetAlways, 1)
	now := time.Unix(2000, 0)
	r := &ZoneResetter{Zones: f.zones, Templates: f.templates, Mobs: f.mobs,
		Now: func() time.Time { return now }}

	r.Tick(ctx)
	r.Tick(ctx)
	now = time.Unix(2010, 0)
	r.Tick(ctx)

	if got := f.live(t); got != 1 {
		t.Fatalf("should never exceed 1 live; got %d", got)
	}
}

func TestZoneResetter_SkipsTemplateWithNoHomeRoom(t *testing.T) {
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

	r := &ZoneResetter{Zones: zones, Templates: templates, Mobs: mobs,
		Now: func() time.Time { return time.Unix(5000, 0) }}
	r.Tick(ctx)

	n, _ := mobs.CountByTemplate(ctx, tpl.ID)
	if n != 0 {
		t.Fatalf("template without home_room_id should not respawn; got %d", n)
	}
}

// authoredDoor seeds an authored-closed-and-locked exit on f.roomID
// pointing to a fresh adjacent room. Returns the exit id.
func authoredDoor(t *testing.T, f resetterFixture) int64 {
	t.Helper()
	// Adjacent room is in a different zone so it's never affected by
	// our reset assertions.
	other := f.rooms.Insert(repo.Room{ExternalID: "other", Name: "Other"})
	ex := f.exits.Insert(repo.Exit{
		FromRoomID: f.roomID, ToRoomID: other.ID, Direction: repo.DirNorth,
		Flags: repo.ExitFlags{
			Closed: true, Locked: true,
			AuthoredClosed: true, AuthoredLocked: true,
		},
	})
	return ex.ID
}

func TestZoneResetter_RestoresDoors(t *testing.T) {
	ctx := context.Background()
	f := newResetterFixture(t, repo.ZoneResetAlways, 60)
	doorID := authoredDoor(t, f)
	// Player opens + unlocks the door.
	if err := f.exits.UpdateFlags(ctx, doorID, false, false); err != nil {
		t.Fatalf("UpdateFlags: %v", err)
	}
	r := f.newResetter(func() time.Time { return time.Unix(2000, 0) }, nil, nil)
	r.Tick(ctx)

	got, err := f.exits.FindByDirection(ctx, f.roomID, repo.DirNorth)
	if err != nil {
		t.Fatalf("FindByDirection: %v", err)
	}
	if !got.Flags.Closed || !got.Flags.Locked {
		t.Fatalf("door not restored: %+v", got.Flags)
	}
	// last_reset_ts should have been stamped.
	ts, _ := f.zones.LastResetTs(ctx, f.zoneID)
	if ts != 2000 {
		t.Fatalf("last_reset_ts = %d, want 2000", ts)
	}
}

func TestZoneResetter_RecreatesMissingItems(t *testing.T) {
	ctx := context.Background()
	f := newResetterFixture(t, repo.ZoneResetAlways, 60)
	specs := map[string][]ItemSpec{
		"test-zone": {{
			ZoneExternalID: "test-zone",
			RoomExternalID: "test-room",
			Item: repo.Item{
				ExternalID: "test.pebble",
				Name:       "a pebble",
				NameLower:  "a pebble",
				ShortDesc:  "a small pebble",
				Type:       repo.ItemTypeTrash,
			},
		}},
	}
	r := f.newResetter(func() time.Time { return time.Unix(2000, 0) }, nil, specs)

	// First tick creates the item.
	r.Tick(ctx)
	got, err := f.items.FindByExternalID(ctx, "test.pebble")
	if err != nil {
		t.Fatalf("FindByExternalID after first tick: %v", err)
	}
	if got.RoomID != f.roomID {
		t.Fatalf("item RoomID = %d, want %d", got.RoomID, f.roomID)
	}

	// Player destroys the item; next interval-elapsed tick recreates it.
	if err := f.items.Delete(ctx, got.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	r.Now = func() time.Time { return time.Unix(2065, 0) }
	r.Tick(ctx)

	got2, err := f.items.FindByExternalID(ctx, "test.pebble")
	if err != nil {
		t.Fatalf("FindByExternalID after respawn: %v", err)
	}
	if got2.RoomID != f.roomID {
		t.Fatalf("respawned item RoomID = %d, want %d", got2.RoomID, f.roomID)
	}
	if got2.ID == got.ID {
		t.Errorf("respawned item reused ID %d (expected fresh row)", got2.ID)
	}
}

func TestZoneResetter_LeavesPlayerHeldItems(t *testing.T) {
	ctx := context.Background()
	f := newResetterFixture(t, repo.ZoneResetAlways, 60)
	specs := map[string][]ItemSpec{
		"test-zone": {{
			ZoneExternalID: "test-zone",
			RoomExternalID: "test-room",
			Item: repo.Item{
				ExternalID: "test.pebble", Name: "a pebble", NameLower: "a pebble",
				Type: repo.ItemTypeTrash,
			},
		}},
	}
	r := f.newResetter(func() time.Time { return time.Unix(2000, 0) }, nil, specs)

	r.Tick(ctx)
	first, err := f.items.FindByExternalID(ctx, "test.pebble")
	if err != nil {
		t.Fatalf("FindByExternalID: %v", err)
	}
	// Player picks it up.
	const charID int64 = 7
	if err := f.items.SetOwner(ctx, first.ID, charID); err != nil {
		t.Fatalf("SetOwner: %v", err)
	}

	// Reset interval elapses; next tick must NOT spawn a duplicate.
	r.Now = func() time.Time { return time.Unix(2065, 0) }
	r.Tick(ctx)

	inv, err := f.items.ListInInventory(ctx, charID)
	if err != nil {
		t.Fatalf("ListInInventory: %v", err)
	}
	if len(inv) != 1 || inv[0].ID != first.ID {
		t.Fatalf("inventory = %+v, want exactly the original item", inv)
	}
	floor, _ := f.items.ListInRoom(ctx, f.roomID)
	if len(floor) != 0 {
		t.Fatalf("floor got duplicate items: %+v", floor)
	}
}

func TestZoneResetter_LeavesContainerNestedItems(t *testing.T) {
	ctx := context.Background()
	f := newResetterFixture(t, repo.ZoneResetAlways, 60)
	specs := map[string][]ItemSpec{
		"test-zone": {{
			ZoneExternalID: "test-zone",
			RoomExternalID: "test-room",
			Item: repo.Item{
				ExternalID: "test.pebble", Name: "a pebble", NameLower: "a pebble",
				Type: repo.ItemTypeTrash,
			},
		}},
	}
	// Pre-create a container item.
	container, err := f.items.Create(ctx, repo.Item{
		ExternalID: "test.bag", Name: "a bag", NameLower: "a bag",
		Type: repo.ItemTypeContainer, RoomID: f.roomID,
		Stats: &repo.ContainerStats{CapacityLbs: 10},
	})
	if err != nil {
		t.Fatalf("Create bag: %v", err)
	}

	r := f.newResetter(func() time.Time { return time.Unix(2000, 0) }, nil, specs)
	r.Tick(ctx)
	pebble, err := f.items.FindByExternalID(ctx, "test.pebble")
	if err != nil {
		t.Fatalf("FindByExternalID: %v", err)
	}
	if err := f.items.SetParent(ctx, pebble.ID, container.ID); err != nil {
		t.Fatalf("SetParent: %v", err)
	}

	r.Now = func() time.Time { return time.Unix(2065, 0) }
	r.Tick(ctx)

	// Pebble must still be inside the container; no respawn on the floor.
	contents, err := f.items.ListInContainer(ctx, container.ID)
	if err != nil {
		t.Fatalf("ListInContainer: %v", err)
	}
	if len(contents) != 1 || contents[0].ID != pebble.ID {
		t.Fatalf("container contents = %+v, want exactly the pebble", contents)
	}
	floor, _ := f.items.ListInRoom(ctx, f.roomID)
	for _, it := range floor {
		if it.ExternalID == "test.pebble" {
			t.Fatalf("pebble respawned on floor while container-held: %+v", it)
		}
	}
}

func TestZoneResetter_NeverModeSkipsAllSteps(t *testing.T) {
	ctx := context.Background()
	f := newResetterFixture(t, repo.ZoneResetNever, 60)
	doorID := authoredDoor(t, f)
	if err := f.exits.UpdateFlags(ctx, doorID, false, false); err != nil {
		t.Fatalf("UpdateFlags: %v", err)
	}
	specs := map[string][]ItemSpec{
		"test-zone": {{
			ZoneExternalID: "test-zone",
			RoomExternalID: "test-room",
			Item: repo.Item{
				ExternalID: "test.pebble", Name: "a pebble", NameLower: "a pebble",
				Type: repo.ItemTypeTrash,
			},
		}},
	}
	r := f.newResetter(func() time.Time { return time.Unix(9000, 0) }, nil, specs)

	r.Tick(ctx)

	if got, _ := f.exits.FindByDirection(ctx, f.roomID, repo.DirNorth); got.Flags.Closed {
		t.Errorf("never-mode restored doors: %+v", got.Flags)
	}
	if _, err := f.items.FindByExternalID(ctx, "test.pebble"); err == nil {
		t.Errorf("never-mode created items")
	}
	if got := f.live(t); got != 0 {
		t.Errorf("never-mode spawned mobs, got %d", got)
	}
}

func TestZoneResetter_EmptyModeBlocksAllSteps(t *testing.T) {
	ctx := context.Background()
	f := newResetterFixture(t, repo.ZoneResetEmpty, 60)
	doorID := authoredDoor(t, f)
	if err := f.exits.UpdateFlags(ctx, doorID, false, false); err != nil {
		t.Fatalf("UpdateFlags: %v", err)
	}
	specs := map[string][]ItemSpec{
		"test-zone": {{
			ZoneExternalID: "test-zone",
			RoomExternalID: "test-room",
			Item: repo.Item{
				ExternalID: "test.pebble", Name: "a pebble", NameLower: "a pebble",
				Type: repo.ItemTypeTrash,
			},
		}},
	}
	occupied := true
	occ := OccupancyCheckerFunc(func(_ context.Context, zid int64) bool {
		return occupied && zid == f.zoneID
	})
	r := f.newResetter(func() time.Time { return time.Unix(2000, 0) }, occ, specs)

	r.Tick(ctx)

	if got, _ := f.exits.FindByDirection(ctx, f.roomID, repo.DirNorth); got.Flags.Closed {
		t.Errorf("empty-occupied restored doors: %+v", got.Flags)
	}
	if _, err := f.items.FindByExternalID(ctx, "test.pebble"); err == nil {
		t.Errorf("empty-occupied created items")
	}
	if got := f.live(t); got != 0 {
		t.Errorf("empty-occupied spawned mobs, got %d", got)
	}
	// last_reset_ts must NOT have been stamped on the deferred tick.
	if ts, _ := f.zones.LastResetTs(ctx, f.zoneID); ts != 0 {
		t.Errorf("deferred tick stamped last_reset_ts=%d", ts)
	}

	// Player leaves; tick again — all three steps run.
	occupied = false
	r.Tick(ctx)
	if got, _ := f.exits.FindByDirection(ctx, f.roomID, repo.DirNorth); !got.Flags.Closed {
		t.Errorf("door not restored after empty: %+v", got.Flags)
	}
	if _, err := f.items.FindByExternalID(ctx, "test.pebble"); err != nil {
		t.Errorf("item not created after empty: %v", err)
	}
	if got := f.live(t); got != 1 {
		t.Errorf("mob not spawned after empty, got %d", got)
	}
}
