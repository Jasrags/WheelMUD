// Package progression holds pure-function helpers for the d20-style
// XP / level curve used by Phase E #23 (Levels & XP curve).
//
// The curve is content, not state — this package never touches the DB
// or any session. Verbs (xp, train) and the level-up commit path import
// these helpers; tests can table-drive every boundary without harness
// scaffolding.
//
// Curve: xp(n) = 1000 · n · (n - 1) / 2  (per ROADMAP §12).
// Level cap: 20 — XP beyond XPForLevel(MaxLevel) still reports MaxLevel.
package progression

// MaxLevel is the V1 hard level cap. XP totals above
// XPForLevel(MaxLevel) still report level == MaxLevel and toNext == 0.
const MaxLevel = 20

// XPForLevel returns the cumulative XP threshold to be at level n.
// Level 1 is the chargen baseline (threshold 0). Inputs below 1 clamp
// to 0; inputs above MaxLevel clamp to XPForLevel(MaxLevel) — the cap
// is hard, not infinite-extrapolated.
func XPForLevel(n int) int64 {
	if n <= 1 {
		return 0
	}
	if n > MaxLevel {
		n = MaxLevel
	}
	// 1000 · n · (n-1) / 2 — exact integer arithmetic.
	return int64(1000) * int64(n) * int64(n-1) / 2
}

// LevelForXP returns the highest n in [1, MaxLevel] whose XPForLevel
// is <= xp. xp <= 0 returns 1 (chargen baseline). xp at or above the
// cap threshold returns MaxLevel.
func LevelForXP(xp int64) int {
	if xp <= 0 {
		return 1
	}
	// Curve is monotonic; small range (20) makes a linear walk
	// trivially the right call. Walk down so the common case
	// (mid-game characters) bottoms out fast on average.
	for n := MaxLevel; n >= 2; n-- {
		if xp >= XPForLevel(n) {
			return n
		}
	}
	return 1
}

// XPToNext reports the current level (LevelForXP(xp)) and how much XP
// remains before the next level threshold. At MaxLevel toNext is 0 —
// callers should render "--" or equivalent rather than a numeric goal.
func XPToNext(xp int64) (level int, toNext int64) {
	level = LevelForXP(xp)
	if level >= MaxLevel {
		return level, 0
	}
	next := XPForLevel(level + 1)
	if xp >= next {
		// Defensive: LevelForXP guarantees this can't happen, but
		// keep the contract crisp.
		return level, 0
	}
	return level, next - xp
}
