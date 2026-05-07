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

	if err := LoadAndSync(ctx, conn, worldFS); err != nil {
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
	err = LoadAndSync(ctx, conn, worldFS)
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
	err = LoadAndSync(ctx, conn, worldFS)
	if err == nil || !strings.Contains(err.Error(), "invalid reset_mode") {
		t.Fatalf("err = %v, want invalid-reset-mode error", err)
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

	if err := LoadAndSync(ctx, conn, worldFS); err != nil {
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

	if err := LoadAndSync(ctx, conn, worldFS); err != nil {
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
	err = LoadAndSync(ctx, conn, worldFS)
	if err == nil {
		t.Fatal("want error on out-of-range open_hour")
	}
	if !strings.Contains(err.Error(), "open_hour") {
		t.Fatalf("err = %q, want it to mention open_hour", err)
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
	err = LoadAndSync(ctx, conn, worldFS)
	if err == nil {
		t.Fatal("want error on unknown stock item")
	}
	if !strings.Contains(err.Error(), "z.ghost") {
		t.Fatalf("err = %q, want it to mention z.ghost", err)
	}
}
