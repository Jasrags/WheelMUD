package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// zoneMapFix builds two zones with a few rooms each so cross-zone
// rendering, sector counts, vertical exits, and flagged rooms can
// all be exercised by a single fixture.
//
// Layout (ids are sequential; id=1 is the starter):
//
//	[F:north_road, two_rivers]   (off-zone, north of the green)
//	         |
//	         n
//	         |
//	[C:green*, emonds_field]--e--[C:inn_yard, emonds_field]--e--[C:inn_common, off-zone, winespring_inn]
//	         |  \                         |
//	         s   sw                       u (vertical exit)
//	         |                            |
//	[f:south_road]                  [C:guest_a, emonds_field]
//
//	[C:dark_cellar, emonds_field, dark+silent flags]  // unreachable, just for flag listing
type zoneMapFix struct {
	rooms  *repo.MemoryRoomRepo
	exits  *repo.MemoryExitRepo
	zones  *repo.MemoryZoneRepo
	emonds repo.Zone
}

func newZoneMapFix(t *testing.T) zoneMapFix {
	t.Helper()
	zones := repo.NewMemoryZoneRepo()
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()

	emonds := zones.Insert(repo.Zone{
		ExternalID: "emonds_field", Name: "Emond's Field",
		Builder: "jrags", MinLevel: 1, MaxLevel: 5,
		ResetIntervalS: 900, ResetMode: repo.ZoneResetEmpty,
		Climate: "temperate",
	})
	twoRivers := zones.Insert(repo.Zone{
		ExternalID: "two_rivers", Name: "Two Rivers",
		Builder: "jrags", MinLevel: 1, MaxLevel: 5,
		ResetIntervalS: 1200, ResetMode: repo.ZoneResetEmpty,
	})
	winespring := zones.Insert(repo.Zone{
		ExternalID: "emonds_field_winespring_inn", Name: "Winespring Inn",
		Builder: "jrags", MinLevel: 1, MaxLevel: 5,
		ResetIntervalS: 900, ResetMode: repo.ZoneResetEmpty,
	})

	// Pin the green to id=1 so it doubles as the global starter.
	rooms.Insert(repo.Room{
		ID: 1, ExternalID: "tr.emonds_field.green", Name: "The Green",
		ZoneID: emonds.ID, Sector: repo.SectorCity,
		Flags: repo.RoomFlags{Peaceful: true},
	})
	rooms.Insert(repo.Room{
		ID: 2, ExternalID: "tr.emonds_field.inn_yard", Name: "Inn Yard",
		ZoneID: emonds.ID, Sector: repo.SectorCity,
	})
	rooms.Insert(repo.Room{
		ID: 3, ExternalID: "tr.emonds_field.south_road", Name: "South Road",
		ZoneID: emonds.ID, Sector: repo.SectorField,
	})
	rooms.Insert(repo.Room{
		ID: 4, ExternalID: "tr.emonds_field.guest_a", Name: "Guest A",
		ZoneID: emonds.ID, Sector: repo.SectorCity,
	})
	rooms.Insert(repo.Room{
		ID: 5, ExternalID: "tr.emonds_field.dark_cellar", Name: "Dark Cellar",
		ZoneID: emonds.ID, Sector: repo.SectorUnderground,
		Flags: repo.RoomFlags{Dark: true, Silent: true},
	})
	rooms.Insert(repo.Room{
		ID: 6, ExternalID: "tr.emonds_field.south_lane", Name: "South Lane",
		ZoneID: emonds.ID, Sector: repo.SectorCity,
	})
	// Off-zone neighbours.
	rooms.Insert(repo.Room{
		ID: 7, ExternalID: "tr.north_road.central", Name: "North Road",
		ZoneID: twoRivers.ID, Sector: repo.SectorField,
	})
	rooms.Insert(repo.Room{
		ID: 8, ExternalID: "tr.emonds_field.winespring_inn.common", Name: "Inn Common",
		ZoneID: winespring.ID, Sector: repo.SectorCity,
	})

	// Exits — green at the centre, inn east, south_road south,
	// south_lane sw, north_road north (off-zone), inn_common east of
	// inn_yard (off-zone), guest_a up from inn_yard.
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirEast})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 1, Direction: repo.DirWest})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 3, Direction: repo.DirSouth})
	exits.Insert(repo.Exit{FromRoomID: 3, ToRoomID: 1, Direction: repo.DirNorth})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 6, Direction: repo.DirSouthwest})
	exits.Insert(repo.Exit{FromRoomID: 6, ToRoomID: 1, Direction: repo.DirNortheast})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 7, Direction: repo.DirNorth})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 8, Direction: repo.DirEast}) // off-zone into winespring_inn

	return zoneMapFix{
		rooms:  rooms,
		exits:  exits,
		zones:  zones,
		emonds: emonds,
	}
}

