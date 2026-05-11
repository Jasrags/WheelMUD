package combat

import (
	"math"
	"math/rand"
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// AttackRoll bundles the components of a melee attack roll. Returned
// from RollAttack so the manager can publish CombatHit / CombatMiss
// with the right values without re-rolling.
//
// SRD-style critical confirmation: when the raw d20 lands in the
// weapon's threat range, RollAttack rolls a second d20 + the same
// modifiers vs the same Defense. `Threat` records that the first roll
// was in the threat range (event renderers use it for the "threat —
// fails to confirm" wording); `IsCrit` is set only when the confirm
// roll also hits, and is what the damage layer reads for the
// multiplier. `Threat && !IsCrit && Hit` is a regular (non-multiplied)
// hit with a distinct event line.
type AttackRoll struct {
	Raw    int  // raw d20, 1..20
	Total  int  // d20 + bab + abilityMod
	Hit    bool // Total >= defender Defense (or natural-20 auto-hit)
	Threat bool // Raw >= weapon ThreatLow (default 20) AND Hit
	IsCrit bool // Threat && confirm roll also hits
}

// unarmedDamage is the fallback weapon stat block used when a
// combatant has nothing in PrimaryWield. Models a "1d3 bludgeon"
// strike — light, never crits special, never subdual.
var unarmedDamage = repo.WeaponStats{
	Damage:     "1d3",
	DamageType: []string{"B"},
	Range:      "melee",
}

// weaponStatsFor pulls *WeaponStats off an Item if Type matches and
// Stats parsed cleanly; falls back to unarmedDamage when the item is
// missing, the wrong type, or wasn't decoded. Caller passes in a
// loaded *repo.Item or nil.
func weaponStatsFor(it *repo.Item) repo.WeaponStats {
	if it == nil {
		return unarmedDamage
	}
	if it.Type != repo.ItemTypeWeapon {
		return unarmedDamage
	}
	ws, ok := it.Stats.(*repo.WeaponStats)
	if !ok || ws == nil {
		return unarmedDamage
	}
	return *ws
}

// weaponPrimaryDamageType maps the first WeaponStats.DamageType string
// (B / P / S) to the creature.DamageType enum. Defaults to
// DamageBludgeon for unknown / empty values — unarmed strikes share
// that default. Subdual weapons override to DamageSubdual at the
// caller (RollDamage handles it via WeaponStats.Subdual).
func weaponPrimaryDamageType(stats repo.WeaponStats) creature.DamageType {
	if stats.Subdual {
		return creature.DamageSubdual
	}
	for _, code := range stats.DamageType {
		switch strings.ToUpper(strings.TrimSpace(code)) {
		case "S":
			return creature.DamageSlash
		case "P":
			return creature.DamagePierce
		case "B":
			return creature.DamageBludgeon
		}
	}
	return creature.DamageBludgeon
}

// threatLow returns the lower bound of the crit-threat range,
// defaulting to 20 (only natural-20 threatens) when the weapon has no
// override.
func threatLow(stats repo.WeaponStats) int {
	if stats.ThreatLow <= 0 || stats.ThreatLow > 20 {
		return 20
	}
	return stats.ThreatLow
}

// critMult returns the crit damage multiplier, defaulting to 2.
func critMult(stats repo.WeaponStats) int {
	if stats.CritMult <= 1 {
		return 2
	}
	return stats.CritMult
}

// RollAttack rolls d20 + BAB + Str-mod (+ bonus) against
// defender.Defense. A natural 1 always misses; a natural 20 always
// hits. A roll in the weapon's threat range (defaulting to 20) flags
// Threat and triggers a confirmation roll vs the same Defense; the
// multiplier-applying IsCrit flag is set only when that confirm also
// hits (SRD rules — natural 20 on confirm auto-succeeds, natural 1
// auto-fails).
//
// When flatFootedDefender is true the defender's effective Defense is
// reduced by max(0, DexMod) — the standard WoT/d20 flat-footed AC
// penalty. bonus is the slice-61 variant attack-roll adjustment
// (Normal 0 / Power -2 / Quick +1); future slices fold feat / stance
// / status modifiers into the same term. The bonus also applies to
// the confirmation roll (the SRD lets feats like Power Critical add a
// separate confirm-only bonus; that surface is deferred).
func RollAttack(rng *rand.Rand, attacker, defender creature.Core, stats repo.WeaponStats, flatFootedDefender bool, bonus int) AttackRoll {
	raw := rng.Intn(20) + 1
	abilityMod := int(attacker.Abilities.StrMod())
	total := raw + int(attacker.BAB) + abilityMod + bonus

	defense := int(effectiveDefense(defender, flatFootedDefender))
	hit := total >= defense
	switch raw {
	case 1:
		hit = false
	case 20:
		hit = true
	}

	threat := hit && raw >= threatLow(stats)
	isCrit := false
	if threat {
		confirmRaw := rng.Intn(20) + 1
		confirmTotal := confirmRaw + int(attacker.BAB) + abilityMod + bonus
		switch confirmRaw {
		case 1:
			isCrit = false
		case 20:
			isCrit = true
		default:
			isCrit = confirmTotal >= defense
		}
	}

	return AttackRoll{
		Raw:    raw,
		Total:  total,
		Hit:    hit,
		Threat: threat,
		IsCrit: isCrit,
	}
}

// effectiveDefense returns defender.Defense reduced by the defender's
// positive Dex modifier when flatFooted is true. A negative DexMod
// already counts against Defense at character-build time, so we don't
// double-subtract on the FlatFooted path.
func effectiveDefense(defender creature.Core, flatFooted bool) int16 {
	if !flatFooted {
		return defender.Defense
	}
	dex := int16(defender.Abilities.DexMod())
	if dex <= 0 {
		return defender.Defense
	}
	return defender.Defense - dex
}

// RollParry rolls a defender's opposed parry total: d20 + BAB +
// DexMod. The Manager compares this against the attacker's AttackRoll
// total — strictly greater wins for the defender, ties go to the
// attacker (matching the d20 contested-roll convention).
func RollParry(rng *rand.Rand, defender creature.Core) int {
	raw := rng.Intn(20) + 1
	return raw + int(defender.BAB) + int(defender.Abilities.DexMod())
}

// RollDamage rolls weapon damage + Str-mod. On crit, the result is
// multiplied by the weapon's CritMult. The variant factor (Power
// ×1.5 / Quick ×0.6 / Normal ×1.0) is applied after crit-mult so a
// Power-crit chains both multipliers. Returns at least 1 on a hit so
// a damage roll of zero (or a Quick swing rounding to 0) never
// produces a "free" no-op hit. The returned amount is pre-DR /
// pre-resist.
func RollDamage(rng *rand.Rand, attacker creature.Core, stats repo.WeaponStats, isCrit bool, variant AttackVariant) int32 {
	base, ok := rollDice(rng, stats.Damage)
	if !ok || base <= 0 {
		// "1d3" fallback when the weapon's Damage string is unparseable
		// or empty — keeps the resolver moving for a malformed catalog
		// row instead of swallowing the hit.
		base = rng.Intn(3) + 1
	}
	dmg := base + int(attacker.Abilities.StrMod())
	if dmg < 1 {
		dmg = 1
	}
	if isCrit {
		dmg *= critMult(stats)
	}
	if dmg < 1 {
		dmg = 1
	}
	if f := VariantDamageFactor(variant); f != 1.0 {
		scaled := int(float64(dmg) * f)
		if scaled < 1 {
			scaled = 1
		}
		dmg = scaled
	}
	// Clamp to int32 so a pathological catalog row (huge flat
	// modifier in WeaponStats.Damage) can't wrap to negative damage,
	// which applyDamage would silently treat as a no-op.
	if dmg > math.MaxInt32 {
		dmg = math.MaxInt32
	}
	return int32(dmg)
}

// rollDice parses a "NdM" or "NdM+K" / "NdM-K" expression and rolls
// it. Returns (sum, true) on success or (0, false) when the string
// can't be parsed. Slice-1 grammar: one term, N and M positive,
// optional signed flat modifier. Compound shorthands like "1d6/1d6"
// (double weapons) take the first half — the second-end-of-staff
// strike is a #18 follow-up.
func rollDice(rng *rand.Rand, expr string) (int, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, false
	}
	// Take the first term of compound expressions ("1d6/1d6").
	if i := strings.IndexByte(expr, '/'); i >= 0 {
		expr = expr[:i]
	}
	flat := 0
	if i := strings.IndexAny(expr, "+-"); i > 0 {
		mod, err := strconv.Atoi(expr[i:])
		if err != nil {
			return 0, false
		}
		// Cap the flat modifier so a malformed catalog row can't
		// produce damage that wraps int32 downstream. Real WoT
		// weapons sit at low single digits.
		if mod < -10000 || mod > 10000 {
			return 0, false
		}
		flat = mod
		expr = expr[:i]
	}
	d := strings.IndexByte(expr, 'd')
	if d <= 0 || d == len(expr)-1 {
		return 0, false
	}
	n, err := strconv.Atoi(expr[:d])
	if err != nil || n <= 0 || n > 100 {
		return 0, false
	}
	m, err := strconv.Atoi(expr[d+1:])
	if err != nil || m <= 0 || m > 1000 {
		return 0, false
	}
	sum := flat
	for i := 0; i < n; i++ {
		sum += rng.Intn(m) + 1
	}
	return sum, true
}

