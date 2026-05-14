package world

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/db"
	"github.com/Jasrags/WheelMUD/internal/dialogue"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// goodWorld is the minimal happy-path fixture used by most cases. Two
// rooms, an exit pair, one item, one mob.
var goodWorld = fstest.MapFS{
	"starter/zone.yaml": &fstest.MapFile{Data: []byte(`
id: starter
name: Starter Town
`)},
	"starter/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: plaza.fountain
  starter: true
  name: Town Plaza
  short: A bustling town plaza.
  long: |
    Cobblestones radiate from a worn fountain.
  exits:
    n: plaza.north_road
- id: plaza.north_road
  name: North Road
  short: A quiet stretch of road.
  long: A quiet stretch.
  exits:
    s: plaza.fountain
`)},
	"starter/items.yaml": &fstest.MapFile{Data: []byte(`
- id: plaza.pebble
  room: plaza.fountain
  name: a small pebble
  short: a smooth grey pebble
`)},
	"starter/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: plaza.crier
  room: plaza.fountain
  name: a town crier
  short: shouts the day's news
`)},
}

func TestLoadAndSync_HappyPath(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	if _, err := LoadAndSync(ctx, conn, goodWorld); err != nil {
		t.Fatalf("LoadAndSync: %v", err)
	}

	rooms := repo.NewSQLiteRoomRepo(conn)
	exits := repo.NewSQLiteExitRepo(conn)
	items := repo.NewSQLiteItemRepo(conn)
	mobs := repo.NewSQLiteMobInstanceRepo(conn)

	// Starter pinned to id=1.
	starter, err := rooms.FindByID(ctx, repo.StarterRoomID)
	if err != nil {
		t.Fatalf("FindByID(starter): %v", err)
	}
	if starter.ExternalID != "plaza.fountain" {
		t.Fatalf("starter ExternalID = %q, want plaza.fountain", starter.ExternalID)
	}
	if starter.Name != "Town Plaza" {
		t.Fatalf("starter name = %q", starter.Name)
	}

	// Other room exists, distinct id.
	north, err := rooms.FindByExternalID(ctx, "plaza.north_road")
	if err != nil {
		t.Fatalf("FindByExternalID(north): %v", err)
	}
	if north.ID == starter.ID {
		t.Fatalf("north and starter share id %d", north.ID)
	}

	// Exit graph round-trips.
	north2plaza, err := exits.FindByDirection(ctx, north.ID, repo.DirSouth)
	if err != nil {
		t.Fatalf("FindByDirection(north, s): %v", err)
	}
	if north2plaza.ToRoomID != starter.ID {
		t.Fatalf("north south exit ToRoomID = %d, want starter %d", north2plaza.ToRoomID, starter.ID)
	}

	// Item + mob in the starter room.
	itemList, err := items.ListInRoom(ctx, starter.ID)
	if err != nil || len(itemList) != 1 || itemList[0].Name != "a small pebble" {
		t.Fatalf("items in starter: %v %+v", err, itemList)
	}
	mobList, err := mobs.ListInRoom(ctx, starter.ID)
	if err != nil || len(mobList) != 1 || mobList[0].Core.Name != "a town crier" {
		t.Fatalf("mobs in starter: %v %+v", err, mobList)
	}
}

func TestLoadAndSync_ReturnsItemSpecsByZone(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	loaded, err := LoadAndSync(ctx, conn, goodWorld)
	if err != nil {
		t.Fatalf("LoadAndSync: %v", err)
	}
	specs := loaded.ItemSpecsByZone["starter"]
	if len(specs) != 1 {
		t.Fatalf("starter zone specs = %d, want 1", len(specs))
	}
	got := specs[0]
	if got.RoomExternalID != "plaza.fountain" {
		t.Errorf("RoomExternalID = %q, want plaza.fountain", got.RoomExternalID)
	}
	if got.ZoneExternalID != "starter" {
		t.Errorf("ZoneExternalID = %q, want starter", got.ZoneExternalID)
	}
	if got.Item.ExternalID != "plaza.pebble" {
		t.Errorf("Item.ExternalID = %q, want plaza.pebble", got.Item.ExternalID)
	}
	if got.Item.Name != "a small pebble" {
		t.Errorf("Item.Name = %q", got.Item.Name)
	}

	// Second invocation against the now-populated DB still produces
	// the spec map (the parse path is unconditional).
	loaded2, err := LoadAndSync(ctx, conn, goodWorld)
	if err != nil {
		t.Fatalf("LoadAndSync (already loaded): %v", err)
	}
	if len(loaded2.ItemSpecsByZone["starter"]) != 1 {
		t.Errorf("re-load specs = %+v, want 1 entry",
			loaded2.ItemSpecsByZone["starter"])
	}
}

