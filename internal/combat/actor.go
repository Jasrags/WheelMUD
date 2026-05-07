// Package combat owns the round-tick orchestration for §11 combat.
//
// Slice 1 (Phase D #16) ships only the spine: a per-room Fight
// aggregate, an initiative-ordered actor list, a round counter
// advanced by tick.Buckets.Combat, and CombatStarted /
// RoundStarted / CombatEnded events on the typed eventbus.
//
// What is intentionally NOT in this slice:
//   - the `attack` verb (#18)
//   - hit/miss / damage rolls (#17, #18)
//   - HP delta / death / corpse / XP grant (#19)
//   - threat tables (#20)
//   - PvE / PvP gating (#21)
//   - persistence — Fight is in-memory; a server restart drops
//     every active fight. Combat is short-lived enough that this
//     is acceptable for V1.
package combat

import "strconv"

// ActorKind tags an ActorRef so a single int64 namespace can be
// disambiguated across characters and mob instances. Future actors
// (familiars, summoned servitors) slot in here without changing
// callers.
type ActorKind uint8

const (
	// ActorKindUnknown is the zero value — useful as a "not set"
	// sentinel; callers should never publish events with this kind.
	ActorKindUnknown ActorKind = iota
	// ActorKindCharacter resolves Ref.ID through CharacterRepo.
	ActorKindCharacter
	// ActorKindMob resolves Ref.ID through MobInstanceRepo.
	ActorKindMob
)

// ActorRef points at one combatant. Characters and mob instances
// both use int64 ids in their respective repos but the namespaces
// are distinct, so callers must keep the kind tag with the id.
type ActorRef struct {
	Kind ActorKind
	ID   int64
}

// String returns a debug-friendly representation. Not used on the
// player-facing wire — that path resolves the ref through the repo
// and renders Name.
func (r ActorRef) String() string {
	id := strconv.FormatInt(r.ID, 10)
	switch r.Kind {
	case ActorKindCharacter:
		return "char:" + id
	case ActorKindMob:
		return "mob:" + id
	}
	return "unknown:" + id
}
