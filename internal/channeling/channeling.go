// Package channeling models per-tick mechanics on a creature.Channeling
// record: 8h slot-pool refresh, Madness accrual while embraced for male
// channelers, and the Stilled gate. All transforms are pure: they
// mutate their *creature.Channeling argument in place and return a
// "changed" bool so the per-tick driver can persist iff something
// actually moved. Stilled chars never refresh; Madness only accrues
// for embraced Saidin channelers.
//
// Phase E #27 ships accrual only — no thresholds, no saves, no
// symptoms. Embracing's rest/heal blockers and the Mental Stability
// save layer on later, gated on a `rest` verb landing.
package channeling

import (
	"math"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// RefreshInterval is the wall-clock gap between full slot refills.
// Wall-clock rather than Clock.HourOfDay() so per-character pacing
// doesn't reset on server restart and a fresh chargen char doesn't
// have to wait until the next in-game dawn for usable slots.
const RefreshInterval = 8 * time.Hour

// MadnessPerPulse is the per-Regen-bucket accrual delta while a male
// channeler holds the Source. The Regen bucket fires every 30s by
// default — at 1/pulse, 100% Madness (int16 max ≈ 32767) takes
// roughly 11 days of continuous embrace, well above any plausible
// session length but bounded to keep the int16 from wrapping.
const MadnessPerPulse int16 = 1

// MadnessMax clamps accrual at the int16 ceiling.
const MadnessMax int16 = math.MaxInt16

// RefreshIfDue refills every Slots[i].Cur to Slots[i].Max iff the
// last refresh was more than RefreshInterval ago and the channeler
// is not Stilled. Returns true iff any state moved.
//
// A Stilled channeler whose slots are already at zero stays at zero
// — RefreshIfDue is the runtime gate the §27 plan calls out. The
// admin verb that flips Stilled also zeroes Cur to make the gate's
// effect immediate.
//
// nil c is a no-op.
func RefreshIfDue(c *creature.Channeling, now time.Time) bool {
	if c == nil || c.Stilled {
		return false
	}
	if !c.LastSlotRefreshAt.IsZero() && now.Sub(c.LastSlotRefreshAt) < RefreshInterval {
		return false
	}
	for i := range c.Slots {
		c.Slots[i].Cur = c.Slots[i].Max
	}
	// Stamp the timestamp on every due pulse so the next gate fires
	// 8h from now and not every tick.
	c.LastSlotRefreshAt = now
	return true
}

// AccrueMadness adds MadnessPerPulse to c.Madness iff the channeler
// is currently Embraced and drawing on Saidin (male channelers).
// Saidar channelers, unembraced channelers, and stilled channelers
// are unaffected. Clamps at MadnessMax.
//
// Returns true iff Madness moved.
//
// nil c is a no-op.
func AccrueMadness(c *creature.Channeling, _ time.Time) bool {
	if c == nil {
		return false
	}
	if c.Stilled || !c.Embraced {
		return false
	}
	if c.GenderSource != creature.SourceSaidin {
		return false
	}
	if c.Madness >= MadnessMax {
		return false
	}
	next := int32(c.Madness) + int32(MadnessPerPulse)
	if next > int32(MadnessMax) {
		next = int32(MadnessMax)
	}
	c.Madness = int16(next)
	return true
}
