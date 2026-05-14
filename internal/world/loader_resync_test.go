package world

import (
	"context"
	"database/sql"
	"testing"
	"testing/fstest"

	"github.com/Jasrags/WheelMUD/internal/db"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// Resync tests live in their own file so the legacy loader_test.go
// stays a stable record of validation and round-trip coverage.

// resyncWorldA is the minimal fixture every resync test starts from:
// one zone, two rooms (starter + north), one exit pair, one item in
// the starter, one mob template in the starter.
var resyncWorldA = fstest.MapFS{
	"starter/zone.yaml": &fstest.MapFile{Data: []byte("id: starter\nname: Starter\n")},
	"starter/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: plaza.fountain
  starter: true
  name: Plaza
  short: A plaza.
  long: A plaza.
  exits:
    n: plaza.north
- id: plaza.north
  name: North
  short: North.
  long: North.
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

// resyncWorldB adds one room ("plaza.south") off the starter plus one
// item that lives in the new room and a new mob. Mirrors the
// production bug: a YAML edit landing after first boot that
// references a brand-new room from a brand-new item.
var resyncWorldB = fstest.MapFS{
	"starter/zone.yaml": &fstest.MapFile{Data: []byte("id: starter\nname: Starter\n")},
	"starter/rooms.yaml": &fstest.MapFile{Data: []byte(`
- id: plaza.fountain
  starter: true
  name: Plaza
  short: A plaza.
  long: A plaza.
  exits:
    n: plaza.north
    s: plaza.south
- id: plaza.north
  name: North
  short: North.
  long: North.
  exits:
    s: plaza.fountain
- id: plaza.south
  name: South Annex
  short: A southern annex.
  long: A southern annex.
  exits:
    n: plaza.fountain
`)},
	"starter/items.yaml": &fstest.MapFile{Data: []byte(`
- id: plaza.pebble
  room: plaza.fountain
  name: a small pebble
  short: a smooth grey pebble
- id: plaza.lantern
  room: plaza.south
  name: a brass lantern
  short: a brass lantern, dimly glowing
`)},
	"starter/mobs.yaml": &fstest.MapFile{Data: []byte(`
- id: plaza.crier
  room: plaza.fountain
  name: a town crier
  short: shouts the day's news
- id: plaza.guard
  room: plaza.south
  name: a watchful guard
  short: stands at attention
`)},
}

// worldSnapshot captures row counts across the world tables so tests
// can assert "nothing changed" after a no-op resync.
type worldSnapshot struct {
	zones, rooms, exits, items, mobTemplates, mobInstances int
}

func takeSnapshot(t *testing.T, conn *sql.DB) worldSnapshot {
	t.Helper()
	ctx := context.Background()
	count := func(table string) int {
		var n int
		row := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table)
		if err := row.Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return n
	}
	return worldSnapshot{
		zones:        count("zones"),
		rooms:        count("rooms"),
		exits:        count("exits"),
		items:        count("items"),
		mobTemplates: count("mob_templates"),
		mobInstances: count("mob_instances"),
	}
}

// openTestDB returns a fresh in-memory DB plus a cleanup hook.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestResync_EmptyDBInsertsEverything(t *testing.T) {
	conn := openTestDB(t)
	if _, err := LoadAndSync(context.Background(), conn, resyncWorldA); err != nil {
		t.Fatalf("LoadAndSync: %v", err)
	}
	snap := takeSnapshot(t, conn)
	if snap.zones != 1 || snap.rooms != 2 || snap.exits != 2 ||
		snap.items != 1 || snap.mobTemplates != 1 || snap.mobInstances != 1 {
		t.Fatalf("snapshot after first load: %+v", snap)
	}
}

func TestResync_SecondLoadIsIdempotent(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()
	if _, err := LoadAndSync(ctx, conn, resyncWorldA); err != nil {
		t.Fatalf("first load: %v", err)
	}
	before := takeSnapshot(t, conn)

	if _, err := LoadAndSync(ctx, conn, resyncWorldA); err != nil {
		t.Fatalf("second load: %v", err)
	}
	after := takeSnapshot(t, conn)

	if before != after {
		t.Fatalf("idempotence violated:\n before=%+v\n after =%+v", before, after)
	}
}