func TestLoadAndSync_ObjectFormExitsAttachDoorState(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"keep/zone.yaml": &fstest.MapFile{Data: []byte("id: keep\nname: Keep\n")},
		"keep/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: keep.gate
  starter: true
  name: Iron Gate
  long: A gate of bound iron.
  exits:
    n:
      to: keep.bailey
      closed: true
      locked: true
      key: iron.key
      difficulty: 15
      description: A heavy oak door bound with iron.
    s: keep.path
- id: keep.bailey
  name: Bailey
  long: An open courtyard.
  exits:
    s: keep.gate
- id: keep.path
  name: Path
  long: A muddy path.
  exits:
    n: keep.gate
`)},
	}

	if _, err := LoadAndSync(ctx, conn, worldFS); err != nil {
		t.Fatalf("LoadAndSync: %v", err)
	}
	exits := repo.NewSQLiteExitRepo(conn)
	gateNorth, err := exits.FindByDirection(ctx, repo.StarterRoomID, repo.DirNorth)
	if err != nil {
		t.Fatalf("FindByDirection: %v", err)
	}
	if !gateNorth.Flags.Closed || !gateNorth.Flags.Locked || !gateNorth.Flags.Pickable {
		t.Errorf("flags = %+v, want closed+locked+pickable", gateNorth.Flags)
	}
	if gateNorth.KeyExternalID != "iron.key" || gateNorth.LockDifficulty != 15 {
		t.Errorf("key/difficulty = %q/%d, want iron.key/15", gateNorth.KeyExternalID, gateNorth.LockDifficulty)
	}
	if gateNorth.Description == "" {
		t.Errorf("description dropped")
	}
	// Authored columns must mirror the YAML closed/locked at boot
	// time so ZoneResetter can restore them on AreaReset.
	if !gateNorth.Flags.AuthoredClosed || !gateNorth.Flags.AuthoredLocked {
		t.Errorf("authored door state not stamped: %+v", gateNorth.Flags)
	}
	gateSouth, err := exits.FindByDirection(ctx, repo.StarterRoomID, repo.DirSouth)
	if err != nil {
		t.Fatalf("FindByDirection south: %v", err)
	}
	if gateSouth.Flags.Closed || gateSouth.Flags.Locked {
		t.Errorf("shorthand exit got door flags: %+v", gateSouth.Flags)
	}
	if !gateSouth.Flags.Pickable {
		t.Errorf("shorthand exit should default Pickable=true; got %+v", gateSouth.Flags)
	}
	// Shorthand exit should also have AuthoredClosed/AuthoredLocked = false.
	if gateSouth.Flags.AuthoredClosed || gateSouth.Flags.AuthoredLocked {
		t.Errorf("shorthand exit got authored door flags: %+v", gateSouth.Flags)
	}
}

func TestLoadAndSync_ItemTaxonomyRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"smith/zone.yaml":  &fstest.MapFile{Data: []byte("id: smith\nname: Smithy\n")},
		"smith/rooms.yaml": &fstest.MapFile{Data: []byte("- id: smith.shop\n  starter: true\n  name: Forge\n  long: Hot.\n")},
		"smith/items.yaml": &fstest.MapFile{Data: []byte(`
- id: smith.longsword
  room: smith.shop
  name: a longsword
  short: A well-kept blade.
  type: weapon
  weight: 4
  value: "15mk"
  quality: masterwork
  flags: [magic, glow]
  stats:
    proficiency: martial
    size: medium
    range: melee
    damage: 1d8
    threat_low: 19
    crit_mult: 2
    damage_type: [S]
    special: [finesse]
- id: smith.iron_key
  room: smith.shop
  name: an iron key
  type: key
  weight: 0
  stats:
    key_id: keep.gate
- id: smith.pebble
  room: smith.shop
  name: a small pebble
`)},
	}

	if _, err := LoadAndSync(ctx, conn, worldFS); err != nil {
		t.Fatalf("LoadAndSync: %v", err)
	}

	items := repo.NewSQLiteItemRepo(conn)
	got, err := items.ListInRoom(ctx, repo.StarterRoomID)
	if err != nil {
		t.Fatalf("ListInRoom: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 items, got %d: %+v", len(got), got)
	}
	byID := map[string]repo.Item{}
	for _, it := range got {
		byID[it.ExternalID] = it
	}

	sword := byID["smith.longsword"]
	if sword.Type != repo.ItemTypeWeapon || sword.Quality != repo.QualityMasterwork {
		t.Errorf("sword type/quality wrong: %+v", sword)
	}
	if sword.Weight != 4 || sword.Value != 1500 {
		t.Errorf("sword weight/value wrong: weight=%g value=%d", sword.Weight, int64(sword.Value))
	}
	if !sword.HasFlag(repo.FlagMagic) || !sword.HasFlag(repo.FlagGlow) {
		t.Errorf("sword flags wrong: %b", sword.Flags)
	}
	ws, ok := sword.Stats.(*repo.WeaponStats)
	if !ok || ws.Damage != "1d8" || ws.ThreatLow != 19 || len(ws.Special) != 1 {
		t.Errorf("sword weapon stats wrong: %+v", sword.Stats)
	}

	key := byID["smith.iron_key"]
	if key.Type != repo.ItemTypeKey {
		t.Errorf("key type wrong: %s", key.Type)
	}
	ks, ok := key.Stats.(*repo.KeyStats)
	if !ok || ks.KeyID != "keep.gate" {
		t.Errorf("key stats wrong: %+v", key.Stats)
	}

	pebble := byID["smith.pebble"]
	if pebble.Type != repo.ItemTypeTrash {
		t.Errorf("untyped item should default to trash; got %s", pebble.Type)
	}
}

func TestLoadAndSync_ConsumableEffectIDStringResolves(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml":  &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte("- id: z.r\n  starter: true\n  name: R\n  long: x\n")},
		"z/items.yaml": &fstest.MapFile{Data: []byte(`
- id: z.potion
  room: z.r
  name: a potion
  type: consumable
  weight: 0.5
  stats:
    charges: 1
    effect_id_string: healing_draught
`)},
	}
	if _, err := LoadAndSync(ctx, conn, worldFS); err != nil {
		t.Fatalf("LoadAndSync: %v", err)
	}

	items := repo.NewSQLiteItemRepo(conn)
	got, err := items.ListInRoom(ctx, repo.StarterRoomID)
	if err != nil {
		t.Fatalf("ListInRoom: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	cs, ok := got[0].Stats.(*repo.ConsumableStats)
	if !ok {
		t.Fatalf("expected *ConsumableStats, got %T", got[0].Stats)
	}
	want := chargen.HashID("healing_draught")
	if cs.EffectID != want {
		t.Fatalf("EffectID: want %d (HashID of healing_draught), got %d", want, cs.EffectID)
	}
}

func TestLoadAndSync_RejectsUnknownItemType(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml":  &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte("- id: z.r\n  starter: true\n  name: R\n  long: x\n")},
		"z/items.yaml": &fstest.MapFile{Data: []byte("- id: z.bad\n  room: z.r\n  name: bad\n  type: floomf\n")},
	}
	if _, err := LoadAndSync(ctx, conn, worldFS); err == nil ||
		!strings.Contains(err.Error(), "unknown type") {
		t.Fatalf("want unknown-type error, got %v", err)
	}
}

func TestLoadAndSync_RejectsUnknownItemFlag(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml":  &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte("- id: z.r\n  starter: true\n  name: R\n  long: x\n")},
		"z/items.yaml": &fstest.MapFile{Data: []byte("- id: z.bad\n  room: z.r\n  name: bad\n  flags: [unknownflag]\n")},
	}
	if _, err := LoadAndSync(ctx, conn, worldFS); err == nil ||
		!strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("want unknown-flag error, got %v", err)
	}
}

func TestLoadAndSync_ZoneMetadataAndRoomLink(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"alpha/zone.yaml": &fstest.MapFile{Data: []byte(`
id: alpha
name: Alpha Town
builder: jrags
level_range: { min: 2, max: 8 }
reset_interval_s: 1200
reset_mode: always
climate: temperate
ambient:
  - The wind shifts in the eaves.
  - A bell tolls somewhere distant.
`)},
		"alpha/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: alpha.start
  starter: true
  name: Alpha Plaza
  short: An open plaza.
  long: Cobbles spread out under your feet.
  exits:
    n: alpha.north
- id: alpha.north
  name: Alpha North
  short: A road north.
  long: A road runs north.
  exits:
    s: alpha.start
`)},
		// A second zone with all defaults exercised, in a different
		// directory depth — proves the loader handles nested layouts
		// like the production data tree.
		"region/beta/zone.yaml":  &fstest.MapFile{Data: []byte("id: beta\nname: Beta\n")},
		"region/beta/rooms.yaml": &fstest.MapFile{Data: []byte("- id: beta.r\n  name: BR\n  long: x\n")},
	}

	if _, err := LoadAndSync(ctx, conn, worldFS); err != nil {
		t.Fatalf("LoadAndSync: %v", err)
	}

	zones := repo.NewSQLiteZoneRepo(conn)
	all, err := zones.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("zone count = %d, want 2", len(all))
	}

	alpha, err := zones.GetByExternalID(ctx, "alpha")
	if err != nil {
		t.Fatalf("GetByExternalID(alpha): %v", err)
	}
	if alpha.Builder != "jrags" {
		t.Errorf("Builder = %q, want jrags", alpha.Builder)
	}
	if alpha.MinLevel != 2 || alpha.MaxLevel != 8 {
		t.Errorf("level range = %d-%d, want 2-8", alpha.MinLevel, alpha.MaxLevel)
	}
	if alpha.ResetIntervalS != 1200 {
		t.Errorf("ResetIntervalS = %d, want 1200", alpha.ResetIntervalS)
	}
	if alpha.ResetMode != repo.ZoneResetAlways {
		t.Errorf("ResetMode = %q, want always", alpha.ResetMode)
	}
	if alpha.Climate != "temperate" {
		t.Errorf("Climate = %q", alpha.Climate)
	}
	if len(alpha.Ambient) != 2 {
		t.Errorf("Ambient len = %d, want 2", len(alpha.Ambient))
	}

	// Beta exercises every default.
	beta, err := zones.GetByExternalID(ctx, "beta")
	if err != nil {
		t.Fatalf("GetByExternalID(beta): %v", err)
	}
	if beta.MinLevel != 1 || beta.MaxLevel != 60 {
		t.Errorf("default level range = %d-%d, want 1-60", beta.MinLevel, beta.MaxLevel)
	}
	if beta.ResetIntervalS != 600 {
		t.Errorf("default ResetIntervalS = %d, want 600", beta.ResetIntervalS)
	}
	if beta.ResetMode != repo.ZoneResetEmpty {
		t.Errorf("default ResetMode = %q, want empty", beta.ResetMode)
	}
	if beta.Builder != "" || beta.Climate != "" {
		t.Errorf("defaults clobbered: builder=%q climate=%q", beta.Builder, beta.Climate)
	}
	if len(beta.Ambient) != 0 {
		t.Errorf("default Ambient = %v, want empty", beta.Ambient)
	}

	// Every room must point at its owning zone.
	rows, err := conn.QueryContext(ctx,
		`SELECT external_id, zone_id FROM rooms ORDER BY external_id`)
	if err != nil {
		t.Fatalf("query rooms: %v", err)
	}
	defer rows.Close()
	want := map[string]int64{
		"alpha.start": alpha.ID,
		"alpha.north": alpha.ID,
		"beta.r":      beta.ID,
	}
	got := make(map[string]int64, len(want))
	for rows.Next() {
		var ext string
		var zid int64
		if err := rows.Scan(&ext, &zid); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[ext] = zid
	}
	for ext, wantID := range want {
		if got[ext] != wantID {
			t.Errorf("room %q zone_id = %d, want %d", ext, got[ext], wantID)
		}
	}
}