func runZoneMapCmd(t *testing.T, fix zoneMapFix, currentRoomID int64, raw string) string {
	t.Helper()
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	s.CurrentRoomID = currentRoomID
	cmd := NewZoneMap(fix.rooms, fix.exits, fix.zones)
	args, err := telnet.Tokenize(raw)
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	c := &telnet.Context{
		Ctx: context.Background(), Session: s, Name: "zonemap",
		Args: args, Raw: raw,
	}
	if err := cmd.Run(c); err != nil {
		t.Fatalf("zonemap.Run: %v", err)
	}
	return conn.String()
}

func TestZoneMap_RendersInZoneAndOffZone(t *testing.T) {
	fix := newZoneMapFix(t)
	out := runZoneMapCmd(t, fix, 1, "")
	// In-zone city cell: the green is the seed marker; inn_yard etc.
	// render as `[C]`.
	if !strings.Contains(out, "[C]") {
		t.Errorf("expected in-zone [C] cells; got:\n%s", out)
	}
	// Off-zone neighbour: inn_common renders as `(C)`.
	if !strings.Contains(out, "(C)") {
		t.Errorf("expected off-zone (C) cell for inn_common; got:\n%s", out)
	}
	// Off-zone field neighbour: north_road is (f).
	if !strings.Contains(out, "(f)") {
		t.Errorf("expected off-zone (f) cell for north_road; got:\n%s", out)
	}
	// Seed marker.
	if !strings.Contains(out, "[*]") {
		t.Errorf("expected [*] for seed; got:\n%s", out)
	}
}

func TestZoneMap_SectorCountsFooter(t *testing.T) {
	fix := newZoneMapFix(t)
	out := runZoneMapCmd(t, fix, 1, "")
	if !strings.Contains(out, "Sectors:") {
		t.Errorf("missing sector counts line; got:\n%s", out)
	}
	// Emond's Field has 4 city rooms reachable from the green (green,
	// inn_yard, south_lane, guest_a) plus 1 field (south_road) plus
	// 1 underground (dark_cellar) — but dark_cellar is unreachable
	// so it should NOT appear in the sector counts.
	if !strings.Contains(out, "city:") {
		t.Errorf("missing city sector count; got:\n%s", out)
	}
	if strings.Contains(out, "underground:") {
		t.Errorf("dark_cellar is unreachable; underground should not appear; got:\n%s", out)
	}
}

func TestZoneMap_CrossZoneExitsFooter(t *testing.T) {
	fix := newZoneMapFix(t)
	out := runZoneMapCmd(t, fix, 1, "")
	if !strings.Contains(out, "Cross-zone exits:") {
		t.Errorf("missing cross-zone exits section; got:\n%s", out)
	}
	if !strings.Contains(out, "two_rivers") {
		t.Errorf("expected two_rivers as a cross-zone target; got:\n%s", out)
	}
	if !strings.Contains(out, "emonds_field_winespring_inn") {
		t.Errorf("expected winespring_inn as a cross-zone target; got:\n%s", out)
	}
}

func TestZoneMap_VerticalExitsFooter(t *testing.T) {
	fix := newZoneMapFix(t)
	// Wire a u/d exit from inn_yard (id=2) to guest_a (id=4).
	fix.exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 4, Direction: repo.DirUp})
	fix.exits.Insert(repo.Exit{FromRoomID: 4, ToRoomID: 2, Direction: repo.DirDown})
	out := runZoneMapCmd(t, fix, 1, "")
	if !strings.Contains(out, "Vertical exits:") {
		t.Errorf("missing vertical exits section; got:\n%s", out)
	}
	if !strings.Contains(out, "inn_yard") || !strings.Contains(out, "u → ") {
		t.Errorf("expected inn_yard u → guest_a; got:\n%s", out)
	}
}

