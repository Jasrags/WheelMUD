package repo

import (
	"context"
	"errors"
)

// TriggerOwnerKind identifies what a Trigger is attached to.
type TriggerOwnerKind string

const (
	TriggerOwnerMobTemplate TriggerOwnerKind = "mob_template"
	TriggerOwnerRoom        TriggerOwnerKind = "room"
)

// TriggerEvent enumerates the canonical event names dispatched by
// the trigger subsystem (§15 / Phase F #29). Consumers wiring
// custom dispatchers should reuse these constants rather than raw
// strings so a typo fails at compile time.
type TriggerEvent string

const (
	TriggerEventOnEnter  TriggerEvent = "on_enter"
	TriggerEventOnSay    TriggerEvent = "on_say"
	TriggerEventOnAttack TriggerEvent = "on_attack"
	TriggerEventOnDeath  TriggerEvent = "on_death"
	TriggerEventOnTick   TriggerEvent = "on_tick"
	// Phase F #32 slice 5b — PC lifecycle events. on_login fires
	// once when a character finishes promoteToGame; on_logout fires
	// once when a session with an active character disconnects.
	// Both are room-owned: the trigger attaches to the character's
	// last-known room (login = promoted-into room; logout = the
	// room the character was standing in at disconnect).
	TriggerEventOnLogin  TriggerEvent = "on_login"
	TriggerEventOnLogout TriggerEvent = "on_logout"
)

// Trigger is one declarative dispatch row. The action vocabulary
// is open at the schema layer; the trigger.Runner validates against
// its in-process action registry at fire time.
type Trigger struct {
	ID        int64
	OwnerKind TriggerOwnerKind
	OwnerID   int64
	Event     TriggerEvent
	Match     string
	Action    string
	Payload   string // JSON; defaults to "{}"
	Priority  int

	// ConsecutiveFaults and Disabled back the per-trigger fault
	// budget added in Phase F #32 slice 1 (migration 0046). The
	// engine increments ConsecutiveFaults each time a Lua script
	// action returns a fault sentinel; at threshold (5) Disabled
	// flips and the trigger stops firing until an operator resets
	// it. A successful invocation resets ConsecutiveFaults to 0.
	// World re-deploys reset both columns.
	ConsecutiveFaults int
	Disabled          bool
}

// TriggerRepo persists declarative event triggers attached to mobs
// or rooms. The world loader rewrites the table on every
// LoadAndSync; OLC mutations land in #34.
type TriggerRepo interface {
	// Create inserts a trigger and returns it with ID populated.
	Create(ctx context.Context, t Trigger) (Trigger, error)
	// ListByOwner returns every trigger for the given owner.
	ListByOwner(ctx context.Context, kind TriggerOwnerKind, ownerID int64) ([]Trigger, error)
	// ListAll returns every trigger, sorted by ID.
	ListAll(ctx context.Context) ([]Trigger, error)
	// DeleteByOwner removes every trigger attached to the given
	// owner (used by the loader to rebuild rows for a re-seeded
	// mob/room).
	DeleteByOwner(ctx context.Context, kind TriggerOwnerKind, ownerID int64) error
	// RecordTriggerFault writes the new fault counter and disabled
	// flag for the given trigger row. Phase F #32 slice 1 — Lua
	// action faults call this with the post-mutation values; the
	// engine has already computed whether the threshold was hit.
	// Returns ErrTriggerNotFound when no row matches id.
	RecordTriggerFault(ctx context.Context, id int64, faults int, disabled bool) error
	// ResetAllFaults zeroes consecutive_faults + disabled across
	// every row. The world loader calls this on every LoadAndSync
	// so a re-deploy never preserves stale fault state.
	ResetAllFaults(ctx context.Context) error
}

// ErrInvalidTrigger is returned from Create when fields fail
// validation (unknown owner_kind/event, empty action, etc).
var ErrInvalidTrigger = errors.New("repo: invalid trigger")

// ErrTriggerNotFound is returned by RecordTriggerFault when no row
// matches the supplied id.
var ErrTriggerNotFound = errors.New("repo: trigger not found")

// ValidTriggerOwnerKind reports whether k is one of the schema's
// allow-listed owner kinds.
func ValidTriggerOwnerKind(k TriggerOwnerKind) bool {
	return k == TriggerOwnerMobTemplate || k == TriggerOwnerRoom
}

// ValidTriggerEvent reports whether e is one of the schema's
// allow-listed event names.
func ValidTriggerEvent(e TriggerEvent) bool {
	switch e {
	case TriggerEventOnEnter, TriggerEventOnSay, TriggerEventOnAttack,
		TriggerEventOnDeath, TriggerEventOnTick,
		TriggerEventOnLogin, TriggerEventOnLogout:
		return true
	}
	return false
}
