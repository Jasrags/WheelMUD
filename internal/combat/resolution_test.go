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
	roll := RollAttack(rng, newAttacker(20, 10), newDefender(0), repo.WeaponStats{}, false)
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

func TestRollAttack_NaturalTwentyAlwaysHits(t *testing.T) {
	rng := rand.New(rand.NewSource(stubSeedForRoll(20)))
	// Attacker with 0 BAB / 0 Str-mod against a 999 defense — only
	// the natural-20 short-circuit makes this hit.
	roll := RollAttack(rng, newAttacker(10, 0), newDefender(999), repo.WeaponStats{}, false)
	if !roll.Hit {
		t.Fatalf("nat 20 must hit: %+v", roll)
	}
}

func TestRollAttack_Threshold(t *testing.T) {
	rng := rand.New(rand.NewSource(stubSeedForRoll(15)))
	// 15 + BAB 5 + Str mod 0 (Str 10) = 20 vs Defense 20 → hit.
	roll := RollAttack(rng, newAttacker(10, 5), newDefender(20), repo.WeaponStats{}, false)
	if !roll.Hit {
		t.Fatalf("total>=defense must hit: %+v", roll)
	}
	rng = rand.New(rand.NewSource(stubSeedForRoll(10)))
	// 10 + 5 + 0 = 15 vs Defense 20 → miss.
	roll = RollAttack(rng, newAttacker(10, 5), newDefender(20), repo.WeaponStats{}, false)
	if roll.Hit {
		t.Fatalf("total<defense must miss: %+v", roll)
	}
}

func TestRollAttack_CritThreshold(t *testing.T) {
	stats := repo.WeaponStats{ThreatLow: 18}
	for _, raw := range []int{17, 18, 19, 20} {
		rng := rand.New(rand.NewSource(stubSeedForRoll(raw)))
		roll := RollAttack(rng, newAttacker(10, 20), newDefender(0), stats, false)
		if !roll.Hit {
			t.Fatalf("raw=%d expected hit: %+v", raw, roll)
		}
		wantCrit := raw >= 18
		if roll.IsCrit != wantCrit {
			t.Fatalf("raw=%d crit=%v want=%v", raw, roll.IsCrit, wantCrit)
		}
	}
}

func TestRollDamage_AppliesStrModAndCritMult(t *testing.T) {
	stats := repo.WeaponStats{Damage: "1d1+0", CritMult: 3}
	rng := rand.New(rand.NewSource(1))
	atk := newAttacker(16, 0) // +3 Str mod
	dmg := RollDamage(rng, atk, stats, false)
	if dmg != 1+3 {
		t.Fatalf("non-crit dmg = %d, want %d", dmg, 4)
	}
	dmg = RollDamage(rng, atk, stats, true)
	if dmg != (1+3)*3 {
		t.Fatalf("crit dmg = %d, want %d", dmg, 12)
	}
}

func TestRollDamage_FloorOne(t *testing.T) {
	// Str 6 = -2 mod, "1d1" base = 1, total 1-2 = -1 → floored to 1.
	stats := repo.WeaponStats{Damage: "1d1"}
	rng := rand.New(rand.NewSource(1))
	atk := newAttacker(6, 0)
	if dmg := RollDamage(rng, atk, stats, false); dmg < 1 {
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