func TestLoadAndSync_DuplicateZoneIDRejected(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"a/zone.yaml":  &fstest.MapFile{Data: []byte("id: dup\nname: A\n")},
		"a/rooms.yaml": &fstest.MapFile{Data: []byte("- id: a.r\n  starter: true\n  name: A\n  long: x\n")},
		"b/zone.yaml":  &fstest.MapFile{Data: []byte("id: dup\nname: B\n")},
		"b/rooms.yaml": &fstest.MapFile{Data: []byte("- id: b.r\n  name: B\n  long: x\n")},
	}
	_, err = LoadAndSync(ctx, conn, worldFS)
	if err == nil || !strings.Contains(err.Error(), "duplicate zone id") {
		t.Fatalf("err = %v, want duplicate-zone-id error", err)
	}
}

func TestLoadAndSync_InvalidResetModeRejected(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml":  &fstest.MapFile{Data: []byte("id: z\nname: Z\nreset_mode: blah\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte("- id: z.r\n  starter: true\n  name: R\n  long: x\n")},
	}
	_, err = LoadAndSync(ctx, conn, worldFS)
	if err == nil || !strings.Contains(err.Error(), "invalid reset_mode") {
		t.Fatalf("err = %v, want invalid-reset-mode error", err)
	}
}

// Phase F #32a slice 2: a mob with both `path` (strict-path) AND
// `wander_radius` (BFS) is rejected at load time. At runtime
// strict-path always wins; surfacing the inconsistency at boot
// keeps builders from chasing "why is my radius being ignored?".
func TestLoadAndSync_RejectsMobWithBothPathAndWanderRadius(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.a
  starter: true
  name: A
  long: room a
  exits: { e: z.b }
- id: z.b
  name: B
  long: room b
  exits: { w: z.a }
`)},
		"z/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: z.confused
  room: z.a
  name: a confused traveler
  short: a confused traveler
  path: [z.a, z.b]
  wander_radius: 2
`)},
	}
	_, err = LoadAndSync(ctx, conn, worldFS)
	if err == nil || !strings.Contains(err.Error(), "cannot set both `path` and `wander_radius`") {
		t.Fatalf("err = %v, want path+wander_radius rejection", err)
	}
}

