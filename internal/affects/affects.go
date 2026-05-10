// Package affects models timed buffs/debuffs on a creature.Core. All
// functions are pure: they never mutate their inputs. Combat reads
// Effective(core) to fold modifier deltas into the rolling Core; the
// per-tick driver calls Tick to count durations down; sources call
// Apply to add/refresh an entry.
//
// V1 (Phase E #25) ships stat-mods only. TickEffect dispatch (poison /
// bleed DOTs) is deferred until a content source needs it; see the
// plan file for the full deferred-followups list.
package affects

import "github.com/Jasrags/WheelMUD/internal/creature"

// TickSeconds is the wall-clock cadence at which the SessionTicker
// pulses. Subscribers in cmd/server/main.go bind the ticker to
// tick.Buckets.Affects (6s); the constant is the single source of
// truth so the `affects` verb's duration readout stays in sync if the
// bucket binding changes.
const TickSeconds = 6

// MaxAffectsPerName caps the number of distinct-Source affects that
// can share a Name. When a new Source pushes the count past the cap
// Apply evicts the entry with the shortest remaining DurationTicks
// (FIFO-ish — the oldest pulse loses). Same-(Source, Name) refresh
// is unaffected by the cap.
const MaxAffectsPerName = 4

// AllowedStatModFields enumerates the field names accepted by Apply /
// Effective. Used by the `affect` admin verb's argument parser to
// reject typos with a useful hint instead of silently no-opping in
// applyMod's switch.
var AllowedStatModFields = []string{
	FieldStrCurrent,
	FieldDexCurrent,
	FieldConCurrent,
	FieldIntCurrent,
	FieldWisCurrent,
	FieldChaCurrent,
	FieldDefense,
	FieldSavesFort,
	FieldSavesRef,
	FieldSavesWill,
	FieldSpeedBase,
	FieldBAB,
}

// StatMod field names. Centralised so callers and tests use the same
// strings — typos in StatMod.Field silently no-op in Effective.
const (
	FieldStrCurrent = "Str.Current"
	FieldDexCurrent = "Dex.Current"
	FieldConCurrent = "Con.Current"
	FieldIntCurrent = "Int.Current"
	FieldWisCurrent = "Wis.Current"
	FieldChaCurrent = "Cha.Current"
	FieldDefense    = "Defense"
	FieldSavesFort  = "Saves.Fort"
	FieldSavesRef   = "Saves.Ref"
	FieldSavesWill  = "Saves.Will"
	FieldSpeedBase  = "Speed.BaseFt"
	FieldBAB        = "BAB"
)

// Effective returns a copy of c with every Affect.Modifier folded into
// the matching numeric field, and every Affect.ConditionMask OR'd
// into Conditions. Combat passes the result of this fn into
// RollAttack/RollDamage/applyDamage so the math reads through-affect
// values. The original c is not mutated.
//
// Zero-affect fast path: returns c unchanged (no allocation).
//
// Unknown StatMod.Field values are ignored — callers can introduce new
// fields ahead of the consumer side without breaking older builds.
func Effective(c creature.Core) creature.Core {
	if len(c.Affects) == 0 {
		return c
	}
	out := c
	for _, a := range c.Affects {
		for _, m := range a.Modifiers {
			applyMod(&out, m)
		}
		out.Conditions |= a.ConditionMask
	}
	return out
}

