package combat

// ActionKind tags a queued combat action. Slice 1 ships only Attack;
// future kinds (flee, kick, weave, ready, defend) slot in here without
// changing the queue plumbing.
type ActionKind uint8

const (
	// ActionNone is the zero value — no action queued.
	ActionNone ActionKind = iota
	// ActionAttack rolls a melee attack against Target this round.
	ActionAttack
)

// Action is one combatant's queued intent for the current round. The
// owner field is implicit in the queue map key on Fight.Actions.
type Action struct {
	Kind   ActionKind
	Target ActorRef
	// WeaponID is the item id of the equipped weapon at the moment the
	// action was queued. Zero means unarmed. The resolver re-reads the
	// weapon stats by id to support a `wield` mid-fight without
	// invalidating an already-queued attack.
	WeaponID int64
}

// EnqueueAction stores an action for actor in the active fight in
// roomID. Returns ErrFightNotFound when no fight is active. The
// previous action (if any) is overwritten — re-issuing `attack` mid-
// round simply re-targets without piling up actions.
func (m *Manager) EnqueueAction(roomID int64, actor ActorRef, a Action) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.fights[roomID]
	if !ok {
		return ErrFightNotFound
	}
	if f.Actions == nil {
		f.Actions = make(map[ActorRef]Action)
	}
	f.Actions[actor] = a
	return nil
}

// PendingAction reports the currently queued action for actor, if any.
// Used by the dispatcher / verb tests to verify state without popping.
func (m *Manager) PendingAction(roomID int64, actor ActorRef) (Action, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.fights[roomID]
	if !ok {
		return Action{}, false
	}
	a, ok := f.Actions[actor]
	return a, ok
}

// popAction removes and returns the queued action for actor. Caller
// must hold m.mu.
func (f *Fight) popAction(actor ActorRef) (Action, bool) {
	if f.Actions == nil {
		return Action{}, false
	}
	a, ok := f.Actions[actor]
	if !ok {
		return Action{}, false
	}
	delete(f.Actions, actor)
	return a, true
}