func TestLoadAndSync_FirstStarterWinsAcrossResync(t *testing.T) {
	// Resync semantics: when the DB already has a row at
	// repo.StarterRoomID (id=1), a subsequent LoadAndSync with a
	// different "starter: true" declaration in YAML must NOT try to
	// force id=1 again (would violate the UNIQUE PK and abort the
	// resync). The first-loaded starter is preserved; the new YAML's
	// starter still lands as an ordinary auto-increment row so its
	// rooms / exits / items remain reachable from the world graph.
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	if _, err := LoadAndSync(ctx, conn, goodWorld); err != nil {
		t.Fatalf("first load: %v", err)
	}
	other := fstest.MapFS{
		"other/zone.yaml": &fstest.MapFile{Data: []byte("id: other\nname: Other\n")},
		"other/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: other.start
  starter: true
  name: Different
  short: x
  long: x
`)},
	}
	if _, err := LoadAndSync(ctx, conn, other); err != nil {
		t.Fatalf("second load: %v", err)
	}

	rooms := repo.NewSQLiteRoomRepo(conn)
	r, err := rooms.FindByID(ctx, repo.StarterRoomID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if r.ExternalID != "plaza.fountain" {
		t.Fatalf("ExternalID = %q, expected first-load value plaza.fountain", r.ExternalID)
	}
	// The new YAML's starter must still have landed somewhere — its
	// auto-increment id will not be 1, but it should be findable.
	otherRoom, err := rooms.FindByExternalID(ctx, "other.start")
	if err != nil {
		t.Fatalf("FindByExternalID(other.start): %v", err)
	}
	if otherRoom.ID == repo.StarterRoomID {
		t.Fatalf("other.start collided with starter id %d", otherRoom.ID)
	}
}

// failingCases drives validation-failure paths through the same loader
// against a fresh in-memory DB per case. Each case builds a MapFS and
// asserts the returned error message contains a specific substring.
func TestLoadAndSync_ValidationFailures(t *testing.T) {
	cases := []struct {
		name     string
		fs       fstest.MapFS
		wantErrs []string
	}{
		{
			name: "no starter",
			fs: fstest.MapFS{
				"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
				"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.a
  name: A
  short: a
  long: a
`)},
			},
			wantErrs: []string{"no room marked", "starter"},
		},
		{
			name: "multiple starters",
			fs: fstest.MapFS{
				"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
				"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.a
  starter: true
  name: A
  short: a
  long: a
- id: z.b
  starter: true
  name: B
  short: b
  long: b
`)},
			},
			wantErrs: []string{"multiple rooms marked"},
		},
		{
			name: "duplicate room id",
			fs: fstest.MapFS{
				"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
				"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.a
  starter: true
  name: A
  short: a
  long: a
- id: z.a
  name: B
  short: b
  long: b
`)},
			},
			wantErrs: []string{"duplicate room id"},
		},
		{
			name: "exit to unknown room",
			fs: fstest.MapFS{
				"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
				"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.a
  starter: true
  name: A
  short: a
  long: a
  exits:
    n: ghost
`)},
			},
			wantErrs: []string{"unknown room", "ghost"},
		},
		{
			name: "invalid direction",
			fs: fstest.MapFS{
				"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
				"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.a
  starter: true
  name: A
  short: a
  long: a
  exits:
    nope: z.a
`)},
			},
			wantErrs: []string{"invalid exit direction"},
		},
		{
			name: "item to unknown room",
			fs: fstest.MapFS{
				"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
				"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.a
  starter: true
  name: A
  short: a
  long: a
`)},
				"z/items.yaml": &fstest.MapFile{Data: []byte(`
- id: z.thing
  room: ghost
  name: a thing
  short: x
`)},
			},
			wantErrs: []string{"unknown room", "ghost"},
		},
		{
			name: "duplicate item id",
			fs: fstest.MapFS{
				"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
				"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.a
  starter: true
  name: A
  short: a
  long: a
`)},
				"z/items.yaml": &fstest.MapFile{Data: []byte(`
- id: dup
  room: z.a
  name: a
  short: a
- id: dup
  room: z.a
  name: b
  short: b
`)},
			},
			wantErrs: []string{"duplicate item id"},
		},
		{
			name:     "no zones",
			fs:       fstest.MapFS{},
			wantErrs: []string{"no zone.yaml"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			conn, err := db.Open(ctx, ":memory:")
			if err != nil {
				t.Fatalf("open db: %v", err)
			}
			t.Cleanup(func() { conn.Close() })

			_, err = LoadAndSync(ctx, conn, tc.fs)
			if err == nil {
				t.Fatalf("want error; got nil")
			}
			msg := err.Error()
			for _, want := range tc.wantErrs {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q missing %q", msg, want)
				}
			}

			// And the DB must still be empty — failed loads must not
			// half-populate.
			rooms := repo.NewSQLiteRoomRepo(conn)
			if _, err := rooms.FindByID(ctx, repo.StarterRoomID); err == nil {
				t.Errorf("DB has rows after failed load; transaction should have rolled back")
			}
		})
	}
}

