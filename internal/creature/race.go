package creature

// Phase L slice 63 — racial speed + stamina pool.
//
// RaceProfile is the per-race trio of cadence + stamina parameters
// the combat manager and the Regen ticker read. Kept as a pure-Go
// table (rather than a YAML race catalog) because the live race
// roster is a 2-value enum and a full chargen-style content surface
// would be scope creep. When new races land — Aiel / Trolloc /
// Myrddraal per docs/PLAN.md — extend the switch in ProfileFor with
// the seed values and add the new constant to the Race enum.

// RaceProfile carries the three values combat reads off Race:
//
//   - SpeedFactor multiplies the base+gear action cost in
//     combat.actorActionCost. <1.0 is faster, >1.0 is slower. The
//     PLAN seeds Human at 1.0 and Ogier at 1.2.
//
//   - StaminaMax is the pool size at chargen finalize.
//
//   - StaminaRegen is the per-pulse refill applied by the Regen
//     ticker (combat.StaminaTicker). A heavy-armor wearer regens
//     half as fast — see EffectiveStaminaRegen in internal/combat.
type RaceProfile struct {
	SpeedFactor  float32
	StaminaMax   int32
	StaminaRegen int32
}

// DefaultRaceProfile is the fallback when no race-specific entry
// exists (or the caller has a zero Race that isn't yet enumerated).
// Treats the actor as Human-baseline so unmapped data never
// accidentally penalizes anyone.
var DefaultRaceProfile = RaceProfile{
	SpeedFactor:  1.0,
	StaminaMax:   100,
	StaminaRegen: 2,
}

// ProfileFor returns the RaceProfile for r. Unknown races return
// DefaultRaceProfile rather than a zero value so a missing entry
// can't silently zero out a character's stamina pool.
func ProfileFor(r Race) RaceProfile {
	switch r {
	case RaceHuman:
		return RaceProfile{SpeedFactor: 1.0, StaminaMax: 100, StaminaRegen: 2}
	case RaceOgier:
		return RaceProfile{SpeedFactor: 1.2, StaminaMax: 150, StaminaRegen: 1}
	}
	return DefaultRaceProfile
}
