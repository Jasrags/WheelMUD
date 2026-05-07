package combat

import "context"

// FleeMover decides whether an actor's ActionFlee succeeds and, on
// success, performs the actual room transition (broadcast, session
// move, repo write-back). Combat stays decoupled from session and
// world plumbing — the cmd-layer implementation owns those concerns.
//
// AttemptFlee MUST NOT be called with the Manager lock held; combat
// invokes it from resolveAction after releasing rngMu, mirroring the
// repo-write pattern the death pipeline uses.
type FleeMover interface {
	AttemptFlee(ctx context.Context, roomID int64, actor ActorRef) FleeResult
}

// FleeResult is the outcome of a flee attempt. Direction / ToRoomID
// are zero on failure. Reason is a short fixed-vocabulary token used
// by tests and forensics; Success is the field the Manager dispatches
// on.
type FleeResult struct {
	Success   bool
	Direction string
	ToRoomID  int64
	Reason    string
}

// SetFleeMover wires the cmd-layer implementation. Optional: when
// unset, ActionFlee resolves as an immediate failure with reason
// "no_mover" so the verb still gets a CombatFlee event.
func (m *Manager) SetFleeMover(mover FleeMover) {
	m.mu.Lock()
	m.fleeMover = mover
	m.mu.Unlock()
}