func TestLoadAndSync_ShopRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"inn/zone.yaml":  &fstest.MapFile{Data: []byte("id: inn\nname: Inn\n")},
		"inn/rooms.yaml": &fstest.MapFile{Data: []byte("- id: inn.common\n  starter: true\n  name: Common Room\n  long: A warm hearth.\n")},
		"inn/items.yaml": &fstest.MapFile{Data: []byte(`
- id: inn.ale
  room: inn.common
  name: a mug of ale
  type: consumable
  value: "5cp"
- id: inn.bread
  room: inn.common
  name: a loaf of bread
  type: food
  value: "2cp"
`)},
		"inn/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: inn.bran
  room: inn.common
  name: Bran al'Vere
  short: the round-faced innkeeper
  shop:
    buy_types: [food, consumable, trade_good]
    sell_markup: 1.0
    buy_markdown: 0.5
    open_hour: 6
    close_hour: 22
    restock_interval_s: 300
    stock:
      - item: inn.ale
        qty: 12
        qty_max: 12
      - item: inn.bread
        qty: -1
        qty_max: -1
`)},
	}

	if _, err := LoadAndSync(ctx, conn, worldFS); err != nil {
		t.Fatalf("LoadAndSync: %v", err)
	}

	templates := repo.NewSQLiteMobTemplateRepo(conn)
	tpl, err := templates.GetByExternalID(ctx, "inn.bran")
	if err != nil {
		t.Fatalf("template lookup: %v", err)
	}

	shops := repo.NewSQLiteShopRepo(conn)
	shop, err := shops.GetByMobTemplateID(ctx, tpl.ID)
	if err != nil {
		t.Fatalf("shop lookup: %v", err)
	}
	if shop.SellMarkup != 1.0 || shop.BuyMarkdown != 0.5 ||
		shop.OpenHour != 6 || shop.CloseHour != 22 ||
		shop.RestockIntervalS != 300 {
		t.Fatalf("shop scalars wrong: %+v", shop)
	}
	wantTypes := map[repo.ItemType]bool{repo.ItemTypeFood: true, repo.ItemTypeConsumable: true, repo.ItemTypeTradeGood: true}
	if len(shop.BuyTypes) != 3 {
		t.Fatalf("buy_types len = %d", len(shop.BuyTypes))
	}
	for _, bt := range shop.BuyTypes {
		if !wantTypes[bt] {
			t.Fatalf("unexpected buy type %q", bt)
		}
	}

	stock, err := shops.ListStock(ctx, shop.ID)
	if err != nil {
		t.Fatalf("ListStock: %v", err)
	}
	if len(stock) != 2 {
		t.Fatalf("got %d stock rows, want 2", len(stock))
	}
	// Sorted by external_id: ale first, bread second.
	if stock[0].ItemExternalID != "inn.ale" || stock[0].Qty != 12 || stock[0].QtyMax != 12 {
		t.Fatalf("ale row wrong: %+v", stock[0])
	}
	if stock[1].ItemExternalID != "inn.bread" || stock[1].Qty != -1 || stock[1].QtyMax != -1 {
		t.Fatalf("bread row (infinite) wrong: %+v", stock[1])
	}
}

func TestLoadAndSync_BankerRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"city/zone.yaml":  &fstest.MapFile{Data: []byte("id: city\nname: City\n")},
		"city/rooms.yaml": &fstest.MapFile{Data: []byte("- id: city.bank\n  starter: true\n  name: Bank\n  long: A vault.\n")},
		"city/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: city.banker
  room: city.bank
  name: Jain the Moneylender
  short: a well-dressed moneylender
  banker:
    open_hour: 8
    close_hour: 18
`)},
	}

	if _, err := LoadAndSync(ctx, conn, worldFS); err != nil {
		t.Fatalf("LoadAndSync: %v", err)
	}

	templates := repo.NewSQLiteMobTemplateRepo(conn)
	tpl, err := templates.GetByExternalID(ctx, "city.banker")
	if err != nil {
		t.Fatalf("template lookup: %v", err)
	}

	bankers := repo.NewSQLiteBankerRepo(conn)
	b, err := bankers.GetByMobTemplateID(ctx, tpl.ID)
	if err != nil {
		t.Fatalf("banker lookup: %v", err)
	}
	if b.OpenHour != 8 || b.CloseHour != 18 {
		t.Fatalf("banker hours wrong: %+v", b)
	}
}

