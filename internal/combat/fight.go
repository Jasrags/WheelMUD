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
}

// Active returns the actor whose turn is currently being resolved,
// or the zero ActorRef when the fight has not started or has empty
// Order (defensive — Manager rejects empty participant lists, so
// this branch is only reachable through direct field manipulation
// in tests).
func (f *Fight) Active() ActorRef {
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
