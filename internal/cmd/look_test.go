package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/testhelper"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// bufConn / newBufConn / bufSession alias the shared helpers in
// internal/testhelper. Existing tests across this package refer to
// these unexported names; the alias preserves the call sites while
// keeping the implementation in one place.
type bufConn = testhelper.BufConn

func newBufConn() *bufConn { return testhelper.NewBufConn() }

func bufSession(t *testing.T) (*telnet.Session, *bufConn) {
	t.Helper()
	return testhelper.BufSession(t)
}

// noonClock returns a frozen-noon Clock for tests that need to pass a
// non-nil *world.Clock but don't care about the day/night cycle. ticks
// is pinned to 675 (mid-day quarter) so EffectiveLight returns the
// stored baseline for outdoor rooms.
func noonClock(t *testing.T) *world.Clock {
	t.Helper()
	frozen := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	return world.NewClock(675, world.WithNow(func() time.Time { return frozen }))
}

func seedWorld(t *testing.T) (*repo.MemoryRoomRepo, *repo.MemoryExitRepo, *repo.MemoryItemRepo, *repo.MemoryMobInstanceRepo) {
	t.Helper()
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	rooms.Insert(repo.Room{ID: 1, Name: "Town Plaza", LongDesc: "Cobblestones radiate outward."})
	rooms.Insert(repo.Room{ID: 2, Name: "North Road", LongDesc: "A quieter road."})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 1, Direction: repo.DirSouth})
	items.Insert(repo.Item{Name: "a small pebble", RoomID: 1})
	if _, err := mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: 1,
		Core:       creature.Core{Name: "a town crier", CurrentRoomID: 1},
	}); err != nil {
		t.Fatalf("seed mob: %v", err)
	}
	return rooms, exits, items, mobs
}

func TestLook_RendersRoomWithExitsItemsAndMobs(t *testing.T) {
	rooms, exits, items, mobs := seedWorld(t)
	s, conn := bufSession(t)
	s.CurrentRoomID = 1

	if err := RenderRoom(context.Background(), s, rooms, exits, items, mobs, noonClock(t)); err != nil {
		t.Fatalf("RenderRoom: %v", err)
	}

	got := conn.String()
	// SGR escapes sit between the label and the value, so we check
	// each token independently rather than asserting on the
	// concatenated form.
	for _, want := range []string{
		"Town Plaza",
		"Cobblestones radiate outward.",
		"Exits:", "north",
		"You see:", "a small pebble",
		"Also here:", "a town crier",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q.\nGot:\n%s", want, got)
		}
	}
}

func TestLook_OmitsEmptySubsections(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 7, Name: "Empty Hall", LongDesc: "A bare stone hall."})
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobInstanceRepo()

	s, conn := bufSession(t)
	s.CurrentRoomID = 7

	if err := RenderRoom(context.Background(), s, rooms, exits, items, mobs, noonClock(t)); err != nil {
		t.Fatalf("RenderRoom: %v", err)
	}
	got := conn.String()
	if !strings.Contains(got, "Empty Hall") {
		t.Fatalf("missing room name; got %q", got)
	}
	// SGR escapes between "Exits:" and "none", so check tokens.
	if !strings.Contains(got, "Exits:") || !strings.Contains(got, "none") {
		t.Fatalf("expected 'Exits:' + 'none' in empty hall; got %q", got)
	}
	if strings.Contains(got, "You see:") {
		t.Fatalf("expected no 'You see:' line; got %q", got)
	}
	if strings.Contains(got, "Also here:") {
		t.Fatalf("expected no 'Also here:' line; got %q", got)
	}
}

func TestLook_MissingRoomMessage(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobInstanceRepo()

	s, conn := bufSession(t)
	s.CurrentRoomID = 999

	if err := RenderRoom(context.Background(), s, rooms, exits, items, mobs, noonClock(t)); err != nil {
		t.Fatalf("RenderRoom: %v", err)
	}
	got := conn.String()
	if !strings.Contains(got, "gone missing") {
		t.Fatalf("expected missing-room message; got %q", got)
	}
}

