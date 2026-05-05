package world

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/testhelper"
	"github.com/Jasrags/WheelMUD/telnet"
)

// bufConn / newBufConn / bufSession alias the shared helpers in
// internal/testhelper. See internal/testhelper/bufconn.go.
type bufConn = testhelper.BufConn

func newBufConn() *bufConn { return testhelper.NewBufConn() }

func bufSession(t *testing.T) (*telnet.Session, *bufConn) {
	t.Helper()
	return testhelper.BufSession(t)
}

// --- helpers --------------------------------------------------------------

// frozenClock returns a Clock pinned at the given tick using the default
// 1800-tick day. now closures over a settable pointer so tests can advance
// the clock without rebuilding it.
func frozenClock(ticks int64) (*Clock, *time.Time) {
	t := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	tp := &t
	c := NewClock(ticks, WithNow(func() time.Time { return *tp }))
	return c, tp
}

// --- transitionFor table --------------------------------------------------

func TestTransitionFor(t *testing.T) {
	tests := []struct {
		prev, curr Phase
		wantT      Transition
		wantOK     bool
	}{
		{PhaseNight, PhaseDawn, TransitionDawn, true},
		{PhaseDawn, PhaseDay, TransitionSunrise, true},
		{PhaseDay, PhaseDusk, TransitionDusk, true},
		{PhaseDusk, PhaseNight, TransitionNightfall, true},

		// No-ops.
		{PhaseDawn, PhaseDawn, 0, false},
		{PhaseDay, PhaseDay, 0, false},
		{PhaseNight, PhaseNight, 0, false},

		// Reverse / non-adjacent (e.g. clock rebase).
		{PhaseDay, PhaseDawn, 0, false},
		{PhaseNight, PhaseDay, 0, false},
		{PhaseDawn, PhaseDusk, 0, false},
		{PhaseDusk, PhaseDawn, 0, false},
	}
	for _, tc := range tests {
		gotT, gotOK := transitionFor(tc.prev, tc.curr)
		if gotOK != tc.wantOK {
			t.Errorf("transitionFor(%v,%v) ok = %v, want %v", tc.prev, tc.curr, gotOK, tc.wantOK)
		}
		if gotOK && gotT != tc.wantT {
			t.Errorf("transitionFor(%v,%v) = %v, want %v", tc.prev, tc.curr, gotT, tc.wantT)
		}
	}
}

// --- sectorAmbients shape -------------------------------------------------

func TestSectorAmbientsCompleteAndNonEmpty(t *testing.T) {
	want := []repo.Sector{
		repo.SectorCity, repo.SectorForest, repo.SectorField,
		repo.SectorHills, repo.SectorMountain, repo.SectorDesert,
		repo.SectorWater, repo.SectorAir,
		// Migration 0025 WoT terrain extensions.
		repo.SectorSwamp, repo.SectorWaste,
		repo.SectorStedding, repo.SectorBlight,
	}
	for _, s := range want {
		lines, ok := sectorAmbients[s]
		if !ok {
			t.Fatalf("sector %q missing from sectorAmbients", s)
		}
		for i, line := range lines {
			if line == "" {
				t.Errorf("sector %q transition %d has empty line", s, i)
			}
		}
	}
	// Underground and underwater are intentionally absent — the gate
	// filters them before lookup.
	for _, s := range []repo.Sector{repo.SectorUnderground, repo.SectorUnderwater} {
		if _, ok := sectorAmbients[s]; ok {
			t.Errorf("sector %q should not be in sectorAmbients (filtered upstream)", s)
		}
	}
}

func TestAmbientLine_EmptySectorDefaultsToCity(t *testing.T) {
	got := ambientLine("", TransitionSunrise)
	want := sectorAmbients[repo.SectorCity][TransitionSunrise]
	if got != want {
		t.Errorf("empty sector sunrise = %q, want city sunrise %q", got, want)
	}
}

// --- roomReceivesAmbient gate ---------------------------------------------

func TestRoomReceivesAmbient(t *testing.T) {
	cases := []struct {
		name string
		room repo.Room
		want bool
	}{
		{"forest outdoor", repo.Room{Sector: repo.SectorForest}, true},
		{"empty sector defaults open", repo.Room{Sector: ""}, true},
		{"silent muted", repo.Room{Sector: repo.SectorCity, Flags: repo.RoomFlags{Silent: true}}, false},
		{"dark muted", repo.Room{Sector: repo.SectorField, Flags: repo.RoomFlags{Dark: true}}, false},
		{"indoors muted", repo.Room{Sector: repo.SectorCity, Flags: repo.RoomFlags{Indoors: true}}, false},
		{"underground filtered", repo.Room{Sector: repo.SectorUnderground}, false},
		{"underwater filtered", repo.Room{Sector: repo.SectorUnderwater}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := roomReceivesAmbient(tc.room); got != tc.want {
				t.Errorf("roomReceivesAmbient = %v, want %v", got, tc.want)
			}
		})
	}
}

// --- end-to-end broadcast -------------------------------------------------

