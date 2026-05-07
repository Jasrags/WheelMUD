package world

import (
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// fixedClock builds a Clock pinned to the given tick count. The
// production constructor anchors `start` at construction time; here
// we want a deterministic phase, so we set baseTicks directly and
// pin `now` to start so elapsed = 0.
func fixedClock(t *testing.T, ticks int64) *Clock {
	t.Helper()
	frozen := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	return NewClock(ticks, WithNow(func() time.Time { return frozen }))
}

func TestApplyCurve(t *testing.T) {
	const baseline int = 100
	const tpd int64 = 1800 // matches default day length at 1 Hz
	cases := []struct {
		name  string
		ticks int64
		want  int
	}{
		{"dawn_start", 0, 0},
		{"dawn_quarter", 112, 24}, // 100 * 112/450 = 24
		{"dawn_end", 449, 99},     // 100 * 449/450 = 99
		{"day_start", 450, 100},
		{"day_mid", 675, 100},
		{"day_end", 899, 100},
		{"dusk_start", 900, 100}, // 100 * (1350-900)/450 = 100
		{"dusk_mid", 1125, 50},   // 100 * (1350-1125)/450 = 50
		{"dusk_end", 1349, 0},    // 100 * (1350-1349)/450 = 0 (int trunc)
		{"night_start", 1350, 0},
		{"night_mid", 1575, 0},
		{"night_end", 1799, 0},
		{"wrap_to_dawn", 1800, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := applyCurve(baseline, tc.ticks, tpd)
			if got != tc.want {
				t.Fatalf("applyCurve(%d, %d) = %d, want %d", baseline, tc.ticks, got, tc.want)
			}
		})
	}
}

func TestApplyCurveZeroBaseline(t *testing.T) {
	if got := applyCurve(0, 675, 1800); got != 0 {
		t.Fatalf("zero baseline must stay zero across phases; got %d", got)
	}
}

func TestPhase(t *testing.T) {
	cases := []struct {
		ticks int64
		want  Phase
	}{
		{0, PhaseDawn},
		{449, PhaseDawn},
		{450, PhaseDay},
		{899, PhaseDay},
		{900, PhaseDusk},
		{1349, PhaseDusk},
		{1350, PhaseNight},
		{1799, PhaseNight},
		{1800, PhaseDawn}, // wrap
	}
	for _, tc := range cases {
		t.Run(tc.want.String(), func(t *testing.T) {
			c := fixedClock(t, tc.ticks)
			if got := c.Phase(); got != tc.want {
				t.Fatalf("Phase at ticks=%d = %v, want %v", tc.ticks, got, tc.want)
			}
		})
	}
}

func TestEffectiveLight_DarkOverride(t *testing.T) {
	c := fixedClock(t, 675) // noon
	room := repo.Room{
		LightLevel: 100,
		Sector:     repo.SectorCity,
		Flags:      repo.RoomFlags{Dark: true},
	}
	if got := c.EffectiveLight(room); got != 0 {
		t.Fatalf("Dark override at noon = %d, want 0", got)
	}
}

func TestEffectiveLight_IndoorIgnoresCycle(t *testing.T) {
	cases := []struct {
		name  string
		ticks int64
	}{
		{"midnight", 1500},
		{"noon", 675},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := fixedClock(t, tc.ticks)
			room := repo.Room{
				LightLevel: 80,
				Sector:     repo.SectorCity,
				Flags:      repo.RoomFlags{Indoors: true},
			}
			if got := c.EffectiveLight(room); got != 80 {
				t.Fatalf("indoor room at %s = %d, want 80 (baseline)", tc.name, got)
			}
		})
	}
}

func TestEffectiveLight_UndergroundIgnoresCycle(t *testing.T) {
	c := fixedClock(t, 1500) // night
	room := repo.Room{LightLevel: 50, Sector: repo.SectorUnderground}
	if got := c.EffectiveLight(room); got != 50 {
		t.Fatalf("underground at night = %d, want 50 (baseline)", got)
	}
}

func TestEffectiveLight_UnderwaterIgnoresCycle(t *testing.T) {
	c := fixedClock(t, 1500)
	room := repo.Room{LightLevel: 50, Sector: repo.SectorUnderwater}
	if got := c.EffectiveLight(room); got != 50 {
		t.Fatalf("underwater at night = %d, want 50 (baseline)", got)
	}
}

func TestEffectiveLight_OutdoorCycles(t *testing.T) {
	cases := []struct {
		name     string
		ticks    int64
		baseline int
		want     int
	}{
		{"noon_full", 675, 100, 100},
		{"midnight_dark", 1500, 100, 0},
		{"dusk_mid", 1125, 100, 50},
		{"dawn_mid", 225, 100, 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := fixedClock(t, tc.ticks)
			room := repo.Room{LightLevel: tc.baseline, Sector: repo.SectorForest}
			if got := c.EffectiveLight(room); got != tc.want {
				t.Fatalf("outdoor at %s = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

func TestTicksAdvanceWithWallClock(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	c := NewClock(100, WithNow(func() time.Time { return now }))
	if got := c.Ticks(); got != 100 {
		t.Fatalf("Ticks at start = %d, want 100", got)
	}
	now = now.Add(45 * time.Second)
	if got := c.Ticks(); got != 145 {
		t.Fatalf("Ticks after +45s = %d, want 145", got)
	}
}
