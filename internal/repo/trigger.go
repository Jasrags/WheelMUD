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
}

// ErrInvalidTrigger is returned from Create when fields fail
// validation (unknown owner_kind/event, empty action, etc).
var ErrInvalidTrigger = errors.New("repo: invalid trigger")

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
		TriggerEventOnDeath, TriggerEventOnTick:
		return true
	}
	return false
}
