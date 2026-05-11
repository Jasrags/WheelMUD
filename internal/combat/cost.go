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

// ApplySpeedFactor multiplies a duration by a per-actor speed
// multiplier. Phase L slice 63 — the racial cadence pass folds the
// RaceProfile's SpeedFactor on top of the gear-derived cost so an
// Ogier's 1.2× tax stacks with their armor and weapon weights.
//
// speedFactor <= 0 returns d unchanged so a missing / zero profile
// can't accidentally collapse cadence to zero or make it negative.
func ApplySpeedFactor(d time.Duration, speedFactor float32) time.Duration {
	if speedFactor <= 0 {
		return d
	}
	return time.Duration(float64(d) * float64(speedFactor))
}

// EffectiveStaminaRegen returns the per-pulse stamina regen after
// the heavy-armor penalty. Phase L slice 63 — wearing heavy body
// armor halves regen (rounded down, floored at 1 when the base is
// positive so a regen of 1 doesn't disappear entirely). Light /
// medium / no armor return base unchanged. Pure; no I/O.
func EffectiveStaminaRegen(base int32, armorWeightClass string) int32 {
	if base <= 0 {
		return base
	}
	if armorWeightClass != "heavy" {
		return base
	}
	half := base / 2
	if half < 1 {
		return 1
	}
	return half
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
//
// ArmorWeightClass carries the canonical string token ("light" /
// "medium" / "heavy") for the worn body armor so downstream consumers
// (score's stamina-regen hint, future feat / status displays) can
// branch on the source value rather than re-inferring it from
// ArmorFactor. Empty when no body armor is worn.
type GearFactors struct {
	WeaponFactor     float64
	ArmorFactor      float64
	ArmorWeightClass string
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
				g.ArmorWeightClass = as.WeightClass
			}
		} else {
			slog.Warn("combat: armor lookup failed; treating as unarmored",
				"item_id", aid, "err", err)
		}
	}
	return g
}

// actorActionCost combines DefaultActionCost with the actor's gear
// factors, feat-driven attenuations, and racial speed multiplier.
// Called from tickRoom after a swing resolves to stamp NextActAt.
// resolveAction-time gear is "as of now" — re-wielding mid-fight
// changes the cadence of the next queued action, matching how combat
// verbs already re-read weapon stats by id. Feats are looked up via
// the chargen catalog every call: combat is ~1Hz and the catalog
// lookup is a single map read, so caching on Core would add
// invalidation complexity for negligible benefit.
//
// Order matters: feats attenuate the gear factors before the racial
// speed multiplier folds in. Stacking on the racial side would let
// an Aiel-with-Light-Step double-discount armor in a way the score
// sheet's bracketed contributors would no longer reflect.
func (m *Manager) actorActionCost(ctx context.Context, ref ActorRef, action Action) time.Duration {
	base := DefaultActionCost(action.Kind, action.Variant)
	cost := base
	fm := m.resolveFeatModifiers(ctx, ref)
	if eq, ok := m.resolveEquipment(ctx, ref); ok {
		g := ResolveGearFactors(ctx, m.items, eq)
		g = ApplyFeatGearAttenuation(g, fm)
		cost = time.Duration(float64(cost) * g.Multiplier())
	}
	cost = ApplySpeedFactor(cost, m.actorSpeedFactor(ctx, ref))
	return cost
}

// resolveFeatModifiers returns the feat-driven cadence aggregate for
// the actor. Mob actors and characters with no catalog wired both
// return the identity aggregate so combat math is well-defined under
// every configuration. Phase L slice 65.
func (m *Manager) resolveFeatModifiers(ctx context.Context, ref ActorRef) FeatModifiers {
	if ref.Kind != ActorKindCharacter || m.chars == nil || m.cat == nil {
		return IdentityFeatModifiers()
	}
	ch, err := m.chars.GetByID(ctx, ref.ID)
	if err != nil {
		return IdentityFeatModifiers()
	}
	return ResolveFeatModifiers(ch.Feats, m.cat)
}