func TestZoneMap_FlaggedRoomsFooter(t *testing.T) {
	fix := newZoneMapFix(t)
	// Make dark_cellar reachable so the BFS includes it; then we
	// can verify dark + silent appear in the footer.
	fix.exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 5, Direction: repo.DirDown})
	fix.exits.Insert(repo.Exit{FromRoomID: 5, ToRoomID: 1, Direction: repo.DirUp})
	out := runZoneMapCmd(t, fix, 1, "")
	if !strings.Contains(out, "Flagged rooms:") {
		t.Errorf("missing flagged rooms section; got:\n%s", out)
	}
	if !strings.Contains(out, "peaceful:") {
		t.Errorf("missing peaceful flag (the green is peaceful); got:\n%s", out)
	}
	if !strings.Contains(out, "dark:") {
		t.Errorf("missing dark flag (dark_cellar); got:\n%s", out)
	}
	if !strings.Contains(out, "silent:") {
		t.Errorf("missing silent flag (dark_cellar); got:\n%s", out)
	}
}

func TestZoneMap_NoSuchZone(t *testing.T) {
	fix := newZoneMapFix(t)
	out := runZoneMapCmd(t, fix, 1, "no_such_zone")
	if !strings.Contains(out, "No such zone") {
		t.Errorf("expected no-such-zone error; got:\n%s", out)
	}
}

func TestZoneMap_DepthOverride_ClampsToMax(t *testing.T) {
	fix := newZoneMapFix(t)
	// `depth=99` is way over zoneMapMaxDepth (12); should silently
	// clamp without erroring. The output should still render.
	out := runZoneMapCmd(t, fix, 1, "depth=99")
	if !strings.Contains(out, "Zone:") {
		t.Errorf("expected rendered zone header; got:\n%s", out)
	}
}

func TestZoneMap_DepthOverride_RejectsZero(t *testing.T) {
	fix := newZoneMapFix(t)
	out := runZoneMapCmd(t, fix, 1, "depth=0")
	if !strings.Contains(out, "depth must be a positive integer") {
		t.Errorf("expected depth-reject error; got:\n%s", out)
	}
}

func TestZoneMap_DepthOverride_RejectsNonNumeric(t *testing.T) {
	fix := newZoneMapFix(t)
	out := runZoneMapCmd(t, fix, 1, "depth=foo")
	if !strings.Contains(out, "depth must be a positive integer") {
		t.Errorf("expected depth-reject error on non-numeric input; got:\n%s", out)
	}
}

func TestZoneMap_NoArgs_UsesCurrentRoom(t *testing.T) {
	fix := newZoneMapFix(t)
	out := runZoneMapCmd(t, fix, 2, "") // start from inn_yard, not green
	if !strings.Contains(out, "Zone:") {
		t.Errorf("expected zone header; got:\n%s", out)
	}
	// inn_yard is in emonds_field, so the rendered zone is
	// emonds_field regardless of which room is the seed.
	if !strings.Contains(out, "emonds_field") {
		t.Errorf("expected emonds_field as resolved zone; got:\n%s", out)
	}
}

func TestZoneMap_BareInvocation_NoCurrentRoom(t *testing.T) {
	fix := newZoneMapFix(t)
	out := runZoneMapCmd(t, fix, 0, "")
	if !strings.Contains(out, "nowhere in particular") {
		t.Errorf("expected 'nowhere in particular' error; got:\n%s", out)
	}
}

func TestZoneMap_ExplicitZone_PicksLowestIDSeed(t *testing.T) {
	fix := newZoneMapFix(t)
	// Calling `zonemap emonds_field` from CurrentRoomID=0 should
	// still work because the seed comes from the named zone.
	out := runZoneMapCmd(t, fix, 0, "emonds_field")
	if !strings.Contains(out, "Zone:") {
		t.Errorf("expected zone header even with no current room; got:\n%s", out)
	}
}
