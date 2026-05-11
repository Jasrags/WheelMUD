package combat

import (
	"math/rand"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

func newAttacker(str int8, bab int16) creature.Core {
	return creature.Core{
		Abilities: creature.Abilities{
			Str: creature.AbilityScore{Current: str},
		},
		BAB: bab,
	}
}

func newDefender(defense int16) creature.Core {
	return creature.Core{Defense: defense}
}

func TestRollAttack_NaturalOneAlwaysMisses(t *testing.T) {
	// Seed 0: rng.Intn(20) returns 14 (15 raw). Force a controlled
	// stream by using a stub rng feeding 1.
	rng := rand.New(rand.NewSource(stubSeedForRoll(1)))
	roll := RollAttack(rng, newAttacker(20, 10), newDefender(0), repo.WeaponStats{}, false, 0)
	if roll.Hit {
		t.Fatalf("nat 1 must miss: %+v", roll)
	}
}

// stubSeedForRoll finds a seed for rand.Intn(20) returning desired-1.
// Cheap brute-force; only used in deterministic test setup.
func stubSeedForRoll(want int) int64 {
	for s := int64(1); s < 1000; s++ {
		r := rand.New(rand.NewSource(s))
		if r.Intn(20)+1 == want {
			return s
		}
	}
	return 1
}

// stubSeedFor2Rolls finds a seed producing the given d20 pair on the
// first two rand.Intn(20)+1 calls. Used by crit-confirmation tests so
// both the initial attack roll and the confirm roll are deterministic.
// Brute force up to ~1M seeds — every (1..20, 1..20) pair is reachable
// well within that window.
func stubSeedFor2Rolls(t *testing.T, first, second int) int64 {
	t.Helper()
	for s := int64(1); s < 1_000_000; s++ {
		r := rand.New(rand.NewSource(s))
		if r.Intn(20)+1 != first {
			continue
		}
		if r.Intn(20)+1 == second {
			return s
		}
	}
	t.Fatalf("no seed found for (%d,%d)", first, second)
	return 0
}

func TestRollAttack_NaturalTwentyAlwaysHits(t *testing.T) {
	rng := rand.New(rand.NewSource(stubSeedForRoll(20)))
	// Attacker with 0 BAB / 0 Str-mod against a 999 defense — only
	// the natural-20 short-circuit makes this hit.
	roll := RollAttack(rng, newAttacker(10, 0), newDefender(999), repo.WeaponStats{}, false, 0)
	if !roll.Hit {
		t.Fatalf("nat 20 must hit: %+v", roll)
	}
}

func TestRollAttack_Threshold(t *testing.T) {
	rng := rand.New(rand.NewSource(stubSeedForRoll(15)))
	// 15 + BAB 5 + Str mod 0 (Str 10) = 20 vs Defense 20 → hit.
	roll := RollAttack(rng, newAttacker(10, 5), newDefender(20), repo.WeaponStats{}, false, 0)
	if !roll.Hit {
		t.Fatalf("total>=defense must hit: %+v", roll)
	}
	rng = rand.New(rand.NewSource(stubSeedForRoll(10)))
	// 10 + 5 + 0 = 15 vs Defense 20 → miss.
	roll = RollAttack(rng, newAttacker(10, 5), newDefender(20), repo.WeaponStats{}, false, 0)
	if roll.Hit {
		t.Fatalf("total<defense must miss: %+v", roll)
	}
}

func TestRollAttack_ThreatThreshold(t *testing.T) {
	// Pre-confirmation Threat flag is set iff Raw >= ThreatLow AND Hit.
	// We seed the second roll to a guaranteed-confirm value (20) so
	// this test isolates the threat-range gate from the confirm logic
	// — confirm correctness is exercised by the dedicated tests below.
	stats := repo.WeaponStats{ThreatLow: 18}
	for _, raw := range []int{17, 18, 19, 20} {
		var seed int64
		if raw >= 18 {
			// Two rolls happen — second is the confirm; force success.
			seed = stubSeedFor2Rolls(t, raw, 20)
		} else {
			// One roll happens — no confirm attempted.
			seed = stubSeedForRoll(raw)
		}
		rng := rand.New(rand.NewSource(seed))
		roll := RollAttack(rng, newAttacker(10, 20), newDefender(0), stats, false, 0)
		if !roll.Hit {
			t.Fatalf("raw=%d expected hit: %+v", raw, roll)
		}
		wantThreat := raw >= 18
		if roll.Threat != wantThreat {
			t.Fatalf("raw=%d Threat=%v want=%v", raw, roll.Threat, wantThreat)
		}
		// With a guaranteed confirm, threats here also confirm.
		if roll.IsCrit != wantThreat {
			t.Fatalf("raw=%d IsCrit=%v want=%v (confirm forced)", raw, roll.IsCrit, wantThreat)
		}
	}
}

func TestRollAttack_CritConfirmation_HighBABConfirms(t *testing.T) {
	// Raw 19 threatens; confirm raw 15 + 20 BAB = 35 >> Defense 0,
	// so the crit confirms.
	stats := repo.WeaponStats{ThreatLow: 18}
	seed := stubSeedFor2Rolls(t, 19, 15)
	rng := rand.New(rand.NewSource(seed))
	roll := RollAttack(rng, newAttacker(10, 20), newDefender(0), stats, false, 0)
	if !roll.Hit || !roll.Threat || !roll.IsCrit {
		t.Fatalf("expected Hit+Threat+IsCrit: %+v", roll)
	}
}

func TestRollAttack_CritConfirmation_LowBABFailsToConfirm(t *testing.T) {
	// Raw 18 threatens (initial hit forced by Hit override: total
	// 18+0+0=18 >= Defense 15 → hit). Confirm raw 2 + 0 = 2 < 15 →
	// fails. Result: regular hit, Threat=true, IsCrit=false.
	stats := repo.WeaponStats{ThreatLow: 18}
	seed := stubSeedFor2Rolls(t, 18, 2)
	rng := rand.New(rand.NewSource(seed))
	roll := RollAttack(rng, newAttacker(10, 0), newDefender(15), stats, false, 0)
	if !roll.Hit {
		t.Fatalf("expected Hit (raw 18 + 0 vs Defense 15): %+v", roll)
	}
	if !roll.Threat {
		t.Fatalf("expected Threat: %+v", roll)
	}
	if roll.IsCrit {
		t.Fatalf("expected !IsCrit (confirm of 2 should miss): %+v", roll)
	}
}

func TestRollAttack_CritConfirmation_Natural20AutoConfirms(t *testing.T) {
	// First raw 19 against a Defense the attacker can otherwise beat;
	// confirm raw 20 auto-confirms even when the modifiers wouldn't.
	stats := repo.WeaponStats{ThreatLow: 18}
	seed := stubSeedFor2Rolls(t, 19, 20)
	rng := rand.New(rand.NewSource(seed))
	// 0 BAB / 0 Str against Defense 999: only the confirm's natural-20
	// short-circuit lets IsCrit fire.
	roll := RollAttack(rng, newAttacker(10, 0), newDefender(999), stats, false, 0)
	if !roll.Hit {
		// First roll was 19, total 19, vs 999 → would miss without nat-20.
		// We assert IsCrit only when Hit is true; if the first roll
		// doesn't land, the threat never happens. So switch fixture:
		// natural-20 first to force the hit, natural-20 confirm to
		// force the crit.
		seed = stubSeedFor2Rolls(t, 20, 20)
		rng = rand.New(rand.NewSource(seed))
		roll = RollAttack(rng, newAttacker(10, 0), newDefender(999), stats, false, 0)
	}
	if !roll.Hit || !roll.Threat || !roll.IsCrit {
		t.Fatalf("expected nat-20 confirm to auto-succeed: %+v", roll)
	}
}

func TestRollAttack_CritConfirmation_Natural1AutoFails(t *testing.T) {
	// First raw 20 (auto-hit, threatens with any ThreatLow ≤ 20);
	// confirm raw 1 auto-fails even when the modifiers would beat
	// Defense.
	stats := repo.WeaponStats{ThreatLow: 18}
	seed := stubSeedFor2Rolls(t, 20, 1)
	rng := rand.New(rand.NewSource(seed))
	// 20 BAB against Defense 0 — total of confirm (1 + 20 = 21) would
	// beat Defense if not for the nat-1 short-circuit.
	roll := RollAttack(rng, newAttacker(10, 20), newDefender(0), stats, false, 0)
	if !roll.Hit || !roll.Threat {
		t.Fatalf("expected Hit+Threat: %+v", roll)
	}
	if roll.IsCrit {
		t.Fatalf("expected !IsCrit (nat-1 confirm must fail): %+v", roll)
	}
}

// countingSource wraps a *rand.Rand-compatible source so a test can
// assert the number of d20 rolls drawn by RollAttack. Used to pin the
// "no confirm roll when not in threat range" invariant.
type countingSource struct {
	inner rand.Source64
	calls int
}

func (c *countingSource) Int63() int64 {
	c.calls++
	return c.inner.Int63()
}
func (c *countingSource) Uint64() uint64 {
	c.calls++
	return c.inner.Uint64()
}
func (c *countingSource) Seed(s int64) { c.inner.Seed(s) }

func TestRollAttack_NoConfirmRollWhenNotThreat(t *testing.T) {
	// Raw 17 with ThreatLow 18 → no threat, no confirm roll. The
	// underlying source must only be drawn once.
	stats := repo.WeaponStats{ThreatLow: 18}
	inner := rand.NewSource(stubSeedForRoll(17)).(rand.Source64)
	counter := &countingSource{inner: inner}
	rng := rand.New(counter)
	roll := RollAttack(rng, newAttacker(10, 20), newDefender(0), stats, false, 0)
	if !roll.Hit || roll.Threat || roll.IsCrit {
		t.Fatalf("expected plain hit: %+v", roll)
	}
	if counter.calls != 1 {
		t.Fatalf("expected 1 source draw, got %d", counter.calls)
	}
}

func TestRollDamage_AppliesStrModAndCritMult(t *testing.T) {
	stats := repo.WeaponStats{Damage: "1d1+0", CritMult: 3}
	rng := rand.New(rand.NewSource(1))
	atk := newAttacker(16, 0) // +3 Str mod
	dmg := RollDamage(rng, atk, stats, false, VariantNormal)
	if dmg != 1+3 {
		t.Fatalf("non-crit dmg = %d, want %d", dmg, 4)
	}
	dmg = RollDamage(rng, atk, stats, true, VariantNormal)
	if dmg != (1+3)*3 {
		t.Fatalf("crit dmg = %d, want %d", dmg, 12)
	}
}

// TestRollAttack_VariantBonus verifies the slice-61 attack-roll
// adjustment composes onto raw + BAB + StrMod. Hits/misses change
// at the boundary between Normal and Power/Quick.
func TestRollAttack_VariantBonus(t *testing.T) {
	atk := newAttacker(10, 5) // +0 Str, +5 BAB
	def := newDefender(20)    // Defense 20
	// raw=15 → total = 15+5+0 = 20 hits Normal; with Power (-2) it
	// drops to 18 and misses; with Quick (+1) it climbs to 21 and
	// hits.
	rng := rand.New(rand.NewSource(stubSeedForRoll(15)))
	if r := RollAttack(rng, atk, def, repo.WeaponStats{}, false, VariantAttackBonus(VariantNormal)); !r.Hit {
		t.Fatalf("normal raw=15 expected hit: %+v", r)
	}
	rng = rand.New(rand.NewSource(stubSeedForRoll(15)))
	if r := RollAttack(rng, atk, def, repo.WeaponStats{}, false, VariantAttackBonus(VariantPower)); r.Hit {
		t.Fatalf("power raw=15 expected miss (-2): %+v", r)
	}
	rng = rand.New(rand.NewSource(stubSeedForRoll(14)))
	if r := RollAttack(rng, atk, def, repo.WeaponStats{}, false, VariantAttackBonus(VariantQuick)); !r.Hit {
		t.Fatalf("quick raw=14 expected hit (+1): %+v", r)
	}
}

// TestRollDamage_VariantFactor verifies Power scales rolled damage
// by 1.5 and Quick by 0.6 (with the ≥1 floor). Power-on-crit
// preserves the weapon's crit multiplier — both factors stack.
func TestRollDamage_VariantFactor(t *testing.T) {
	stats := repo.WeaponStats{Damage: "1d1+0", CritMult: 2}
	atk := newAttacker(16, 0) // +3 Str → base damage 1+3 = 4
	rng := rand.New(rand.NewSource(1))

	// Normal non-crit = 4
	if dmg := RollDamage(rng, atk, stats, false, VariantNormal); dmg != 4 {
		t.Errorf("normal non-crit = %d, want 4", dmg)
	}
	// Power non-crit = floor(4 * 1.5) = 6
	if dmg := RollDamage(rng, atk, stats, false, VariantPower); dmg != 6 {
		t.Errorf("power non-crit = %d, want 6", dmg)
	}
	// Quick non-crit = floor(4 * 0.6) = 2
	if dmg := RollDamage(rng, atk, stats, false, VariantQuick); dmg != 2 {
		t.Errorf("quick non-crit = %d, want 2", dmg)
	}
	// Power crit chains both: (4 * 2) * 1.5 = 12
	if dmg := RollDamage(rng, atk, stats, true, VariantPower); dmg != 12 {
		t.Errorf("power crit = %d, want 12", dmg)
	}
}

// TestRollDamage_QuickFloorsAtOne verifies Quick on a 1-damage
// base still produces ≥1 instead of rounding to zero.
func TestRollDamage_QuickFloorsAtOne(t *testing.T) {
	stats := repo.WeaponStats{Damage: "1d1"}
	atk := newAttacker(10, 0) // +0 Str → base 1
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 5; i++ {
		if dmg := RollDamage(rng, atk, stats, false, VariantQuick); dmg < 1 {
			t.Fatalf("quick floor: dmg=%d", dmg)
		}
	}
}

func TestRollDamage_FloorOne(t *testing.T) {
	// Str 6 = -2 mod, "1d1" base = 1, total 1-2 = -1 → floored to 1.
	stats := repo.WeaponStats{Damage: "1d1"}
	rng := rand.New(rand.NewSource(1))
	atk := newAttacker(6, 0)
	if dmg := RollDamage(rng, atk, stats, false, VariantNormal); dmg < 1 {
		t.Fatalf("damage floor: got %d, want >=1", dmg)
	}
}

func TestApplyDamage_DRClamps(t *testing.T) {
	target := &creature.Core{
		HPCurrent: 50,
		DR:        []creature.DamageReduction{{Amount: 5}},
	}
	dealt := applyDamage(target, 12, creature.DamageSlash)
	if dealt != 7 {
		t.Fatalf("DR clamp: dealt = %d, want 7", dealt)
	}
	if target.HPCurrent != 43 {
		t.Fatalf("HP after = %d, want 43", target.HPCurrent)
	}

	dealt = applyDamage(target, 3, creature.DamageSlash)
	if dealt != 0 {
		t.Fatalf("DR fully absorbs: dealt = %d, want 0", dealt)
	}
	if target.HPCurrent != 43 {
		t.Fatalf("HP must not move on absorbed dmg: %d", target.HPCurrent)
	}
}

func TestApplyDamage_ResistPercent(t *testing.T) {
	target := &creature.Core{
		HPCurrent: 100,
		Resists:   []creature.Resist{{Type: creature.DamageFire, Pct: 50}},
	}
	dealt := applyDamage(target, 20, creature.DamageFire)
	if dealt != 10 {
		t.Fatalf("50%% resist: dealt = %d, want 10", dealt)
	}
	if target.HPCurrent != 90 {
		t.Fatalf("HP after = %d, want 90", target.HPCurrent)
	}
}

func TestApplyDamage_VulnerabilityNegativeResist(t *testing.T) {
	target := &creature.Core{
		HPCurrent: 100,
		Resists:   []creature.Resist{{Type: creature.DamageCold, Pct: -50}},
	}
	dealt := applyDamage(target, 10, creature.DamageCold)
	if dealt != 15 {
		t.Fatalf("vuln: dealt = %d, want 15", dealt)
	}
}

func TestApplyDamage_SubdualMovesPool(t *testing.T) {
	target := &creature.Core{HPCurrent: 50}
	dealt := applyDamage(target, 8, creature.DamageSubdual)
	if dealt != 8 {
		t.Fatalf("subdual dealt = %d, want 8", dealt)
	}
	if target.Subdual != 8 {
		t.Fatalf("Subdual pool = %d, want 8", target.Subdual)
	}
	if target.HPCurrent != 50 {
		t.Fatalf("HP must not move on subdual: %d", target.HPCurrent)
	}
}

func TestApplyDamage_NilOrZeroNoop(t *testing.T) {
	if dealt := applyDamage(nil, 5, creature.DamageSlash); dealt != 0 {
		t.Fatalf("nil target: dealt = %d, want 0", dealt)
	}
	target := &creature.Core{HPCurrent: 10}
	if dealt := applyDamage(target, 0, creature.DamageSlash); dealt != 0 {
		t.Fatalf("zero amt: dealt = %d, want 0", dealt)
	}
	if target.HPCurrent != 10 {
		t.Fatalf("HP moved on zero dmg: %d", target.HPCurrent)
	}
}

func TestWeaponPrimaryDamageType(t *testing.T) {
	cases := []struct {
		name string
		ws   repo.WeaponStats
		want creature.DamageType
	}{
		{"slash", repo.WeaponStats{DamageType: []string{"S"}}, creature.DamageSlash},
		{"pierce", repo.WeaponStats{DamageType: []string{"p"}}, creature.DamagePierce},
		{"bludgeon", repo.WeaponStats{DamageType: []string{"B"}}, creature.DamageBludgeon},
		{"empty defaults bludgeon", repo.WeaponStats{}, creature.DamageBludgeon},
		{"unknown defaults bludgeon", repo.WeaponStats{DamageType: []string{"X"}}, creature.DamageBludgeon},
		{"subdual flag overrides", repo.WeaponStats{Subdual: true, DamageType: []string{"S"}}, creature.DamageSubdual},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := weaponPrimaryDamageType(tc.ws); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRollDice_Parses(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	cases := []struct {
		expr string
		minV int
		maxV int
	}{
		{"1d1", 1, 1},
		{"2d1", 2, 2},
		{"1d6", 1, 6},
		{"2d4+1", 3, 9},
		{"1d6/1d6", 1, 6}, // first term only for slice 1
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			for i := 0; i < 20; i++ {
				v, ok := rollDice(rng, tc.expr)
				if !ok {
					t.Fatalf("rollDice(%q) failed", tc.expr)
				}
				if v < tc.minV || v > tc.maxV {
					t.Fatalf("rollDice(%q) = %d, out of [%d,%d]", tc.expr, v, tc.minV, tc.maxV)
				}
			}
		})
	}
	for _, bad := range []string{"", "abc", "d6", "0d6", "1d", "1d0"} {
		if _, ok := rollDice(rng, bad); ok {
			t.Fatalf("rollDice(%q) succeeded, want failure", bad)
		}
	}
}
