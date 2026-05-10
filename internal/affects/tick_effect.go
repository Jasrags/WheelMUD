package affects

import "github.com/Jasrags/WheelMUD/internal/creature"

// TickEvent is one HP delta from a single TickEffect-bearing affect.
// The session ticker turns these into player-visible lines via a
// cmd-layer subscriber.
type TickEvent struct {
	Name   string // affect name (used as the "cause" label)
	Delta  int16  // signed delta applied to HPCurrent (negative = damage)
	NewHP  int32  // HPCurrent after this tick
	Effect string // the TickEffect script ref (informational; "poison", "bleed", "regen")
}

// ApplyTickEffects walks c.Affects and folds every entry whose
// TickEffect != "" and TickDamage != 0 into HPCurrent. Returns the
// post-tick HP plus a per-affect event slice in the order processed.
//
// Pure: c is not mutated. Clamps at 0 (floor) and HPMax (ceiling).
// Affects without a TickEffect or with TickDamage == 0 are skipped so
// pure stat-mod / condition-only affects don't accidentally tick HP.
//
// HPMax == 0 is treated as "no ceiling" (defensive — chargen always
// stamps HPMax > 0, but a corrupt row shouldn't double-clamp the
// player to zero).
func ApplyTickEffects(c creature.Core) (newHP int32, events []TickEvent) {
	hp := c.HPCurrent
	for _, a := range c.Affects {
		if a.TickEffect == "" || a.TickDamage == 0 {
			continue
		}
		delta := int32(a.TickDamage)
		next := hp + delta
		if next < 0 {
			next = 0
		}
		if c.HPMax > 0 && next > c.HPMax {
			next = c.HPMax
		}
		applied := next - hp
		if applied == 0 {
			continue
		}
		events = append(events, TickEvent{
			Name:   a.Name,
			Delta:  int16(applied),
			NewHP:  next,
			Effect: a.TickEffect,
		})
		hp = next
	}
	return hp, events
}
