// Package world hosts world-loading and world-state primitives. The
// dayclock implements §9's day/night cycle: a single global clock
// derived from wall-clock + a persisted tick base, exposing a
// per-room EffectiveLight that ramps with phase for outdoor rooms.
package world

import (
	"sync"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// Cycle parameters. dayDuration is real-time per game day; tickInterval
// is the resolution at which the clock advances. ticksPerDay = the
// quotient — 1800 at the defaults, evenly divided into four phases of
// 450 ticks each. Override via WithDayDuration in tests.
const (
	defaultDayDuration = 30 * time.Minute
	defaultTickInterval = 1 * time.Second
)

// Phase is a coarse-grained day/night bucket. Rendering uses the
// finer-grained applyCurve output for actual light values; Phase is
// for human-facing displays (whereami, future `time` command).
type Phase int

const (
	PhaseDawn Phase = iota
	PhaseDay
	PhaseDusk
	PhaseNight
)

func (p Phase) String() string {
	switch p {
	case PhaseDawn:
		return "dawn"
	case PhaseDay:
		return "day"
	case PhaseDusk:
		return "dusk"
	case PhaseNight:
		return "night"
	default:
		return "unknown"
	}
}

// Clock is the day/night clock. baseTicks is the persisted tick count
// at process start; live ticks are derived from real-time elapsed
// since `start`. No goroutine; reads are pure.
type Clock struct {
	baseTicks    int64
	start        time.Time
	tickInterval time.Duration
	dayDuration  time.Duration
	now          func() time.Time
	mu           sync.Mutex // guards baseTicks updates (none today, future-proof)
}

// Option tunes a Clock at construction. Production uses defaults;
// tests inject WithNow + WithDayDuration for deterministic phases.
type Option func(*Clock)

// WithNow overrides the time source. The default is time.Now.
func WithNow(now func() time.Time) Option { return func(c *Clock) { c.now = now } }

// WithDayDuration overrides the real-time-per-game-day mapping.
func WithDayDuration(d time.Duration) Option { return func(c *Clock) { c.dayDuration = d } }

// WithTickInterval overrides the tick resolution.
func WithTickInterval(d time.Duration) Option { return func(c *Clock) { c.tickInterval = d } }

// NewClock constructs a clock anchored at the persisted base. Pass
// the value loaded from WorldStateRepo.GetTicks; on a fresh database
// migration 0024 seeds it at 675 (noon).
func NewClock(baseTicks int64, opts ...Option) *Clock {
	c := &Clock{
		baseTicks:    baseTicks,
		tickInterval: defaultTickInterval,
		dayDuration:  defaultDayDuration,
		now:          time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	c.start = c.now()
	return c
}

// Ticks returns the live tick count. Used by the persist.Manager
// saver to write the latest value back to world_state.
func (c *Clock) Ticks() int64 {
	elapsed := c.now().Sub(c.start)
	return c.baseTicks + int64(elapsed/c.tickInterval)
}

// ticksPerDay derives the cycle length from the tunable parameters.
func (c *Clock) ticksPerDay() int64 {
	return int64(c.dayDuration / c.tickInterval)
}

// Phase returns the current day-phase bucket.
func (c *Clock) Phase() Phase {
	tpd := c.ticksPerDay()
	if tpd <= 0 {
		return PhaseDay
	}
	q := tpd / 4
	p := c.Ticks() % tpd
	switch {
	case p < q:
		return PhaseDawn
	case p < 2*q:
		return PhaseDay
	case p < 3*q:
		return PhaseDusk
	default:
		return PhaseNight
	}
}

// EffectiveLight returns the room's current light level after applying
// the day/night cycle. Indoor / underground / underwater rooms ignore
// the cycle and return the stored baseline. The Dark flag is an
// explicit override and forces 0 regardless of phase or sector.
func (c *Clock) EffectiveLight(room repo.Room) int {
	if room.Flags.Dark {
		return 0
	}
	if room.Flags.Indoors ||
		room.Sector == repo.SectorUnderground ||
		room.Sector == repo.SectorUnderwater {
		return room.LightLevel
	}
	return applyCurve(room.LightLevel, c.Ticks(), c.ticksPerDay())
}

// applyCurve maps the (baseline, phase) pair onto the piecewise-linear
// dawn → day → dusk → night curve. Exported through EffectiveLight;
// kept package-private so the policy stays a single source of truth.
//
// At ticks 0..q-1   light ramps from 0 to baseline (dawn).
// At ticks q..2q-1  light is baseline (day).
// At ticks 2q..3q-1 light ramps from baseline to 0 (dusk).
// At ticks 3q..tpd  light is 0 (night).
func applyCurve(baseline int, ticks, ticksPerDay int64) int {
	if ticksPerDay <= 0 || baseline <= 0 {
		return baseline
	}
	q := ticksPerDay / 4
	p := ticks % ticksPerDay
	switch {
	case p < q:
		// Dawn: linear ramp 0 → baseline.
		return int(int64(baseline) * p / q)
	case p < 2*q:
		// Day.
		return baseline
	case p < 3*q:
		// Dusk: linear ramp baseline → 0.
		return int(int64(baseline) * (3*q - p) / q)
	default:
		// Night.
		return 0
	}
}