func TestResync_PreservesDriftedRowEdits(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()
	if _, err := LoadAndSync(ctx, conn, resyncWorldA); err != nil {
		t.Fatalf("first load: %v", err)
	}

	// Hand-edit a room's name to simulate a runtime mutation (OLC
	// edit, manual SQL). The resync must NOT overwrite it.
	if _, err := conn.ExecContext(ctx,
		`UPDATE rooms SET name = 'edited' WHERE external_id = 'plaza.fountain'`); err != nil {
		t.Fatalf("hand-edit: %v", err)
	}

	if _, err := LoadAndSync(ctx, conn, resyncWorldA); err != nil {
		t.Fatalf("second load: %v", err)
	}

	rooms := repo.NewSQLiteRoomRepo(conn)
	r, err := rooms.FindByExternalID(ctx, "plaza.fountain")
	if err != nil {
		t.Fatalf("FindByExternalID: %v", err)
	}
	if r.Name != "edited" {
		t.Fatalf("resync clobbered drift: name = %q, want 'edited'", r.Name)
	}
}

func TestResync_AddsNewContentToExistingDB(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()
	if _, err := LoadAndSync(ctx, conn, resyncWorldA); err != nil {
		t.Fatalf("first load: %v", err)
	}
	before := takeSnapshot(t, conn)

	if _, err := LoadAndSync(ctx, conn, resyncWorldB); err != nil {
		t.Fatalf("second load (with new content): %v", err)
	}
	after := takeSnapshot(t, conn)

	// World B adds: +1 room (plaza.south), +2 exits (plaza.fountain
	// s, plaza.south n), +1 item (plaza.lantern), +1 mob template
	// (plaza.guard) + 1 mob instance.
	want := worldSnapshot{
		zones:        before.zones,
		rooms:        before.rooms + 1,
		exits:        before.exits + 2,
		items:        before.items + 1,
		mobTemplates: before.mobTemplates + 1,
		mobInstances: before.mobInstances + 1,
	}
	if after != want {
		t.Fatalf("new-content resync:\n before=%+v\n after =%+v\n want  =%+v", before, after, want)
	}
}

// TestResync_NewItemInNewRoomGetsCorrectRoomID is the exact
// regression for the production "zoneresetter: resolve home room"
// warnings. An item whose `room:` references a brand-new room must
// end up with the right `room_id` after resync — not 0 (the silent-
// fallback bug) and not a stale id from before.
func TestResync_NewItemInNewRoomGetsCorrectRoomID(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()
	if _, err := LoadAndSync(ctx, conn, resyncWorldA); err != nil {
		t.Fatalf("first load: %v", err)
	}
	if _, err := LoadAndSync(ctx, conn, resyncWorldB); err != nil {
		t.Fatalf("second load: %v", err)
	}

	rooms := repo.NewSQLiteRoomRepo(conn)
	south, err := rooms.FindByExternalID(ctx, "plaza.south")
	if err != nil {
		t.Fatalf("FindByExternalID(plaza.south): %v", err)
	}

	items := repo.NewSQLiteItemRepo(conn)
	inSouth, err := items.ListInRoom(ctx, south.ID)
	if err != nil {
		t.Fatalf("ListInRoom(south): %v", err)
	}
	if len(inSouth) != 1 {
		t.Fatalf("south room contents: got %d items, want 1: %+v", len(inSouth), inSouth)
	}
	if inSouth[0].ExternalID != "plaza.lantern" {
		t.Fatalf("south item ExternalID = %q, want plaza.lantern", inSouth[0].ExternalID)
	}
}

func TestResync_NewMobTemplateSpawnsOneInstance(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()
	if _, err := LoadAndSync(ctx, conn, resyncWorldA); err != nil {
		t.Fatalf("first load: %v", err)
	}
	if _, err := LoadAndSync(ctx, conn, resyncWorldB); err != nil {
		t.Fatalf("second load: %v", err)
	}

	rooms := repo.NewSQLiteRoomRepo(conn)
	south, err := rooms.FindByExternalID(ctx, "plaza.south")
	if err != nil {
		t.Fatalf("FindByExternalID(plaza.south): %v", err)
	}

	mobs := repo.NewSQLiteMobInstanceRepo(conn)
	inSouth, err := mobs.ListInRoom(ctx, south.ID)
	if err != nil {
		t.Fatalf("ListInRoom(south mobs): %v", err)
	}
	if len(inSouth) != 1 {
		t.Fatalf("south mob count = %d, want exactly 1", len(inSouth))
	}
	if inSouth[0].Core.Name != "a watchful guard" {
		t.Fatalf("south mob name = %q, want 'a watchful guard'", inSouth[0].Core.Name)
	}
}
