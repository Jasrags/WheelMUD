package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// clockAt builds a frozen-time Clock pinned at the given tick. Default
// dayDuration (30m) and tickInterval (1s) → 1800 ticks/day, q=450.
func clockAt(ticks int64) *world.Clock {
	frozen := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	return world.NewClock(ticks, world.WithNow(func() time.Time { return frozen }))
}

func runTime(t *testing.T, clock *world.Clock) string {
	t.Helper()
	c := NewTime(clock)
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	ctx := &telnet.Context{Ctx: context.Background(), Session: s, Name: "time"}
	if err := c.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return conn.String()
}

func TestTime_PhasesAndProgress(t *testing.T) {
	tests := []struct {
		name      string
		ticks     int64
		wantPhase string
		wantNext  string
		// approximate progress percentage we expect (q=450, integer math).
		wantProgress string
	}{
		{"dawn-start", 0, "dawn", "day", "0%"},
		{"dawn-mid", 225, "dawn", "day", "50%"},
		{"day-start", 450, "day", "dusk", "0%"},
		{"day-mid", 675, "day", "dusk", "50%"},
		{"dusk-quarter", 900 + 112, "dusk", "night", "24%"},
		{"night-near-end", 1750, "night", "dawn", "88%"},
		// second cycle: ticks = tpd + 100 → into-day = 100, dawn at 22%.
		{"second-cycle", 1800 + 100, "dawn", "day", "22%"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := runTime(t, clockAt(tc.ticks))
			if !strings.Contains(out, tc.wantPhase) {
				t.Errorf("expected phase %q in output:\n%s", tc.wantPhase, out)
			}
			if !strings.Contains(out, "until") || !strings.Contains(out, tc.wantNext) {
				t.Errorf("expected %q as next phase in output:\n%s", tc.wantNext, out)
			}
			if !strings.Contains(out, tc.wantProgress) {
				t.Errorf("expected progress %q in output:\n%s", tc.wantProgress, out)
			}
		})
	}
}

func TestTime_ReportsCycleAndDayLength(t *testing.T) {
	out := runTime(t, clockAt(675))
	for _, want := range []string{
		"Day clock:",
		"Ticks:",
		"675",
		"cycle 0",
		"tick 675 / 1800",
		"Phase:",
		"day",
		"Tick interval:",
		"1s",
		"Day length:",
		"30m0s",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestTime_NilClock(t *testing.T) {
	c := NewTime(nil)
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	ctx := &telnet.Context{Ctx: context.Background(), Session: s, Name: "time"}
	if err := c.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(conn.String(), "not configured") {
		t.Fatalf("expected not-configured message, got %q", conn.String())
	}
}

func TestTime_NonDivisibleDayLength(t *testing.T) {
	// 17s day, 1s tick → tpd=17, q=4. Quarters: dawn 0-3, day 4-7,
	// dusk 8-11, night 12-16 (length 5, not 4). Pin ticks=15 → night,
	// intoPhase=3, phaseLen=5, untilNext=2, progress=60%.
	frozen := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	clock := world.NewClock(15,
		world.WithNow(func() time.Time { return frozen }),
		world.WithDayDuration(17*time.Second),
		world.WithTickInterval(1*time.Second),
	)
	out := runTime(t, clock)
	if !strings.Contains(out, "night") {
		t.Errorf("expected night phase in output:\n%s", out)
	}
	// Night length is 5 ticks (tpd-3q), not q=4. progress = 3*100/5 = 60.
	if !strings.Contains(out, "60%") {
		t.Errorf("expected progress 60%% (night phaseLen=5):\n%s", out)
	}
	if !strings.Contains(out, "2 ticks") {
		t.Errorf("expected 2 ticks until dawn:\n%s", out)
	}
}

func TestTime_DegenerateConfig(t *testing.T) {
	// dayDuration < tickInterval → ticksPerDay = 0. Command should
	// still report something useful instead of dividing by zero.
	frozen := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	clock := world.NewClock(0,
		world.WithNow(func() time.Time { return frozen }),
		world.WithDayDuration(500*time.Millisecond),
		world.WithTickInterval(1*time.Second),
	)
	out := runTime(t, clock)
	if !strings.Contains(out, "ticksPerDay <= 0") {
		t.Fatalf("expected misconfiguration warning, got %q", out)
	}
}
