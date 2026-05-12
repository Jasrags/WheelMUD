package combat

import (
	"github.com/Jasrags/WheelMUD/internal/chargen"
)

// Phase L slice 65 — feats that modify cadence.
//
// FeatModifiers is the multiplicative aggregate of a character's
// cadence-affecting feats. The struct is pure data; ResolveFeatModifiers
// walks creature.Core.Feats (a []int32 of FNV-32a hashes — see
// chargen.HashID) and stacks the per-feat fields. Zero value of the
// returned struct is the identity (no feats → no change).
//
// Why a layered struct instead of separate per-call functions: the
// combat hot path (actorActionCost, drainStamina, StaminaTicker)
// needs three different fields off the same set of feats. Resolving
// once per pulse and threading the struct through is cheaper than
// re-walking core.Feats three times, and unit-testing the resolver
// stays straightforward.

// FeatModifiers carries the aggregated cadence-modifier fields for an
// actor. Identity values: *Mul = 1.0, StaminaRegenAdd = 0. Active is
// the human-readable list of contributing feat names; the score
// sheet renders these so players can see which feats are firing.
type FeatModifiers struct {
	WeaponWeightPenaltyMul float32
	ArmorWeightPenaltyMul  float32
	StaminaCostMul         float32
	StaminaRegenAdd        int16
	// OffHandCostMul scales the cadence cost of off-hand swings only
	// (Phase D slice 4). Identity 1.0; <1.0 speeds the off-hand
	// chain. feat_two_weapon_grace is the seed consumer.
	OffHandCostMul float32
	Active         []string
}

// IdentityFeatModifiers returns the no-op aggregate. Used by callers
// that need an explicit baseline rather than the zero value (the zero
// value has 0.0 multipliers which would zero out durations; methods on
// FeatModifiers substitute 1.0 internally, but the explicit baseline
// is clearer at call sites).
func IdentityFeatModifiers() FeatModifiers {
	return FeatModifiers{
		WeaponWeightPenaltyMul: 1.0,
		ArmorWeightPenaltyMul:  1.0,
		StaminaCostMul:         1.0,
		OffHandCostMul:         1.0,
	}
}

// ResolveFeatModifiers walks the hashed feat list, looks each up via
// the catalog's reverse index, and accumulates the cadence fields.
// nil catalog or nil/empty feats slice returns the identity aggregate
// — combat must keep working even when chargen content fails to load
// in tests. Unknown hashes (a stale feat removed from the catalog
// between sessions) are silently skipped; the spend verb is the
// authoritative entry point for catalog churn.
func ResolveFeatModifiers(feats []int32, cat *chargen.Catalog) FeatModifiers {
	fm := IdentityFeatModifiers()
	if cat == nil || len(feats) == 0 {
		return fm
	}
	for _, key := range feats {
		f := cat.FeatByHashedID(key)
		if f == nil {
			continue
		}
		contributes := false
		if f.WeaponWeightPenaltyMul != 0 {
			fm.WeaponWeightPenaltyMul *= f.WeaponWeightPenaltyMul
			contributes = true
		}
		if f.ArmorWeightPenaltyMul != 0 {
			fm.ArmorWeightPenaltyMul *= f.ArmorWeightPenaltyMul
			contributes = true
		}
		if f.StaminaCostMul != 0 {
			fm.StaminaCostMul *= f.StaminaCostMul
			contributes = true
		}
		if f.StaminaRegenAdd != 0 {
			fm.StaminaRegenAdd += f.StaminaRegenAdd
			contributes = true
		}
		if f.OffHandCostMul != 0 {
			fm.OffHandCostMul *= f.OffHandCostMul
			contributes = true
		}
		if contributes {
			fm.Active = append(fm.Active, f.Name)
		}
	}
	return fm
}

// ApplyFeatGearAttenuation rewrites a GearFactors so the "penalty
// portion" of each factor (the slice above 1.0) is multiplied by the
// matching feat modifier. Bonus factors (below 1.0 — light weapon,
// no armor) pass through unchanged so Blademaster doesn't hand a
// dagger-wielder a second discount on top of the dagger's existing
// 0.8× factor.
//
// Pure function — no I/O, no mutation of g (returns a new copy).
// Defensive against zero multipliers (treat as 1.0 — the identity).
func ApplyFeatGearAttenuation(g GearFactors, fm FeatModifiers) GearFactors {
	out := g
	out.WeaponFactor = attenuatePenalty(g.WeaponFactor, fm.WeaponWeightPenaltyMul)
	out.ArmorFactor = attenuatePenalty(g.ArmorFactor, fm.ArmorWeightPenaltyMul)
	return out
}

// attenuatePenalty halves a factor's distance above 1.0 by mul. A
// factor at or below 1.0 returns unchanged; a zero mul is the
// identity (returns f). Pure helper, no I/O.
func attenuatePenalty(f float64, mul float32) float64 {
	if f <= 1.0 {
		return f
	}
	if mul <= 0 {
		return f
	}
	return 1.0 + (f-1.0)*float64(mul)
}
