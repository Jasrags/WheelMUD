package world

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Jasrags/WheelMUD/internal/db"
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

	if err := LoadAndSync(ctx, conn, goodWorld); err != nil {
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

	if err := LoadAndSync(ctx, conn, worldFS); err != nil {
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

	if err := LoadAndSync(ctx, conn, worldFS); err != nil {
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
	if err := LoadAndSync(ctx, conn, worldFS); err == nil ||
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
	if err := LoadAndSync(ctx, conn, worldFS); err == nil ||
		!strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("want unknown-flag error, got %v", err)
	}
}

func TestLoadAndSync_AlreadyLoadedIsNoop(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	if err := LoadAndSync(ctx, conn, goodWorld); err != nil {
		t.Fatalf("first load: %v", err)
	}
	// Second load with a *different* world should still be a no-op.
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
	if err := LoadAndSync(ctx, conn, other); err != nil {
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
			name: "no zones",
			fs:   fstest.MapFS{},
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

			err = LoadAndSync(ctx, conn, tc.fs)
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