func TestLoadAndSync_BankerRejectsBadHour(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml":  &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte("- id: z.r\n  starter: true\n  name: R\n  long: x\n")},
		"z/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: z.banker
  room: z.r
  name: Bad Banker
  banker:
    open_hour: 25
    close_hour: 9
`)},
	}
	_, err = LoadAndSync(ctx, conn, worldFS)
	if err == nil {
		t.Fatal("want error on out-of-range open_hour")
	}
	if !strings.Contains(err.Error(), "open_hour") {
		t.Fatalf("err = %q, want it to mention open_hour", err)
	}
}

func TestLoadAndSync_TrainerRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"city/zone.yaml":  &fstest.MapFile{Data: []byte("id: city\nname: City\n")},
		"city/rooms.yaml": &fstest.MapFile{Data: []byte("- id: city.hall\n  starter: true\n  name: Hall\n  long: A training hall.\n")},
		"city/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: city.weaponmaster
  room: city.hall
  name: Lan the Weaponmaster
  short: a hardened weaponmaster
  trainer:
    class: armsman
`)},
	}

	if _, err := LoadAndSync(ctx, conn, worldFS); err != nil {
		t.Fatalf("LoadAndSync: %v", err)
	}

	templates := repo.NewSQLiteMobTemplateRepo(conn)
	tpl, err := templates.GetByExternalID(ctx, "city.weaponmaster")
	if err != nil {
		t.Fatalf("template lookup: %v", err)
	}

	trainers := repo.NewSQLiteTrainerRepo(conn)
	tr, err := trainers.GetByMobTemplateID(ctx, tpl.ID)
	if err != nil {
		t.Fatalf("trainer lookup: %v", err)
	}
	if tr.ClassID != "armsman" {
		t.Fatalf("trainer class wrong: %+v", tr)
	}
}