func TestPhaseAmbientWatcher_BroadcastsOnSunrise(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, ExternalID: "forest.path", Name: "Forest Path", Sector: repo.SectorForest})
	rooms.Insert(repo.Room{ID: 2, ExternalID: "temple", Name: "Silent Temple", Sector: repo.SectorCity, Flags: repo.RoomFlags{Silent: true}})
	rooms.Insert(repo.Room{ID: 3, ExternalID: "tavern", Name: "Tavern Common", Sector: repo.SectorCity, Flags: repo.RoomFlags{Indoors: true}})

	reg := session.NewRegistry()

	// In-forest player — should hear the forest sunrise line.
	forester, forestConn := bufSession(t)
	forester.CharacterID = 11
	forester.CharacterName = "Forester"
	forester.CurrentRoomID = 1
	reg.Bind(101, forester)

	// In-temple player — Silent room, should hear nothing.
	priest, priestConn := bufSession(t)
	priest.CharacterID = 12
	priest.CharacterName = "Priest"
	priest.CurrentRoomID = 2
	reg.Bind(102, priest)

	// In-tavern player — Indoors, should hear nothing.
	tavernGoer, tavernConn := bufSession(t)
	tavernGoer.CharacterID = 13
	tavernGoer.CharacterName = "Bard"
	tavernGoer.CurrentRoomID = 3
	reg.Bind(103, tavernGoer)

	// Pre-promotion player at the login prompt — no character / room.
	loginer, loginConn := bufSession(t)
	reg.Bind(104, loginer)

	// Construct watcher mid-dawn (tick=200 of 1800). lastPhase = dawn.
	clock, nowPtr := frozenClock(200)
	w := NewPhaseAmbientWatcher(clock, rooms, reg)

	// Advance the underlying time source by 251 ticks (251 s) to push
	// the clock from tick 200 (dawn) into tick 451 (day). The phase
	// transition is dawn → day, which fires TransitionSunrise.
	*nowPtr = nowPtr.Add(251 * time.Second)

	w.Tick(context.Background())

	wantForest := sectorAmbients[repo.SectorForest][TransitionSunrise]
	if !strings.Contains(forestConn.String(), wantForest) {
		t.Errorf("forester missing sunrise line.\n got: %q\nwant substring: %q", forestConn.String(), wantForest)
	}
	if priestConn.String() != "" {
		t.Errorf("silent-room priest should be silent; got %q", priestConn.String())
	}
	if tavernConn.String() != "" {
		t.Errorf("indoors tavern-goer should be silent; got %q", tavernConn.String())
	}
	if loginConn.String() != "" {
		t.Errorf("pre-promotion session should be silent; got %q", loginConn.String())
	}
}

func TestPhaseAmbientWatcher_NoFireOnNoOp(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, Sector: repo.SectorForest})
	reg := session.NewRegistry()
	s, conn := bufSession(t)
	s.CharacterID = 1
	s.CharacterName = "Alone"
	s.CurrentRoomID = 1
	reg.Bind(1, s)

	clock, _ := frozenClock(675) // mid-day
	w := NewPhaseAmbientWatcher(clock, rooms, reg)

	w.Tick(context.Background())
	w.Tick(context.Background())

	if conn.String() != "" {
		t.Errorf("no-op tick should not write; got %q", conn.String())
	}
}

func TestPhaseAmbientWatcher_BootSeedSuppressesSpuriousFire(t *testing.T) {
	// Seed at night (tick=1500). If the watcher mistakenly defaulted
	// lastPhase to PhaseDawn (zero value), the very first poll would
	// see "PhaseDawn → PhaseNight" and emit something. With the seed
	// in place, the first poll sees no change and stays silent.
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, Sector: repo.SectorField})
	reg := session.NewRegistry()
	s, conn := bufSession(t)
	s.CharacterID = 1
	s.CharacterName = "Watcher"
	s.CurrentRoomID = 1
	reg.Bind(1, s)

	clock, _ := frozenClock(1500) // night
	w := NewPhaseAmbientWatcher(clock, rooms, reg)

	w.Tick(context.Background())

	if conn.String() != "" {
		t.Errorf("first tick after night-boot should be silent; got %q", conn.String())
	}
}

func TestPhaseAmbientWatcher_NonAdjacentJumpDoesNotBroadcast(t *testing.T) {
	// Jump dawn → night in a single tick (e.g. a future `time set`
	// rebase). transitionFor returns false; nothing should write,
	// and lastPhase resyncs so the next clean transition fires
	// normally.
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, Sector: repo.SectorField})
	reg := session.NewRegistry()
	s, conn := bufSession(t)
	s.CharacterID = 1
	s.CharacterName = "Jumper"
	s.CurrentRoomID = 1
	reg.Bind(1, s)

	clock, nowPtr := frozenClock(200) // dawn
	w := NewPhaseAmbientWatcher(clock, rooms, reg)

	// Jump forward 1300s → tick 1500 (night). dawn → night is
	// non-adjacent.
	*nowPtr = nowPtr.Add(1300 * time.Second)
	w.Tick(context.Background())

	if conn.String() != "" {
		t.Errorf("non-adjacent jump should not broadcast; got %q", conn.String())
	}
}
