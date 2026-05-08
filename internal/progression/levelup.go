package progression

// levelup.go ships the per-level recompute formulas + the
// ComputeLevelUp entry point that the cmd-layer `train` verb calls
// before persisting via CharacterRepo.RecordLevelUp.
//
// Pure functions only — no DB, no session, no goroutines. The
// catalog dependency is read-only.

import (
	"errors"
	"fmt"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// ErrUnknownClass is returned when a level-up references a class id
// that isn't in the live chargen catalog. The cmd-layer translates
// this into a player-facing refusal — there's no partial state.
var ErrUnknownClass = errors.New("progression: unknown class")

// AbilityModifier returns the d20 ability modifier floor((s-10)/2),
// adjusted so odd scores below 10 round down (9 → -1, not 0). Mirrors
// internal/mode/character_create.go::abilityModifier; hoisting both
// to a shared helper is a deferred cleanup.
func AbilityModifier(score int) int {
	diff := score - 10
	if diff < 0 && diff%2 != 0 {
		return (diff - 1) / 2
	}
	return diff / 2
}

// HPDelta returns the deterministic HP gained on a level-up: average
// HitDie roll rounded up + Con modifier, floored at +1. d10 = +6, d8
// = +5, d6 = +4, d4 = +3 before Con. (Average rounded up: ceil(n/2)
// + 1; for an even die n, that's n/2 + 1.)
func HPDelta(hitDie int, conMod int) int32 {
	if hitDie < 1 {
		hitDie = 1
	}
	avgUp := hitDie/2 + 1
	if hitDie%2 == 1 {
		// Odd dies (rare; not used by the V1 catalog) round up:
		// ceil(n/2) = (n+1)/2.
		avgUp = (hitDie+1)/2 + 1
	}
	delta := avgUp + conMod
	if delta < 1 {
		return 1
	}
	return int32(delta)
}

// babPerClass returns the cumulative BAB at level n for one class
// progression. high = n, medium = n*3/4 (floor), low = n/2 (floor).
func babPerClass(prog chargen.BABProgression, n int) int {
	if n <= 0 {
		return 0
	}
	switch prog {
	case chargen.BABHigh:
		return n
	case chargen.BABMedium:
		return n * 3 / 4
	case chargen.BABLow:
		return n / 2
	}
	return 0
}

// savePerClass returns the cumulative base save at level n for one
// class progression. high = 2 + n/2 (floor), low = n/3 (floor).
func savePerClass(prog chargen.SaveProgression, n int) int {
	if n <= 0 {
		return 0
	}
	if prog == chargen.SaveHigh {
		return 2 + n/2
	}
	return n / 3
}

// BABTotal sums the BAB contribution from every class in
// classLevels. Each class contributes its progression evaluated at
// its own class-level (not character-level) — the d20 multiclass
// rule.
func BABTotal(classLevels map[creature.Class]int8, cat *chargen.Catalog) int16 {
	if cat == nil {
		return 0
	}
	total := 0
	for k, n := range classLevels {
		cl := classByEnum(cat, k)
		if cl == nil {
			continue
		}
		total += babPerClass(cl.BAB, int(n))
	}
	return int16(total)
}

// SaveTotal sums per-class base save contributions plus the
// character-wide ability modifier (which is added once, NOT per
// class). pickSave selects which save progression to read off each
// class — pass SaveFortOf / SaveRefOf / SaveWillOf.
func SaveTotal(classLevels map[creature.Class]int8, cat *chargen.Catalog,
	pickSave func(*chargen.Class) chargen.SaveProgression, abilityMod int) int16 {
	if cat == nil {
		return int16(abilityMod)
	}
	base := 0
	for k, n := range classLevels {
		cl := classByEnum(cat, k)
		if cl == nil {
			continue
		}
		base += savePerClass(pickSave(cl), int(n))
	}
	return int16(base + abilityMod)
}

// SaveFortOf / SaveRefOf / SaveWillOf are picker helpers passed to
// SaveTotal so callers don't need closures at every call site.
func SaveFortOf(c *chargen.Class) chargen.SaveProgression { return c.SaveFort }
func SaveRefOf(c *chargen.Class) chargen.SaveProgression  { return c.SaveRef }
func SaveWillOf(c *chargen.Class) chargen.SaveProgression { return c.SaveWill }

// LevelGains is the immutable result of a level-up computation. The
// caller persists ClassLevels + the four Core fields + the pending-
// pool deltas atomically via CharacterRepo.RecordLevelUp (mapping
// shape onto repo.LevelUpFields).
type LevelGains struct {
	NewHPCurrent int32
	NewHPMax     int32
	NewBAB       int16
	NewSaves     creature.Saves
	ClassLevels  map[creature.Class]int8
	HPDelta      int32 // for the player-facing line
	NewLevel     int8  // ClassLevels[classKey] after the bump

	// Per-pool deltas deposited at this level-up. Phase E #23 slice
	// 4. The cmd-layer renders any non-zero values into the train
	// success line and forwards them to RecordLevelUp where they
	// `pending_x += delta` on the row.
	FeatDelta     int32 // +1 when NewLevel%3==0
	SkillDelta    int32 // max(1, class.SkillPoints + IntMod)
	AbilityDelta  int32 // +1 when NewLevel%4==0
	WeaveDelta    int32 // +1 for channeler classes, else 0
	PracticeDelta int32 // +1 every level (Phase E #28 — mid-game weave currency)
}

// ComputeLevelUp returns the LevelGains for advancing classKey by
// one level on ch. Pure: ch is not mutated; the returned ClassLevels
// map is a fresh copy. Returns ErrUnknownClass when classKey isn't
// in the chargen catalog (typo or stale content).
//
// HP semantics: MaxHP grows by HPDelta; HPCurrent grows by the same
// delta but is capped at the new MaxHP — level-up does NOT fully
// heal. A character at 1 HP who levels up gets 1 + delta HP; a
// character at full HP stays at the new full.
func ComputeLevelUp(ch repo.Character, cat *chargen.Catalog,
	classKey creature.Class) (LevelGains, error) {
	if cat == nil {
		return LevelGains{}, ErrUnknownClass
	}
	cl := classByEnum(cat, classKey)
	if cl == nil {
		return LevelGains{}, fmt.Errorf("%w: %v", ErrUnknownClass, classKey)
	}

	// Build the new ClassLevels map with classKey bumped by 1.
	newLevels := make(map[creature.Class]int8, len(ch.ClassLevels)+1)
	for k, v := range ch.ClassLevels {
		newLevels[k] = v
	}
	newLevels[classKey] = newLevels[classKey] + 1

	conMod := AbilityModifier(int(ch.Core.Abilities.Con.Current))
	dexMod := AbilityModifier(int(ch.Core.Abilities.Dex.Current))
	wisMod := AbilityModifier(int(ch.Core.Abilities.Wis.Current))
	intMod := AbilityModifier(int(ch.Core.Abilities.Int.Current))

	hpDelta := HPDelta(cl.HitDie, conMod)
	newMax := ch.Core.HPMax + hpDelta
	newCur := ch.Core.HPCurrent + hpDelta
	if newCur > newMax {
		newCur = newMax
	}

	bab := BABTotal(newLevels, cat)
	saves := creature.Saves{
		Fort: SaveTotal(newLevels, cat, SaveFortOf, conMod),
		Ref:  SaveTotal(newLevels, cat, SaveRefOf, dexMod),
		Will: SaveTotal(newLevels, cat, SaveWillOf, wisMod),
	}

	newLevel := newLevels[classKey]
	skillDelta := int32(cl.SkillPoints) + int32(intMod)
	if skillDelta < 1 {
		skillDelta = 1
	}
	var featDelta, abilityDelta, weaveDelta int32
	if newLevel%3 == 0 {
		featDelta = 1
	}
	if newLevel%4 == 0 {
		abilityDelta = 1
	}
	if cl.Channeler {
		weaveDelta = 1
	}

	return LevelGains{
		NewHPCurrent:  newCur,
		NewHPMax:      newMax,
		NewBAB:        bab,
		NewSaves:      saves,
		ClassLevels:   newLevels,
		HPDelta:       hpDelta,
		NewLevel:      newLevel,
		FeatDelta:     featDelta,
		SkillDelta:    skillDelta,
		AbilityDelta:  abilityDelta,
		WeaveDelta:    weaveDelta,
		PracticeDelta: 1,
	}, nil
}

// classByEnum returns the chargen.Class whose Enum field matches k,
// or nil when no class in the catalog maps to k. Linear over a tiny
// slice — the V1 catalog has 7 classes.
func classByEnum(cat *chargen.Catalog, k creature.Class) *chargen.Class {
	for _, c := range cat.Classes() {
		if c.Enum == k {
			return c
		}
	}
	return nil
}
