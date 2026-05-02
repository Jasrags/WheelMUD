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
