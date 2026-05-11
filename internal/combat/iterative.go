package combat

// iterativeTargetGone reports whether the named target is no longer a
// viable swing candidate inside the fight at roomID — either absent
// from Order entirely, or queued for prune via Dead/Fled. Used by the
// iterative drain loop (Phase L #66) to short-circuit follow-up
// swings after the first swing kills the target or it flees during
// resolution. The zero ActorRef (e.g. an Attack queued without a
// target) is treated as "gone" so a malformed action can't loop.
//
// Takes m.mu for the read; safe to call between resolveAction
// invocations inside the per-actor swing chain.
func (m *Manager) iterativeTargetGone(roomID int64, target ActorRef) bool {
	var zero ActorRef
	if target == zero {
		return true
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.fights[roomID]
	if !ok {
		return true
	}
	if _, dead := f.Dead[target]; dead {
		return true
	}
	if _, fled := f.Fled[target]; fled {
		return true
	}
	for _, e := range f.Order {
		if e.Ref == target {
			return false
		}
	}
	return true
}

// IterativeBonusesFor returns the per-swing attack-roll deltas implied
// by a creature's BAB. Mirrors the D&D 3.x iterative-attack
// progression: BAB 6 unlocks a second swing at -5, BAB 11 a third at
// -10, BAB 16 a fourth at -15.
//
// The returned slice is always non-empty — non-positive BAB still
// yields one swing at 0. Length doubles as the per-pulse swing count
// for ActorEntry.PendingSwings.
func IterativeBonusesFor(bab int16) []int16 {
	switch {
	case bab >= 16:
		return []int16{0, -5, -10, -15}
	case bab >= 11:
		return []int16{0, -5, -10}
	case bab >= 6:
		return []int16{0, -5}
	default:
		return []int16{0}
	}
}
