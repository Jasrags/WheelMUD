package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// zonesFix wires a memory ZoneRepo + RoomRepo and seeds them so the
// `zones` command has something to enumerate.
type zonesFix struct {
	zones *repo.MemoryZoneRepo
	rooms *repo.MemoryRoomRepo
}

func newZonesFix(t *testing.T) zonesFix {
	t.Helper()
	z := repo.NewMemoryZoneRepo()
	r := repo.NewMemoryRoomRepo()

	emonds := z.Insert(repo.Zone{
		ExternalID: "emonds_field", Name: "Emond's Field",
		Builder:  "jrags",
		MinLevel: 1, MaxLevel: 5,
		ResetIntervalS: 900,
		ResetMode:      repo.ZoneResetEmpty,
		Climate:        "temperate",
		Ambient: []string{
			"The smell of fresh bread drifts across the green.",
			"A pair of doves break cover from the inn roof.",
		},
	})
	z.Insert(repo.Zone{
		ExternalID: "watch_hill", Name: "Watch Hill",
		MinLevel: 1, MaxLevel: 5,
		ResetIntervalS: 600, ResetMode: repo.ZoneResetEmpty,
	})

	// Three rooms in emonds_field, none elsewhere.
	for i, ext := range []string{"green", "winespring", "inn_yard"} {
		r.Insert(repo.Room{
			ID: int64(10 + i), ExternalID: "tr.emonds_field." + ext,
			ZoneID: emonds.ID, Name: ext,
		})
	}
	return zonesFix{zones: z, rooms: r}
}

func runZonesCmd(t *testing.T, fix zonesFix, args ...string) string {
	t.Helper()
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	cmd := NewZones(fix.zones, fix.rooms)
	c := &telnet.Context{Ctx: context.Background(), Session: s, Name: "zones", Args: args}
	if err := cmd.Run(c); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return conn.String()
}

func TestZones_ListShowsAllZonesSortedByExternalID(t *testing.T) {
	fix := newZonesFix(t)
	got := runZonesCmd(t, fix)
	// Sorted: emonds_field before watch_hill.
	if i := strings.Index(got, "emonds_field"); i < 0 {
		t.Fatalf("missing emonds_field in output:\n%s", got)
	}
	if i := strings.Index(got, "watch_hill"); i < 0 {
		t.Fatalf("missing watch_hill in output:\n%s", got)
	}
	if strings.Index(got, "emonds_field") >= strings.Index(got, "watch_hill") {
		t.Errorf("not sorted by external_id:\n%s", got)
	}
}

func TestZones_ListAcceptsExplicitListSubcommand(t *testing.T) {
	fix := newZonesFix(t)
	got := runZonesCmd(t, fix, "list")
	if !strings.Contains(got, "emonds_field") {
		t.Fatalf("missing emonds_field in `zones list` output:\n%s", got)
	}
}

func TestZones_ShowRendersAllFields(t *testing.T) {
	fix := newZonesFix(t)
	got := runZonesCmd(t, fix, "show", "emonds_field")
	for _, want := range []string{
		"emonds_field", "Emond's Field",
		"jrags",
		"1-5",
		"900s", "empty",
		"temperate",
		"Rooms:", "3",
		"Ambient:",
		"fresh bread",
		"doves",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in `zones show` output:\n%s", want, got)
		}
	}
}

func TestZones_ShowAcceptsBareExternalID(t *testing.T) {
	fix := newZonesFix(t)
	got := runZonesCmd(t, fix, "emonds_field")
	if !strings.Contains(got, "Emond's Field") {
		t.Fatalf("bare-id form did not show zone:\n%s", got)
	}
}

func TestZones_ShowMissingReturnsErrorMessage(t *testing.T) {
	fix := newZonesFix(t)
	got := runZonesCmd(t, fix, "show", "no_such_zone")
	if !strings.Contains(got, "No such zone") {
		t.Fatalf("expected not-found message; got:\n%s", got)
	}
}

func TestZones_ShowDefangsCfmtInjection(t *testing.T) {
	fix := newZonesFix(t)
	// A hostile id closes the styled error tag and tries to open a
	// fresh style. After defanging, neither token survives intact.
	got := runZonesCmd(t, fix, "show", "}}::white{{evil")
	if strings.Contains(got, "}}::white{{") {
		t.Fatalf("cfmt injection survived defang:\n%s", got)
	}
	if !strings.Contains(got, "No such zone") {
		t.Fatalf("expected not-found message; got:\n%s", got)
	}
}

func TestZones_ShowMissingExternalIDArg(t *testing.T) {
	fix := newZonesFix(t)
	got := runZonesCmd(t, fix, "show")
	if !strings.Contains(got, "Usage") {
		t.Fatalf("expected usage line on missing arg; got:\n%s", got)
	}
}

func TestZones_AuthAdminGate(t *testing.T) {
	// A Guest-level session calling NewZones directly bypasses the
	// registry's Auth gate (the registry returns "Unknown command"
	// for under-privileged callers), so prove the Auth marker is set.
	cmd := NewZones(repo.NewMemoryZoneRepo(), repo.NewMemoryRoomRepo())
	if cmd.Auth != telnet.AuthAdmin {
		t.Errorf("Auth = %v, want AuthAdmin", cmd.Auth)
	}
}