func TestLook_DarkRoomHidesContents(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	rooms.Insert(repo.Room{
		ID: 5, Name: "Pitch Cellar", LongDesc: "The walls drip.",
		Flags: repo.RoomFlags{Dark: true}, LightLevel: 0,
	})
	items.Insert(repo.Item{Name: "a glittering coin", RoomID: 5})

	s, conn := bufSession(t)
	s.CurrentRoomID = 5
	if err := RenderRoom(context.Background(), s, rooms, exits, items, mobs, noonClock(t)); err != nil {
		t.Fatalf("RenderRoom: %v", err)
	}
	got := conn.String()
	if !strings.Contains(got, "pitch black") {
		t.Fatalf("expected pitch-black message; got %q", got)
	}
	for _, leak := range []string{"Pitch Cellar", "drip", "glittering coin"} {
		if strings.Contains(got, leak) {
			t.Fatalf("dark room leaked %q in output: %q", leak, got)
		}
	}
}

func TestLook_HidesHiddenExitsAndAnnotatesDoors(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	rooms.Insert(repo.Room{ID: 1, Name: "Foyer", LongDesc: "A polished foyer."})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth,
		Flags: repo.ExitFlags{Closed: true, Locked: true}})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 3, Direction: repo.DirEast,
		Flags: repo.ExitFlags{Closed: true}})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 4, Direction: repo.DirSouth,
		Flags: repo.ExitFlags{Hidden: true}})

	s, conn := bufSession(t)
	s.CurrentRoomID = 1
	if err := RenderRoom(context.Background(), s, rooms, exits, items, mobs, noonClock(t)); err != nil {
		t.Fatalf("RenderRoom: %v", err)
	}
	got := conn.String()
	if !strings.Contains(got, "north (locked)") {
		t.Errorf("missing locked annotation; got %q", got)
	}
	if !strings.Contains(got, "east (closed)") {
		t.Errorf("missing closed annotation; got %q", got)
	}
	if strings.Contains(got, "south") {
		t.Errorf("hidden exit leaked: %q", got)
	}
}

func TestLook_KeywordHitRendersExtraDesc(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{
		ID: 1, Name: "Plaza", LongDesc: "Cobblestones.",
		ExtraDescs: map[string]string{
			"fountain": "A marble fountain spills clear water into a basin.",
		},
	})
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobInstanceRepo()

	look := NewLook(rooms, exits, items, mobs, noonClock(t))
	s, conn := bufSession(t)
	s.CurrentRoomID = 1
	s.AuthLevel = telnet.AuthPlayer

	ctx := &telnet.Context{Ctx: context.Background(), Session: s, Name: "look", Args: []string{"Fountain"}}
	if err := look.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(conn.String(), "marble fountain") {
		t.Fatalf("expected fountain desc; got %q", conn.String())
	}
}

func TestLook_KeywordMissFallsThrough(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, Name: "Plaza", LongDesc: "Cobblestones."})
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobInstanceRepo()

	look := NewLook(rooms, exits, items, mobs, noonClock(t))
	s, conn := bufSession(t)
	s.CurrentRoomID = 1
	s.AuthLevel = telnet.AuthPlayer

	ctx := &telnet.Context{Ctx: context.Background(), Session: s, Name: "look", Args: []string{"dragon"}}
	if err := look.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(conn.String(), "nothing special") {
		t.Fatalf("expected miss message; got %q", conn.String())
	}
}

