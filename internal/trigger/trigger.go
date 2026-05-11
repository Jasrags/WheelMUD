// Package trigger is the §15 / Phase F #29 declarative event-dispatch
// layer. Builders attach triggers to mob templates and rooms via YAML
// (`triggers:` sub-block). At runtime a Dispatcher subscribes to the
// existing eventbus events and tick.Buckets.Phase pulses, consults
// the in-memory Registry for matching triggers, and invokes a
// registered ActionHandler.
//
// V1 ships with three built-in actions (`noop`, `say`, `emote`) —
// just enough to validate the surface end-to-end without the §32 Lua
// machinery. Consumers (NPC dialogue #30, quest engine #31, embedded
// Lua #32) extend the action vocabulary by registering more handlers
// at boot.
package trigger

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// OwnerKind / Event aliases preserve the repo allow-listed enums at
// the package boundary so callers don't import repo just to spell a
// string constant.
type (
	OwnerKind = repo.TriggerOwnerKind
	Event     = repo.TriggerEvent
)

const (
	OwnerMobTemplate = repo.TriggerOwnerMobTemplate
	OwnerRoom        = repo.TriggerOwnerRoom

	EventOnEnter  = repo.TriggerEventOnEnter
	EventOnSay    = repo.TriggerEventOnSay
	EventOnAttack = repo.TriggerEventOnAttack
	EventOnDeath  = repo.TriggerEventOnDeath
	EventOnTick   = repo.TriggerEventOnTick
)

// OwnerRef is the resolved owner of a trigger at dispatch time. For
// mob_template-owned triggers, ID is the template id and InstanceID
// is the live mob instance whose presence in the room caused the
// dispatch (zero when the trigger is room-owned). RoomID is the room
// the action handler should write into.
type OwnerRef struct {
	Kind       OwnerKind
	ID         int64
	InstanceID int64
	RoomID     int64
}

// EventCtx is the per-fire context handed to action handlers. Fields
// are populated only for events that carry them (e.g. Text only for
// on_say; Actor only for events with a publishing actor). Callers
// should treat zero-valued fields as "not applicable".
type EventCtx struct {
	Event      Event
	RoomID     int64
	ActorKind  string // "character" / "mob" / ""
	ActorID    int64
	TargetKind string // for on_attack / on_death; "character" / "mob" / ""
	TargetID   int64
	Text       string // on_say utterance
	BucketName string // on_tick: "phase" / "combat" / "regen" / ...
}

// ActionHandler is invoked once per matching trigger with the
// trigger's payload (raw JSON) and the dispatch context. Errors are
// logged but do not abort the rest of the trigger fan-out — one bad
// action MUST NOT take down the bus.
type ActionHandler func(ctx context.Context, deps ActionDeps, owner OwnerRef, ev EventCtx, payload json.RawMessage) error

// SayPayload is the shared payload shape for say + emote actions.
type SayPayload struct {
	Text string `json:"text"`
}

// LogPayload is the noop action's payload (optional message logged
// at debug level).
type LogPayload struct {
	Message string `json:"message"`
}

// loggerOr returns deps.Logger if non-nil, else slog.Default.
func loggerOr(deps ActionDeps) *slog.Logger {
	if deps.Logger != nil {
		return deps.Logger
	}
	return slog.Default()
}
