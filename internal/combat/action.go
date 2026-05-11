package combat

import "time"

// DefaultActionCost returns the wall-clock interval an actor must
// wait after resolving an action of the given kind before they can
// act again. Pure function over kind so #62 (gear), #63 (race), and
// #65 (feats) can layer multiplicative modifiers on top without
// changing call sites. ActionNone advances by one second so an idle
// actor doesn't perpetually re-trigger the ready check and burst-act
// the moment input arrives.
func DefaultActionCost(kind ActionKind) time.Duration {
	switch kind {
	case ActionAttack:
		return 3 * time.Second
	case ActionParry:
		return 2 * time.Second
	case ActionFlee:
		return 2 * time.Second
	default:
		return 1 * time.Second
	}
}

// ActionKind tags a queued combat action. Slice 1 ships only Attack;
// future kinds (flee, kick, weave, ready, defend) slot in here without
// changing the queue plumbing.
type ActionKind uint8

const (
	// ActionNone is the zero value — no action queued.
	ActionNone ActionKind = iota
	// ActionAttack rolls a melee attack against Target this round.
	ActionAttack
	// ActionParry sets the actor into a parrying stance for the rest of
	// this round. The next incoming attack against the actor triggers
	// an opposed roll; on success the attack is negated and the
	// attacker becomes flat-footed for one round. Stance is consumed
	// after one trigger.
	ActionParry
	// ActionFlee attempts to retreat from the active fight. On
	// resolution the Manager hands off to its FleeMover; success
	// moves the actor to a neighbouring room and prunes them from
	// the fight order on the next pulse.
	ActionFlee
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