func TestLoadAndSync_TrainerRejectsEmptyClass(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml":  &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte("- id: z.r\n  starter: true\n  name: R\n  long: x\n")},
		"z/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: z.trainer
  room: z.r
  name: Mystery Trainer
  trainer:
    class: ""
`)},
	}
	_, err = LoadAndSync(ctx, conn, worldFS)
	if err == nil {
		t.Fatal("want error on empty trainer.class")
	}
	if !strings.Contains(err.Error(), "trainer.class") {
		t.Fatalf("err = %q, want it to mention trainer.class", err)
	}
}

func TestLoadAndSync_WeaveTeacherRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"city/zone.yaml":  &fstest.MapFile{Data: []byte("id: city\nname: City\n")},
		"city/rooms.yaml": &fstest.MapFile{Data: []byte("- id: city.tower\n  starter: true\n  name: Tower\n  long: A weave-teacher's chamber.\n")},
		"city/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: city.aes_sedai
  room: city.tower
  name: Sister Anaiya
  short: a kindly Aes Sedai
  weave_teacher:
    max_level_taught: 1
    affinity_filter: [air, fire]
`)},
	}

	if _, err := LoadAndSync(ctx, conn, worldFS); err != nil {
		t.Fatalf("LoadAndSync: %v", err)
	}

	templates := repo.NewSQLiteMobTemplateRepo(conn)
	tpl, err := templates.GetByExternalID(ctx, "city.aes_sedai")
	if err != nil {
		t.Fatalf("template lookup: %v", err)
	}
	teachers := repo.NewSQLiteWeaveTeacherRepo(conn)
	teacher, err := teachers.GetByMobTemplateID(ctx, tpl.ID)
	if err != nil {
		t.Fatalf("teacher lookup: %v", err)
	}
	if teacher.MaxLevelTaught != 1 {
		t.Errorf("MaxLevelTaught = %d, want 1", teacher.MaxLevelTaught)
	}
	wantFilter := creature.PowerSet(1<<creature.PowerAir | 1<<creature.PowerFire)
	if teacher.AffinityFilter != wantFilter {
		t.Errorf("AffinityFilter = %d, want %d", teacher.AffinityFilter, wantFilter)
	}
}

func TestLoadAndSync_WeaveTeacherRejectsBadPower(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml":  &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte("- id: z.r\n  starter: true\n  name: R\n  long: x\n")},
		"z/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: z.teacher
  room: z.r
  name: Suspicious Teacher
  weave_teacher:
    max_level_taught: 0
    affinity_filter: [chaos]
`)},
	}
	_, err = LoadAndSync(ctx, conn, worldFS)
	if err == nil {
		t.Fatal("want error on bad Power name")
	}
	if !strings.Contains(err.Error(), "weave_teacher") {
		t.Fatalf("err = %q, want it to mention weave_teacher", err)
	}
}

func TestLoadAndSync_ShopRejectsUnknownItem(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml":  &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte("- id: z.r\n  starter: true\n  name: R\n  long: x\n")},
		"z/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: z.bran
  room: z.r
  name: Bran
  shop:
    buy_types: [food]
    stock:
      - item: z.ghost
        qty: 1
        qty_max: 1
`)},
	}
	_, err = LoadAndSync(ctx, conn, worldFS)
	if err == nil {
		t.Fatal("want error on unknown stock item")
	}
	if !strings.Contains(err.Error(), "z.ghost") {
		t.Fatalf("err = %q, want it to mention z.ghost", err)
	}
}

func TestLoadAndSync_TriggersRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.commons
  starter: true
  name: Commons
  long: A common room.
  triggers:
    - event: on_enter
      action: noop
      payload:
        message: "someone arrives"
      priority: 5
`)},
		"z/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: z.innkeeper
  room: z.commons
  name: the innkeeper
  triggers:
    - event: on_say
      match: rumor
      action: emote
      payload:
        text: "leans in conspiratorially."
    - event: on_enter
      action: say
      payload:
        text: "Welcome to the inn."
`)},
	}

	if _, err := LoadAndSync(ctx, conn, worldFS); err != nil {
		t.Fatalf("LoadAndSync: %v", err)
	}

	triggers := repo.NewSQLiteTriggerRepo(conn)
	roomRows, err := triggers.ListByOwner(ctx, repo.TriggerOwnerRoom, repo.StarterRoomID)
	if err != nil {
		t.Fatalf("list room triggers: %v", err)
	}
	if len(roomRows) != 1 {
		t.Fatalf("room triggers len = %d, want 1", len(roomRows))
	}
	if roomRows[0].Event != repo.TriggerEventOnEnter || roomRows[0].Action != "noop" {
		t.Errorf("room trigger: %+v", roomRows[0])
	}
	if !strings.Contains(roomRows[0].Payload, "someone arrives") {
		t.Errorf("room trigger payload: %q", roomRows[0].Payload)
	}
	if roomRows[0].Priority != 5 {
		t.Errorf("priority = %d, want 5", roomRows[0].Priority)
	}

	mobs := repo.NewSQLiteMobTemplateRepo(conn)
	tpl, err := mobs.GetByExternalID(ctx, "z.innkeeper")
	if err != nil {
		t.Fatalf("template: %v", err)
	}
	mobRows, err := triggers.ListByOwner(ctx, repo.TriggerOwnerMobTemplate, tpl.ID)
	if err != nil {
		t.Fatalf("list mob triggers: %v", err)
	}
	if len(mobRows) != 2 {
		t.Fatalf("mob triggers len = %d, want 2", len(mobRows))
	}
	var sawOnSay, sawOnEnter bool
	for _, r := range mobRows {
		if r.Event == repo.TriggerEventOnSay {
			sawOnSay = true
			if r.Match != "rumor" || r.Action != "emote" {
				t.Errorf("on_say row: %+v", r)
			}
		}
		if r.Event == repo.TriggerEventOnEnter {
			sawOnEnter = true
			if r.Action != "say" {
				t.Errorf("on_enter row: %+v", r)
			}
		}
	}
	if !sawOnSay || !sawOnEnter {
		t.Fatalf("missing one of the events: rows=%+v", mobRows)
	}
}

