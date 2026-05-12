package combat

import (
	"sort"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// SwingPlan is one entry in the per-pulse swing schedule. Slot is the
// equipment slot the swing is sourced from (SlotPrimaryWield or
// SlotOffHand); Bonus is the attack-roll delta applied to that swing
// (iterative penalty plus the TWF -4 baked in for off-hand swings).
//
// The schedule is pinned at Start so a mid-fight wield change does
// not retroactively alter the per-pulse swing count, mirroring how
// IterativeBonusesFor used to pin off creature.BAB at Start.
type SwingPlan struct {
	Slot  creature.Slot
	Bonus int16
}

// ActorEntry is one combatant's slot in the initiative order.
// Initiative is the resolved roll (d20 + DexMod + InitMod). Tiebreak
// is the raw d20 captured at Start so a stable resort after a
// mid-fight join produces the same order — ties resolve by Tiebreak
// then by ActorRef.ID.
//
// NextActAt is the wall-clock instant at which this actor is next
// allowed to resolve a queued action. tickRoom walks the order and
// fires every entry whose NextActAt <= now. Initialized to
// Fight.StartedAt at Start so everyone is ready on the opening pulse
// — initiative still orders the within-pulse fan-out.
//
// LastActedAt is debug/test surface: snapshots the time of the
// previous resolution. Lets a test assert "Aiel acted 3× while
// Trolloc acted 1×" by counting forward-progress on this field.
type ActorEntry struct {
	Ref         ActorRef
	Initiative  int   // d20 + DexMod + InitMod
	Tiebreak    int32 // raw d20 captured at Start
	NextActAt   time.Time
	LastActedAt time.Time

	// PendingSwings is the per-pulse swing count for this actor.
	// Equals len(Swings). For a single-wielder this is just the
	// iterative chain (1 / 2 / 3 / 4 by BAB tier). A dual-wielder
	// doubles the count — see Swings.
	PendingSwings int

	// Swings is the per-pulse swing schedule. The primary chain comes
	// first, followed by the off-hand chain when the actor has a
	// weapon in SlotOffHand at Start. Pinned at Start; mid-fight BAB
	// shifts or off-hand wield changes do not retroactively rewrite
	// the schedule (matching the "Initiative pinned at Start"
	// precedent). The per-swing slot is consumed by resolveAction to
	// pick which equipment slot's weapon stats apply.
	Swings []SwingPlan
}

// Fight is the per-room combat aggregate. One Fight per RoomID at a
// time — the Manager rejects double-Start. Order is sorted descending
// by Initiative on Start and stays stable through the rest of the
// fight; per-actor cadence (NextActAt on each entry) decides who
// resolves on any given pulse.
//
// Round is a monotonically-incrementing per-fight act counter — ++'s
// every time an actor resolves an action (any kind). Used by
// stance-bookkeeping maps (ParryingUntil / FlatFootedUntil) keyed by
// "next act" semantics; consumers compare entries to the current
// round at attack-resolution time.
type Fight struct {
	RoomID    int64
	Round     int          // per-act counter; 0 = not started
	Order     []ActorEntry // sorted descending by initiative
	StartedAt time.Time

	// Actions is the per-actor queue of intents. Lazily initialized
	// by EnqueueAction. Resolved + cleared by the Manager's Tick when
	// the owning actor's NextActAt fires.
	Actions map[ActorRef]Action

	// DamageTally accumulates the total post-DR/post-resist damage
	// each actor has dealt over the lifetime of the fight. Used to
	// allocate XP weighted by contribution when a mob dies. Lazily
	// initialized in resolveAction.
	DamageTally map[ActorRef]int32

	// Dead is the set of actors that have hit HP ≤ 0 inside this
	// fight. Populated by the death handler; consumed by tickRoom at
	// the top of the next pulse to prune Order before resolution.
	// Mutating Order in-flight inside resolveAction would shift the
	// iteration under the Tick caller, so the prune is deferred.
	Dead map[ActorRef]struct{}

	// Fled mirrors Dead for actors that successfully retreated via
	// ActionFlee. pruneDead removes both sets in one pass — the
	// distinction exists so death-only consumers (XP allocation,
	// corpse spawn) don't fire for fleeing actors.
	Fled map[ActorRef]struct{}

	// ParryingUntil maps an actor to the round number through which
	// their parrying stance is active. Set by ActionParry resolution
	// to `currentRound + 1` so the stance covers the very next
	// incoming swing (the next actor-act-round); cleared on
	// consumption (a successful opposed roll) or when the round
	// counter passes.
	ParryingUntil map[ActorRef]int

	// FlatFootedUntil maps an actor to the round number through which
	// they are flat-footed (lose Dex bonus to Defense). Set when a
	// successful parry deflects their attack, or by ActionSidestep
	// from an in-room defender naming the attacker.
	FlatFootedUntil map[ActorRef]int

	// DodgeUntil maps an actor to the round number through which
	// their dodge stance is active. Set by ActionDodge resolution
	// to `currentRound + 1`; consumed on the first incoming attack
	// that would otherwise resolve normally (grants +4 Defense and
	// flat-foot immunity for that one swing).
	DodgeUntil map[ActorRef]int

	// Threat[defender][attacker] = cumulative threat that attacker
	// has generated against defender over the lifetime of the fight.
	// Damage adds 1:1 (post-DR/post-resist, same value DamageTally
	// uses). Lazily initialized in resolveAction. Future NPC AI
	// reads HighestThreat(defender) to pick a retarget candidate;
	// healing / taunts / feign-death extend this in later slices.
	Threat map[ActorRef]map[ActorRef]int32
}

// HighestThreat returns the attacker with the largest threat score
// against defender, breaking ties by ActorRef.ID ascending so the
// result is deterministic. Returns the zero ActorRef when defender
// has no recorded threat. Caller is responsible for synchronization
// (Manager holds m.mu around mutations; tests read directly).
func (f *Fight) HighestThreat(defender ActorRef) ActorRef {
	row, ok := f.Threat[defender]
	if !ok || len(row) == 0 {
		return ActorRef{}
	}
	var (
		best      ActorRef
		bestScore int32 = -1
	)
	for ref, score := range row {
		if score > bestScore {
			best = ref
			bestScore = score
			continue
		}
		if score == bestScore && ref.ID < best.ID {
			best = ref
		}
	}
	return best
}

// pruneDead removes every entry in Dead or Fled from Order and clears
// both sets, plus any stance / threat bookkeeping keyed on the
// removed actors. Caller must hold the Manager write lock. Returns
// true when at least one actor was removed. Under per-actor cadence
// there is no index to walk back — each remaining entry has its own
// NextActAt and resolves on its own clock.
func (f *Fight) pruneDead() bool {
	if len(f.Dead) == 0 && len(f.Fled) == 0 {
		return false
	}
	removed := func(ref ActorRef) bool {
		if _, ok := f.Dead[ref]; ok {
			return true
		}
		if _, ok := f.Fled[ref]; ok {
			return true
		}
		return false
	}
	out := make([]ActorEntry, 0, len(f.Order))
	for _, e := range f.Order {
		if removed(e.Ref) {
			delete(f.ParryingUntil, e.Ref)
			delete(f.FlatFootedUntil, e.Ref)
			delete(f.DodgeUntil, e.Ref)
			// Threat: drop the defender row, then walk every other
			// row to drop the attacker column. Keeps the map bounded
			// as a fight grinds on and prevents stale refs from
			// influencing HighestThreat after a kill.
			delete(f.Threat, e.Ref)
			for _, inner := range f.Threat {
				delete(inner, e.Ref)
			}
			continue
		}
		out = append(out, e)
	}
	f.Order = out
	f.Dead = nil
	f.Fled = nil
	return true
}

// sortInitiative orders entries descending by Initiative, breaking
// ties by Tiebreak (descending) then by Ref.ID (ascending) so the
// order is deterministic given a fixed RNG seed.
func sortInitiative(entries []ActorEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Initiative != entries[j].Initiative {
			return entries[i].Initiative > entries[j].Initiative
		}
		if entries[i].Tiebreak != entries[j].Tiebreak {
			return entries[i].Tiebreak > entries[j].Tiebreak
		}
		return entries[i].Ref.ID < entries[j].Ref.ID
	})
}
