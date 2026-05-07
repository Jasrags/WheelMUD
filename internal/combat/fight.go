package combat

import (
	"sort"
	"time"
)

// ActorEntry is one combatant's slot in the initiative order.
// Initiative is the resolved roll (d20 + DexMod + InitMod). Tiebreak
// is the raw d20 captured at Start so a stable resort after a
// mid-fight join produces the same order — ties resolve by Tiebreak
// then by ActorRef.ID.
type ActorEntry struct {
	Ref        ActorRef
	Initiative int   // d20 + DexMod + InitMod
	Tiebreak   int32 // raw d20 captured at Start
}

// Fight is the per-room combat aggregate. One Fight per RoomID at a
// time — the Manager rejects double-Start. Order is sorted descending
// by Initiative on Start and stays stable through the rest of the
// fight; ActiveIdx wraps each round.
type Fight struct {
	RoomID    int64
	Round     int          // 1-based; 0 = not started
	Order     []ActorEntry // sorted descending
	ActiveIdx int          // index into Order; advances on Tick
	StartedAt time.Time

	// Actions is the per-actor queue of intents for the current round.
	// Lazily initialized by EnqueueAction. Resolved + cleared by the
	// Manager's Tick after the active actor's action lands.
	Actions map[ActorRef]Action

	// DamageTally accumulates the total post-DR/post-resist damage
	// each actor has dealt over the lifetime of the fight. Used to
	// allocate XP weighted by contribution when a mob dies. Lazily
	// initialized in resolveAction. Pre-cursor to #20 threat tables.
	DamageTally map[ActorRef]int32

	// Dead is the set of actors that have hit HP ≤ 0 inside this
	// fight. Populated by the death handler; consumed by tickRoom at
	// the top of the next pulse to prune Order before ActiveIdx
	// advances. Mutating Order in-flight inside resolveAction would
	// shift indices under the Tick caller, so the prune is deferred.
	Dead map[ActorRef]struct{}

	// Fled mirrors Dead for actors that successfully retreated via
	// ActionFlee. pruneDead removes both sets in one pass — the
	// distinction exists so death-only consumers (XP allocation,
	// corpse spawn) don't fire for fleeing actors.
	Fled map[ActorRef]struct{}

	// ParryingUntil maps an actor to the round number through which
	// their parrying stance is active. Set by ActionParry resolution;
	// cleared on consumption (the first incoming attack that triggers
	// an opposed roll) or when the round number passes.
	ParryingUntil map[ActorRef]int

	// FlatFootedUntil maps an actor to the round number through which
	// they are flat-footed (lose Dex bonus to Defense). Set when a
	// successful parry deflects their attack.
	FlatFootedUntil map[ActorRef]int
}

// pruneDead removes every entry in Dead or Fled from Order and clears
// both sets. Caller must hold the Manager write lock. Re-clamps
// ActiveIdx so it never points past the new end of Order. Stance maps
// (ParryingUntil, FlatFootedUntil) drop entries for the removed
// actors. Returns true when at least one actor was removed.
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
	removedBefore := 0
	for i, e := range f.Order {
		if removed(e.Ref) {
			if i <= f.ActiveIdx {
				removedBefore++
			}
			delete(f.ParryingUntil, e.Ref)
			delete(f.FlatFootedUntil, e.Ref)
			continue
		}
		out = append(out, e)
	}
	f.Order = out
	f.Dead = nil
	f.Fled = nil
	if len(f.Order) == 0 {
		f.ActiveIdx = 0
		return true
	}
	// Walk ActiveIdx back by however many dead entries sat at-or-
	// before it so the next round's "(ActiveIdx+1) % len(Order)"
	// resumes on the actor that would have come after the killed
	// one rather than skipping a turn. When the active actor itself
	// died (ActiveIdx underflows), clamp to -1 so the next round-
	// advance lands on index 0 — the new head — rather than wrapping
	// forward and silently skipping the head's first turn.
	f.ActiveIdx -= removedBefore
	if f.ActiveIdx < 0 {
		f.ActiveIdx = -1
	}
	if f.ActiveIdx >= len(f.Order) {
		f.ActiveIdx = -1
	}
	return true
}

// Active returns the actor whose turn is currently being resolved,
// or the zero ActorRef when the fight has not started or has empty
// Order (defensive — Manager rejects empty participant lists, so
// this branch is only reachable through direct field manipulation
// in tests).
func (f *Fight) Active() ActorRef {
	if f.ActiveIdx < 0 {
		// pruneDead may park ActiveIdx at -1 when the active actor
		// itself was killed; the next round-advance lands on index 0.
		// External readers (debug verbs, tests) that peek between
		// prune and round-advance get the zero ref instead of a
		// panic.
		return ActorRef{}
	}
	if len(f.Order) == 0 {
		return ActorRef{}
	}
	return f.Order[f.ActiveIdx].Ref
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
