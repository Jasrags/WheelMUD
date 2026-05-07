package combat

import (
	"math/rand"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// TestRollAttack_FlatFootedReducesDefense forces a borderline roll
// and asserts the FlatFooted gate flips a miss into a hit by
// removing the defender's positive Dex bonus from effective Defense.
func TestRollAttack_FlatFootedReducesDefense(t *testing.T) {
	// Defender has Dex 16 (+3 mod) and Defense 18 (the +3 already
	// baked in). Attacker rolls 14 + 0 BAB + 0 Str = 14.
	defender := creature.Core{
		Defense: 18,
		Abilities: creature.Abilities{
			Dex: creature.AbilityScore{Current: 16},
		},
	}
	attacker := newAttacker(10, 0)

	rng := rand.New(rand.NewSource(stubSeedForRoll(14)))
	roll := RollAttack(rng, attacker, defender, repo.WeaponStats{}, false)
	if roll.Hit {
		t.Fatalf("baseline 14 vs Defense 18 should miss: %+v", roll)
	}

	// Same inputs, FlatFooted=true → effective Defense drops by Dex
	// mod (3) to 15; 14 still misses.
	rng = rand.New(rand.NewSource(stubSeedForRoll(14)))
	roll = RollAttack(rng, attacker, defender, repo.WeaponStats{}, true)
	if roll.Hit {
		t.Fatalf("flat-footed 14 vs effective 15 should still miss: %+v", roll)
	}

	// Roll 15: baseline still misses (15 < 18), flat-footed hits
	// (15 >= 15).
	rng = rand.New(rand.NewSource(stubSeedForRoll(15)))
	if r := RollAttack(rng, attacker, defender, repo.WeaponStats{}, false); r.Hit {
		t.Fatalf("baseline 15 vs Defense 18 should miss: %+v", r)
	}
	rng = rand.New(rand.NewSource(stubSeedForRoll(15)))
	if r := RollAttack(rng, attacker, defender, repo.WeaponStats{}, true); !r.Hit {
		t.Fatalf("flat-footed 15 vs effective 15 should hit: %+v", r)
	}
}

// TestEffectiveDefense_NegativeDexNotDoubled verifies a defender
// with negative DexMod doesn't get *more* defense penalty when
// flat-footed — the negative mod is already in Defense at build
// time, so the gate only subtracts positive Dex.
func TestEffectiveDefense_NegativeDexNotDoubled(t *testing.T) {
	def := creature.Core{
		Defense: 10,
		Abilities: creature.Abilities{
			Dex: creature.AbilityScore{Current: 6}, // -2 mod
		},
	}
	if got := effectiveDefense(def, true); got != 10 {
		t.Fatalf("flat-footed with negative Dex: got %d, want 10", got)
	}
}

// TestRollParry_RangeAndComponents asserts the parry roll uses
// d20 + BAB + DexMod. Sweep d20 outcomes and check the total math.
func TestRollParry_RangeAndComponents(t *testing.T) {
	defender := creature.Core{
		BAB: 5,
		Abilities: creature.Abilities{
			Dex: creature.AbilityScore{Current: 14}, // +2 mod
		},
	}
	for raw := 1; raw <= 20; raw++ {
		rng := rand.New(rand.NewSource(stubSeedForRoll(raw)))
		got := RollParry(rng, defender)
		want := raw + 5 + 2
		if got != want {
			t.Fatalf("raw=%d RollParry=%d want=%d", raw, got, want)
		}
	}
}
