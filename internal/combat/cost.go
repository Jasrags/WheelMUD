package combat

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// Phase L slice 62 — gear-driven cadence.
//
// ActionCost layers two multiplicative factors on top of the base
// (kind, variant) cost from DefaultActionCost: weapon weight from the
// item in SlotPrimaryWield, and armor weight class from the item in
// SlotArmor. Future slices (#63 race, #65 feats) add more factors;
// each factor stays a pure function so the score sheet and the combat
// resolver can't disagree.

// ActionCost returns the wall-clock cost of an action given a base
// (from DefaultActionCost) and the gear-derived factors. Pure; no I/O.
//
// weaponWeight is in pounds (matches repo.Item.Weight); 0 means
// unarmed. armorWeightClass is the lowercase string from
// repo.ArmorStats.WeightClass ("light" / "medium" / "heavy"); empty
// means no body armor worn.
func ActionCost(base time.Duration, weaponWeight float64, armorWeightClass string) time.Duration {
	factor := weaponSpeedFactor(weaponWeight) * armorSpeedFactor(armorWeightClass)
	return time.Duration(float64(base) * factor)
}

// weaponSpeedFactor maps weight in pounds onto a cadence multiplier.
// The bands match the WoT-d20 weapon table loosely: daggers / clubs
// at the low end, longswords / maces in the medium band, battleaxes
// and warhammers heavy, two-handed swords / polearms at the top.
// Unarmed sits between light and medium — fists are quick but lack
// reach, so we don't reward them as much as a light blade.
func weaponSpeedFactor(weightLb float64) float64 {
	switch {
	case weightLb <= 0:
		return 0.90 // unarmed
	case weightLb <= 2:
		return 0.80 // light
	case weightLb <= 10:
		return 1.00 // medium
	case weightLb <= 15:
		return 1.30 // heavy
	default:
		return 1.50 // two-handed
	}
}

// armorSpeedFactor maps the ArmorStats.WeightClass string onto a
// cadence multiplier. Empty / unknown values are treated as no armor
// (1.00×) so a missing stat block can't accidentally penalize a
// fighter. The unknown case logs once per call site over the lifetime
// of the process — combat is hot enough that we don't want a slog
// entry every swing.
func armorSpeedFactor(class string) float64 {
	switch class {
	case "", "none":
		return 1.00
	case "light":
		return 1.05
	case "medium":
		return 1.15
	case "heavy":
		return 1.30
	default:
		slog.Warn("combat: unknown armor weight class; treating as unarmored",
			"class", class)
		return 1.00
	}
}

// GearFactors holds the two cadence multipliers for an actor.
// Returned by ResolveGearFactors so the score sheet and the combat
// resolver share one code path. Zero value is "unarmed and naked"
// (1.0× cadence — no penalty, no bonus).
type GearFactors struct {
	WeaponFactor float64
	ArmorFactor  float64
}

// Multiplier is the combined factor applied to a base action cost.
func (g GearFactors) Multiplier() float64 { return g.WeaponFactor * g.ArmorFactor }

// ResolveGearFactors reads the primary-wield and armor slots and
// returns the cadence factors for the actor. Items repo is optional —
// nil yields the unarmed/naked baseline (used by tests). Lookup
// failures degrade to the same baseline rather than panicking combat.
//
// ctx is propagated to the repo calls; in combat-resolution paths it
// is the per-tick ctx, in score rendering it is the dispatcher ctx.
func ResolveGearFactors(ctx context.Context, items repo.ItemRepo, eq creature.Equipment) GearFactors {
	g := GearFactors{WeaponFactor: 0.90, ArmorFactor: 1.00}
	if items == nil {
		return g
	}
	if wid := eq.Get(creature.SlotPrimaryWield); wid != 0 {
		if it, err := items.GetByID(ctx, wid); err == nil {
			g.WeaponFactor = weaponSpeedFactor(it.Weight)
		} else {
			slog.Warn("combat: weapon lookup failed; treating as unarmed",
				"item_id", wid, "err", err)
		}
	}
	if aid := eq.Get(creature.SlotArmor); aid != 0 {
		if it, err := items.GetByID(ctx, aid); err == nil {
			if as, ok := it.Stats.(*repo.ArmorStats); ok {
				g.ArmorFactor = armorSpeedFactor(as.WeightClass)
			}
		} else {
			slog.Warn("combat: armor lookup failed; treating as unarmored",
				"item_id", aid, "err", err)
		}
	}
	return g
}

// actorActionCost combines DefaultActionCost with the actor's gear
// factors. Called from tickRoom after a swing resolves to stamp
// NextActAt. resolveAction-time gear is "as of now" — re-wielding
// mid-fight changes the cadence of the next queued action, matching
// how combat verbs already re-read weapon stats by id.
func (m *Manager) actorActionCost(ctx context.Context, ref ActorRef, action Action) time.Duration {
	base := DefaultActionCost(action.Kind, action.Variant)
	eq, ok := m.resolveEquipment(ctx, ref)
	if !ok {
		return base
	}
	g := ResolveGearFactors(ctx, m.items, eq)
	return time.Duration(float64(base) * g.Multiplier())
}

// resolveEquipment fetches the actor's current Equipment snapshot.
// Sibling to resolveCore; lookup errors degrade to the zero
// Equipment (treated as unarmed/naked by ResolveGearFactors).
func (m *Manager) resolveEquipment(ctx context.Context, ref ActorRef) (creature.Equipment, bool) {
	switch ref.Kind {
	case ActorKindCharacter:
		ch, err := m.chars.GetByID(ctx, ref.ID)
		if err != nil {
			return creature.Equipment{}, false
		}
		return ch.Equipment, true
	case ActorKindMob:
		mob, err := m.mobs.GetByID(ctx, ref.ID)
		if err != nil {
			return creature.Equipment{}, false
		}
		return mob.Equipment, true
	}
	return creature.Equipment{}, false
}
