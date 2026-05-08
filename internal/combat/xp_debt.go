package combat

import "github.com/Jasrags/WheelMUD/internal/progression"

// DeathPenaltyFraction is the share of "XP toward next level" that
// becomes XP debt when a character dies. 10% is the d20 MUD baseline:
// stings, scales with level, never wipes a level.
const DeathPenaltyFraction = 10

// DeathDebt returns the debt delta to add on character death.
//
// Formula: the current XP-into-current-level (xp - XPForLevel(level)),
// divided by DeathPenaltyFraction. Non-negative; clamps to 0 when xp
// is at or below the current level threshold so a fresh-level kill
// at the floor produces no sting and the player can never be pushed
// below their level threshold by the debt model.
//
// At MaxLevel the player has no "next level" target, but they still
// have XP-into-current that the formula bites — death keeps a cost
// at cap.
func DeathDebt(curXP int64, curLevel int) int64 {
	floor := progression.XPForLevel(curLevel)
	if curXP <= floor {
		return 0
	}
	return (curXP - floor) / DeathPenaltyFraction
}

// ApplyXPAward drains a pending debt off the top of an XP award.
// Returns the amount the player actually gains (added to their
// total XP) and the remaining debt after the drain. Negative or
// zero awards pass through with no debt change.
//
// Invariants (verified by tests):
//   - gain + (currentDebt - newDebt) == award (when award > 0).
//   - newDebt is never negative.
//   - newDebt is never > currentDebt.
//   - When currentDebt >= award, gain == 0 and newDebt =
//     currentDebt - award.
//   - When currentDebt < award, gain = award - currentDebt and
//     newDebt = 0.
func ApplyXPAward(award, currentDebt int64) (gain, newDebt int64) {
	if award <= 0 {
		return 0, currentDebt
	}
	if currentDebt <= 0 {
		return award, 0
	}
	if currentDebt >= award {
		return 0, currentDebt - award
	}
	return award - currentDebt, 0
}
