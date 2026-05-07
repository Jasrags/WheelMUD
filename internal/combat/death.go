package combat

import (
	"github.com/Jasrags/WheelMUD/internal/creature"
)

// xpValueForChallenge returns the base XP awarded when a mob of the
// given ChallengeCode dies. WoT canon uses A–I letter codes; the
// curve below is a linear-then-doubling table the design notes call
// out for V1. A YAML-overridable value on MobTemplate is a deferred
// follow-up.
//
// Unknown / zero codes default to the A-tier amount so a malformed
// template still grants something rather than silently zeroing the
// reward. Defaulting up rather than down keeps "I never get XP from
// these guys" off the bug list during early playtest.
func xpValueForChallenge(code creature.ChallengeCode) int64 {
	switch code {
	case 'A':
		return 100
	case 'B':
		return 250
	case 'C':
		return 600
	case 'D':
		return 1200
	case 'E':
		return 2400
	case 'F':
		return 4800
	case 'G':
		return 9600
	case 'H':
		return 19200
	case 'I':
		return 38400
	}
	return 100
}

// allocateXP splits totalXP across attackers proportional to their
// recorded damage tally. Returns a stable map of per-actor awards.
// Actors with zero recorded damage are excluded. When the tally is
// empty (e.g. a mob died to environmental damage that never went
// through resolveAction) the full amount goes to the killer if
// known, else nothing is awarded.
func allocateXP(tally map[ActorRef]int32, totalXP int64, killer ActorRef) map[ActorRef]int64 {
	if totalXP <= 0 {
		return nil
	}
	out := make(map[ActorRef]int64)
	var sum int64
	for _, dmg := range tally {
		if dmg > 0 {
			sum += int64(dmg)
		}
	}
	if sum == 0 {
		if killer.Kind != ActorKindUnknown {
			out[killer] = totalXP
		}
		return out
	}
	allocated := int64(0)
	for ref, dmg := range tally {
		if dmg <= 0 {
			continue
		}
		share := totalXP * int64(dmg) / sum
		if share > 0 {
			out[ref] = share
			allocated += share
		}
	}
	// Round-off remainder goes to the killer (or any actor with the
	// largest tally if killer's share already accounts for it). Keeps
	// "1 XP missing" trivia off the bug list.
	if remainder := totalXP - allocated; remainder > 0 {
		if killer.Kind != ActorKindUnknown {
			out[killer] += remainder
		}
	}
	return out
}