// applyDamage mutates target.HPCurrent by amount, after subtracting
// any matching DR (flat) and applying percent resists. Returns the
// damage actually taken — the value that should appear in the
// CombatHit event. Subdual damage moves into target.Subdual rather
// than HPCurrent.
//
// DR.Bypass is a single keyword today; a future iteration can split
// on whitespace or commas. An empty Bypass means the DR applies to
// every attack of any type.
func applyDamage(target *creature.Core, amount int32, dt creature.DamageType) int32 {
	if amount <= 0 || target == nil {
		return 0
	}
	for _, dr := range target.DR {
		if dr.Amount <= 0 {
			continue
		}
		if dr.Bypass != "" {
			// Bypass is type-name based until the weapon catalog grows
			// real keyword tags. "magic" / "cold-iron" stay deferred.
			continue
		}
		amount -= int32(dr.Amount)
		if amount <= 0 {
			return 0
		}
	}
	for _, r := range target.Resists {
		if r.Type != dt || r.Pct == 0 {
			continue
		}
		// Pct positive = resist (reduce); negative = vulnerability.
		adjusted := int64(amount) * (100 - int64(r.Pct)) / 100
		if adjusted < 0 {
			adjusted = 0
		}
		amount = int32(adjusted)
	}
	if amount <= 0 {
		return 0
	}
	if dt == creature.DamageSubdual {
		target.Subdual += amount
		return amount
	}
	target.HPCurrent -= amount
	return amount
}