func TestLoadAndSync_TriggerRejectsBadEvent(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.r
  starter: true
  name: R
  long: x
  triggers:
    - event: on_lol
      action: noop
`)},
	}
	_, err = LoadAndSync(ctx, conn, worldFS)
	if err == nil {
		t.Fatal("want error on unknown event")
	}
	if !strings.Contains(err.Error(), "on_lol") {
		t.Fatalf("err = %q, want it to mention on_lol", err)
	}
}

func TestLoadAndSync_TriggerRejectsScalarPayload(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	// A bare-string payload would JSON-marshal to "hello" and silently
	// no-op every action handler at fire time. Boot must reject it.
	worldFS := fstest.MapFS{
		"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.r
  starter: true
  name: R
  long: x
  triggers:
    - event: on_enter
      action: say
      payload: "hello"
`)},
	}
	_, err = LoadAndSync(ctx, conn, worldFS)
	if err == nil {
		t.Fatal("want error on scalar payload")
	}
	if !strings.Contains(err.Error(), "payload must be a mapping") {
		t.Fatalf("err = %q", err)
	}
}

func TestLoadAndSync_DialogueRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.commons
  starter: true
  name: Commons
  long: A common room.
`)},
		"z/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: z.elder
  room: z.commons
  name: village elder
  dialogue:
    root: greet
    nodes:
      - id: greet
        prompt: "Greetings, traveler."
        responses:
          - match: [hello, hi]
            reply: "Well met."
            next: farewell
          - match: [quest]
            effects:
              - kind: set_flag
                args:
                  name: started_quest
            next: farewell
      - id: farewell
        prompt: "Travel safely."
`)},
	}

	if _, err := LoadAndSync(ctx, conn, worldFS); err != nil {
		t.Fatalf("LoadAndSync: %v", err)
	}

	mobs := repo.NewSQLiteMobTemplateRepo(conn)
	tpl, err := mobs.GetByExternalID(ctx, "z.elder")
	if err != nil {
		t.Fatalf("template: %v", err)
	}
	if len(tpl.DialogueJSON) == 0 {
		t.Fatal("DialogueJSON empty after roundtrip")
	}
	var got dialogue.Tree
	if err := json.Unmarshal(tpl.DialogueJSON, &got); err != nil {
		t.Fatalf("unmarshal dialogue: %v", err)
	}
	if got.Root != "greet" {
		t.Fatalf("root = %q, want greet", got.Root)
	}
	greet, ok := got.Nodes["greet"]
	if !ok {
		t.Fatal("missing greet node")
	}
	if len(greet.Responses) != 2 {
		t.Fatalf("greet responses len = %d, want 2", len(greet.Responses))
	}
	if greet.Responses[1].Effects[0].Kind != dialogue.EffectSetFlag {
		t.Fatalf("effect kind = %q", greet.Responses[1].Effects[0].Kind)
	}
}

func TestLoadAndSync_DialogueRejectsDanglingNext(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.r
  starter: true
  name: R
  long: x
`)},
		"z/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: z.npc
  room: z.r
  name: stranger
  dialogue:
    root: greet
    nodes:
      - id: greet
        prompt: hi
        responses:
          - match: [hello]
            next: ghost
`)},
	}

	_, err = LoadAndSync(ctx, conn, worldFS)
	if err == nil {
		t.Fatal("want error on dangling next")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("err = %q, want it to mention ghost", err)
	}
}

func TestLoadAndSync_DialogueRejectsDuplicateNodeID(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.r
  starter: true
  name: R
  long: x
`)},
		"z/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: z.npc
  room: z.r
  name: stranger
  dialogue:
    root: greet
    nodes:
      - id: greet
        prompt: hi
      - id: greet
        prompt: also hi
`)},
	}

	_, err = LoadAndSync(ctx, conn, worldFS)
	if err == nil {
		t.Fatal("want error on duplicate node id")
	}
	if !strings.Contains(err.Error(), "duplicate dialogue node id") {
		t.Fatalf("err = %q", err)
	}
}

func TestLoadAndSync_TriggerRejectsEmptyAction(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	worldFS := fstest.MapFS{
		"z/zone.yaml": &fstest.MapFile{Data: []byte("id: z\nname: Z\n")},
		"z/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: z.r
  starter: true
  name: R
  long: x
  triggers:
    - event: on_enter
      action: ""
`)},
	}
	_, err = LoadAndSync(ctx, conn, worldFS)
	if err == nil {
		t.Fatal("want error on empty action")
	}
	if !strings.Contains(err.Error(), "action is empty") {
		t.Fatalf("err = %q", err)
	}
}
