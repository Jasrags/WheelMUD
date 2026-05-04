package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewTime builds the day-clock inspector. Reports tick count, current
// phase, phase progress, and time until the next phase boundary so
// builders can sanity-check the §9 day/night cycle without scraping it
// out of `whereami` one room at a time. Admin-only — exposes process
// state, and the player-facing "peek the sky" verb is a separate scope.
func NewTime(clock *world.Clock) *telnet.Command {
	return &telnet.Command{
		Name: "time",
		Help: "Show the global day-clock: ticks, phase, and time until the next phase",
		Auth: telnet.AuthAdmin,
		Run: func(c *telnet.Context) error {
			if clock == nil {
				return c.Session.WriteString("{{Day clock is not configured.}}::red\r\n")
			}
			return c.Session.WriteString(formatTime(clock))
		},
	}
}

func formatTime(clock *world.Clock) string {
	tpd := clock.TicksPerDay()
	// Snapshot ticks once and derive phase from the snapshot. Calling
	// clock.Phase() separately re-reads the wall clock, so a tick that
	// crosses a phase boundary between the two reads can leave intoDay
	// in the *next* phase while phase still names the previous one,
	// producing a negative intoPhase and a >100% progress.
	ticks := clock.Ticks()
	interval := clock.TickInterval()

	var b strings.Builder
	b.WriteString("{{Day clock:}}::cyan|bold\r\n")

	if tpd <= 0 {
		// Degenerate config (dayDuration < tickInterval). Still report
		// what we have so the operator can see the misconfiguration.
		fmt.Fprintf(&b, "  {{Ticks:}}::yellow %d\r\n", ticks)
		b.WriteString("  {{(ticksPerDay <= 0; check dayDuration/tickInterval)}}::red\r\n")
		return b.String()
	}

	cycle := ticks / tpd
	intoDay := ticks % tpd
	q := tpd / 4
	phase := phaseAt(intoDay, q)
	phaseStart := int64(phase) * q
	// Night runs until tpd, not 4q (tpd may not divide by 4).
	phaseEnd := phaseStart + q
	if phase == world.PhaseNight {
		phaseEnd = tpd
	}
	phaseLen := phaseEnd - phaseStart
	intoPhase := intoDay - phaseStart
	untilNext := phaseEnd - intoDay
	progress := 0
	if phaseLen > 0 {
		progress = int(intoPhase * 100 / phaseLen)
	}

	next := nextPhase(phase)

	fmt.Fprintf(&b,
		"  {{Ticks:}}::yellow %d {{(cycle %d, tick %d / %d)}}::gray\r\n",
		ticks, cycle, intoDay, tpd,
	)
	fmt.Fprintf(&b,
		"  {{Phase:}}::yellow %s {{(%d/4)}}::gray\r\n",
		colorizePhase(phase), int(phase)+1,
	)
	fmt.Fprintf(&b,
		"  {{Progress:}}::yellow %d%% — %d ticks in, %d ticks (%s) until %s\r\n",
		progress, intoPhase, untilNext, humanDur(time.Duration(untilNext)*interval),
		colorizePhase(next),
	)
	fmt.Fprintf(&b,
		"  {{Tick interval:}}::yellow %s    {{Day length:}}::yellow %s\r\n",
		humanDur(interval), humanDur(time.Duration(tpd)*interval),
	)
	return b.String()
}

// phaseAt mirrors Clock.Phase's quarter math but operates on a caller-
// supplied tick snapshot, keeping the phase derivation consistent with
// the tick value used everywhere else in the rendered output.
func phaseAt(intoDay, q int64) world.Phase {
	switch {
	case intoDay < q:
		return world.PhaseDawn
	case intoDay < 2*q:
		return world.PhaseDay
	case intoDay < 3*q:
		return world.PhaseDusk
	default:
		return world.PhaseNight
	}
}

func nextPhase(p world.Phase) world.Phase {
	switch p {
	case world.PhaseDawn:
		return world.PhaseDay
	case world.PhaseDay:
		return world.PhaseDusk
	case world.PhaseDusk:
		return world.PhaseNight
	default:
		return world.PhaseDawn
	}
}

func colorizePhase(p world.Phase) string {
	switch p {
	case world.PhaseDawn:
		return "{{dawn}}::yellow"
	case world.PhaseDay:
		return "{{day}}::green"
	case world.PhaseDusk:
		return "{{dusk}}::magenta"
	case world.PhaseNight:
		return "{{night}}::blue"
	default:
		return p.String()
	}
}

// humanDur renders a duration as a compact "1m30s" / "45s" string.
// time.Duration.String() returns "1m30.000s" for fractional minutes;
// truncating to the second gives the cleaner form for tick math.
func humanDur(d time.Duration) string {
	return d.Truncate(time.Second).String()
}