// TestLook_EmitsANSIEscapes confirms the cfmt path actually produces
// SGR sequences — guards against a future regression where someone
// switches WriteString back to WriteRaw and silently strips colour.
func TestLook_EmitsANSIEscapes(t *testing.T) {
	rooms, exits, items, mobs := seedWorld(t)
	s, conn := bufSession(t)
	s.CurrentRoomID = 1
	if err := RenderRoom(context.Background(), s, rooms, exits, items, mobs, noonClock(t)); err != nil {
		t.Fatalf("RenderRoom: %v", err)
	}
	if !strings.Contains(conn.String(), "\x1b[") {
		t.Fatalf("output has no ANSI escapes; got %q", conn.String())
	}
}

// fixedDayClock pins the day clock to a specific tick so tests can
// exercise the night/dusk/dawn paths deterministically.
func fixedDayClock(t *testing.T, ticks int64) *world.Clock {
	t.Helper()
	frozen := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	return world.NewClock(ticks, world.WithNow(func() time.Time { return frozen }))
}

func TestLook_OutdoorRoomAtMidnightRendersPitchBlack(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{
		ID: 1, Name: "Open Field", LongDesc: "Tall grass under a wide sky.",
		Sector:     repo.SectorField,
		LightLevel: 100,
	})
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobInstanceRepo()

	s, conn := bufSession(t)
	s.CurrentRoomID = 1
	clock := fixedDayClock(t, 1500) // night quarter

	if err := RenderRoom(context.Background(), s, rooms, exits, items, mobs, clock); err != nil {
		t.Fatalf("RenderRoom: %v", err)
	}
	if !strings.Contains(conn.String(), "pitch black") {
		t.Fatalf("expected pitch-black at night; got %q", conn.String())
	}
	if strings.Contains(conn.String(), "Open Field") {
		t.Fatalf("room title should not render at night; got %q", conn.String())
	}
}

func TestLook_OutdoorRoomAtNoonRendersNormally(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{
		ID: 1, Name: "Open Field", LongDesc: "Tall grass under a wide sky.",
		Sector:     repo.SectorField,
		LightLevel: 100,
	})
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobInstanceRepo()

	s, conn := bufSession(t)
	s.CurrentRoomID = 1
	clock := fixedDayClock(t, 675) // day quarter

	if err := RenderRoom(context.Background(), s, rooms, exits, items, mobs, clock); err != nil {
		t.Fatalf("RenderRoom: %v", err)
	}
	if !strings.Contains(conn.String(), "Open Field") {
		t.Fatalf("expected room title; got %q", conn.String())
	}
}

func TestLook_IndoorRoomIgnoresCycle(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{
		ID: 1, Name: "Inn Common Room", LongDesc: "A warm hearth.",
		Sector:     repo.SectorCity,
		Flags:      repo.RoomFlags{Indoors: true},
		LightLevel: 80,
	})
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobInstanceRepo()

	s, conn := bufSession(t)
	s.CurrentRoomID = 1
	clock := fixedDayClock(t, 1500) // night, but indoors should ignore

	if err := RenderRoom(context.Background(), s, rooms, exits, items, mobs, clock); err != nil {
		t.Fatalf("RenderRoom: %v", err)
	}
	if strings.Contains(conn.String(), "pitch black") {
		t.Fatalf("indoor room should not go pitch-black at night; got %q", conn.String())
	}
}

func TestLook_DarkFlagOverridesCycle(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{
		ID: 1, Name: "Sealed Vault",
		Sector:     repo.SectorUnderground,
		Flags:      repo.RoomFlags{Dark: true},
		LightLevel: 100, // ignored by Dark
	})
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobInstanceRepo()

	s, conn := bufSession(t)
	s.CurrentRoomID = 1
	clock := fixedDayClock(t, 675) // would be full daylight outdoors

	if err := RenderRoom(context.Background(), s, rooms, exits, items, mobs, clock); err != nil {
		t.Fatalf("RenderRoom: %v", err)
	}
	if !strings.Contains(conn.String(), "pitch black") {
		t.Fatalf("Dark-flagged room should always be pitch black; got %q", conn.String())
	}
}