// actorSpeedFactor returns the RaceProfile.SpeedFactor for the actor,
// or 1.0 when the actor isn't a character or the lookup fails. Mob
// SpeedFactor is intentionally 1.0 today — racial cadence is a
// player-side mechanic in V1.
func (m *Manager) actorSpeedFactor(ctx context.Context, ref ActorRef) float32 {
	if ref.Kind != ActorKindCharacter || m.chars == nil {
		return 1.0
	}
	ch, err := m.chars.GetByID(ctx, ref.ID)
	if err != nil {
		return 1.0
	}
	return creature.ProfileFor(ch.Race).SpeedFactor
}

// drainStamina deducts the action's stamina cost from a character
// actor's pool. Mob actors and zero-cost kinds short-circuit. Repo
// failures log-and-continue rather than abort the swing — combat
// must keep flowing even if one stamina write hits a transient
// error. Phase L slice 63.
func (m *Manager) drainStamina(ctx context.Context, ref ActorRef, action Action) {
	if ref.Kind != ActorKindCharacter || m.chars == nil {
		return
	}
	cost := DefaultActionStamina(action.Kind, action.Variant)
	if cost <= 0 {
		return
	}
	ch, err := m.chars.GetByID(ctx, ref.ID)
	if err != nil {
		slog.Warn("combat: stamina load failed", "char", ref.ID, "error", err)
		return
	}
	// Skip drain on unconfigured pools (StaminaMax == 0). Mirrors the
	// EnqueueAction gate so pre-0049 characters and test fixtures
	// stay unmetered.
	if ch.Core.StaminaMax <= 0 {
		return
	}
	// Phase L slice 65: feat-driven cost-mul (Endurance = 0.8×).
	// Identity mul = 1.0 → no change. Floor at 0 so a stacked discount
	// can't credit stamina back to the actor; round to nearest so
	// integer SP costs don't all collapse to the same value.
	if m.cat != nil {
		fm := ResolveFeatModifiers(ch.Feats, m.cat)
		if fm.StaminaCostMul > 0 && fm.StaminaCostMul != 1.0 {
			scaled := float32(cost) * fm.StaminaCostMul
			if scaled < 0 {
				scaled = 0
			}
			cost = int32(scaled + 0.5)
		}
	}
	next := ch.Core.StaminaCurrent - cost
	if next < 0 {
		next = 0
	}
	if err := m.chars.RecordStamina(ctx, ref.ID, next); err != nil {
		slog.Warn("combat: stamina write failed", "char", ref.ID, "error", err)
	}
}

// hasStaminaFor is a non-mutating "can this actor afford the next
// swing" gate used by the iterative drain loop (Phase L #66). Mirrors
// drainStamina's cost calculation — DefaultActionStamina × feat
// StaminaCostMul, rounded, floored at 0 — but answers boolean instead
// of writing. Mobs (no character row) and unconfigured pools
// (StaminaMax <= 0) always return true so they swing freely; this
// matches drainStamina's early-return behavior so the gate and the
// drain agree on "did we pay anything."
func (m *Manager) hasStaminaFor(ctx context.Context, ref ActorRef, action Action) bool {
	if ref.Kind != ActorKindCharacter || m.chars == nil {
		return true
	}
	cost := DefaultActionStamina(action.Kind, action.Variant)
	if cost <= 0 {
		return true
	}
	ch, err := m.chars.GetByID(ctx, ref.ID)
	if err != nil {
		// Same fail-safe stance as drainStamina: a transient repo
		// hiccup shouldn't truncate the chain. Let the swing proceed;
		// drainStamina will log if the write also fails.
		return true
	}
	if ch.Core.StaminaMax <= 0 {
		return true
	}
	if m.cat != nil {
		fm := ResolveFeatModifiers(ch.Feats, m.cat)
		if fm.StaminaCostMul > 0 && fm.StaminaCostMul != 1.0 {
			scaled := float32(cost) * fm.StaminaCostMul
			if scaled < 0 {
				scaled = 0
			}
			cost = int32(scaled + 0.5)
		}
	}
	return ch.Core.StaminaCurrent >= cost
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