func applyMod(c *creature.Core, m creature.StatMod) {
	d := int16(m.Delta)
	switch m.Field {
	case FieldStrCurrent:
		c.Abilities.Str.Current = clampInt8(int16(c.Abilities.Str.Current) + d)
	case FieldDexCurrent:
		c.Abilities.Dex.Current = clampInt8(int16(c.Abilities.Dex.Current) + d)
	case FieldConCurrent:
		c.Abilities.Con.Current = clampInt8(int16(c.Abilities.Con.Current) + d)
	case FieldIntCurrent:
		c.Abilities.Int.Current = clampInt8(int16(c.Abilities.Int.Current) + d)
	case FieldWisCurrent:
		c.Abilities.Wis.Current = clampInt8(int16(c.Abilities.Wis.Current) + d)
	case FieldChaCurrent:
		c.Abilities.Cha.Current = clampInt8(int16(c.Abilities.Cha.Current) + d)
	case FieldDefense:
		c.Defense += d
	case FieldSavesFort:
		c.Saves.Fort += d
	case FieldSavesRef:
		c.Saves.Ref += d
	case FieldSavesWill:
		c.Saves.Will += d
	case FieldSpeedBase:
		v := c.Speed.BaseFt + d
		if v < 0 {
			v = 0
		}
		c.Speed.BaseFt = v
	case FieldBAB:
		c.BAB += d
	}
}

func clampInt8(v int16) int8 {
	switch {
	case v < -128:
		return -128
	case v > 127:
		return 127
	default:
		return int8(v)
	}
}

// Tick decrements every Affect's DurationTicks by 1 and drops affects
// whose duration drops to <= 0. Returns (newAffects, expiredNames). An
// affect with DurationTicks == 0 on input is treated as expired and
// dropped (it never had any duration left to begin with).
//
// out preserves input order for non-expired entries. Callers detect
// "did anything change" via len(expired) > 0 || len(out) != len(in).
//
// Returns (nil, nil) for an empty input — allocation only happens when
// at least one affect is dropped or carried forward.
func Tick(in []creature.Affect) (out []creature.Affect, expired []string) {
	if len(in) == 0 {
		return nil, nil
	}
	out = make([]creature.Affect, 0, len(in))
	for _, a := range in {
		a.DurationTicks--
		if a.DurationTicks <= 0 {
			expired = append(expired, a.Name)
			continue
		}
		out = append(out, a)
	}
	return out, expired
}

// Apply adds a new affect or refreshes an existing one keyed by
// (Source, Name). The new affect REPLACES the prior entry — longer
// durations don't merge, the latest cast wins. Returns a new slice;
// the input is never mutated.
//
// (Source, Name) dedup means:
//   - distinct Sources with the same Name coexist (two casters' poisons
//     stack as separate entries) up to MaxAffectsPerName.
//   - distinct Names from the same Source coexist (one item proccing
//     "blessed" and "shielded").
//   - identical (Source, Name) — the new one wins outright.
//
// Stacking cap: when MaxAffectsPerName entries already share a.Name
// from different Sources, the entry with the lowest remaining
// DurationTicks is evicted to make room for the new one. The refresh
// path bypasses the cap (replacing your own affect can't push you
// over).
func Apply(in []creature.Affect, a creature.Affect) []creature.Affect {
	for i, prev := range in {
		if prev.Source != a.Source || prev.Name != a.Name {
			continue
		}
		out := make([]creature.Affect, len(in))
		copy(out, in)
		out[i] = a
		return out
	}
	// New (Source, Name) pair. Enforce cap on shared Name before
	// appending.
	out := make([]creature.Affect, len(in), len(in)+1)
	copy(out, in)
	out = evictForName(out, a.Name)
	out = append(out, a)
	return out
}

// evictForName drops the lowest-DurationTicks entry sharing name when
// the slice already holds MaxAffectsPerName of them. Returns the
// (possibly shorter) slice; the input slice may be mutated in place
// (it is the caller-owned working copy from Apply).
func evictForName(in []creature.Affect, name string) []creature.Affect {
	count := 0
	for _, a := range in {
		if a.Name == name {
			count++
		}
	}
	if count < MaxAffectsPerName {
		return in
	}
	victim := -1
	var minDur int32
	for i, a := range in {
		if a.Name != name {
			continue
		}
		if victim == -1 || a.DurationTicks < minDur {
			victim = i
			minDur = a.DurationTicks
		}
	}
	if victim == -1 {
		return in
	}
	return append(in[:victim], in[victim+1:]...)
}
