package cmd

import (
	"github.com/Jasrags/WheelMUD/internal/creature"
)

// carryCapHeavy returns the heavy-load weight cap (lbs) for a given
// Strength score. Approximates the WoT d20 / D&D 3.5 carrying-
// capacity table: a fixed table for Str 1–10 and a smooth ×4-per-10
// exponential for Str ≥ 11. The exponential lands within ~1% of the
// printed Table 9-1 values (e.g. Str 13: table 150, formula ~152) but
// is not pixel-identical — it is a continuous curve, not a lookup.
// Light = ⅓ heavy, Medium = ⅔ heavy.
func carryCapHeavy(str int) float64 {
	if str < 1 {
		return baseHeavy[1]
	}
	if str <= 10 {
		return baseHeavy[str]
	}
	// 11 → 115; every +10 above 10 multiplies by 4.
	cap := baseHeavy[10]
	for s := 10; s < str; s++ {
		cap *= multStep[(s-10)%10]
	}
	return cap
}

// baseHeavy is the heavy-load table for Str 1-10.
var baseHeavy = map[int]float64{
	1: 10, 2: 20, 3: 30, 4: 40, 5: 50,
	6: 60, 7: 70, 8: 80, 9: 90, 10: 100,
}

// multStep is the per-Str-point multiplier above 10. Repeats every 10
// points so Str 20 is 4× Str 10, Str 30 is 16×, etc. Each step is the
// 10th root of 4 ≈ 1.1487.
var multStep = [10]float64{
	1.1487, 1.1487, 1.1487, 1.1487, 1.1487,
	1.1487, 1.1487, 1.1487, 1.1487, 1.1487,
}

// LoadFor reports the encumbrance band a given carried weight falls
// into for the given Strength score, plus the heavy-cap (max carry).
// Light: ≤ ⅓ heavy. Medium: ≤ ⅔ heavy. Heavy: ≤ heavy. Above heavy
// is Overloaded — pickup is blocked at this band.
func LoadFor(str int, carried float64) (creature.Load, float64) {
	heavy := carryCapHeavy(str)
	switch {
	case carried <= heavy/3:
		return creature.LoadLight, heavy
	case carried <= 2*heavy/3:
		return creature.LoadMedium, heavy
	case carried <= heavy:
		return creature.LoadHeavy, heavy
	default:
		return creature.LoadOverloaded, heavy
	}
}

// loadName returns a short player-facing label for a load band.
func loadName(l creature.Load) string {
	switch l {
	case creature.LoadLight:
		return "light load"
	case creature.LoadMedium:
		return "medium load"
	case creature.LoadHeavy:
		return "heavy load"
	default:
		return "overloaded"
	}
}
