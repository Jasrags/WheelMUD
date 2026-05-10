package affects

import (
	"context"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// Candidate is a (characterID, roomID) tuple supplied by the cmd-layer
// adapter. RoomID is read from the session's in-world snapshot at
// fetch time. The ticker treats the slice as a frozen snapshot —
// stale rooms are still valid because we re-read the character row
// from the repo before mutating.
type Candidate struct {
	CharacterID int64
	RoomID      int64
}

// CandidateSource fetches the current snapshot of in-world characters
// to consider for affects ticking. The cmd-layer adapter pulls these
// from the session.Registry by walking sessions.Snapshot() and
// calling Session.InWorld() on each — this package stays free of any
// telnet/session import.
type CandidateSource func() []Candidate

// FightLookup reports whether a room currently has an active combat
// fight. The session ticker skips rooms with active fights so combat's
// end-of-round tick handles those participants instead — no double-tick.
type FightLookup interface {
	Active(roomID int64) bool
}

// CharLoader is the slim subset of repo.CharacterRepo the ticker
// needs. Defined here so tests can substitute a fake without a real
// repo / DB.
//
// RecordHP commits a HP delta from the TickEffect dispatcher. The
// ticker uses the narrow HP-only write — NOT RecordCore — because
// it holds a stale snapshot of conditions/position_flags; combat
// runs concurrently and may have set CondProne/CondStunned/etc.
// between the load and the write. RecordHP leaves those columns
// untouched.
type CharLoader interface {
	GetByID(ctx context.Context, id int64) (Character, error)
	RecordAffects(ctx context.Context, id int64, affects []creature.Affect) error
	RecordHP(ctx context.Context, id int64, hp, subdual int32) error
}

// Character is the projection the ticker reads. The full repo.Character
// type satisfies this implicitly via duck typing through the adapter
// in cmd/server/main.go (see characterLoaderAdapter there).
//
// HP fields + Conditions / Position are needed for the TickEffect
// dispatcher; an Affect with TickEffect != "" runs through
// ApplyTickEffects to fold per-tick deltas into HPCurrent.
type Character struct {
	Affects   []creature.Affect
	HPCurrent int32
	HPMax     int32
	Subdual   int32
	Condition creature.Condition
	Position  creature.PositionFlags
}

// DeathHook is the slim callback the cmd-layer wires to the
// combat.Manager death pipeline. Fired when a TickEffect drains a
// character's HP to zero (or below). The ticker passes the affect
// Name as the cause label so the death broadcast can read "X dies
// from poison." instead of a generic line.
//
// nil is allowed — the ticker just skips publishing in that case
// (used by tests that don't wire a manager).
type DeathHook func(ctx context.Context, characterID int64, cause string)

// EventPublisher is the slim subset of *eventbus.Bus the ticker needs.
type EventPublisher interface {
	Publish(ctx context.Context, ev any)
}

// SessionTicker walks every in-world character not currently in a
// fight, decrements their affect durations, persists changes, and
// publishes an Expired event for each character that lost at least
// one affect.
//
// The ticker takes no locks. Concurrency safety relies on:
//   - CandidateSource returning a frozen slice copy.
//   - FightLookup being safe for concurrent reads (combat.Manager.Active
//     takes its RLock).
//   - CharLoader writes use repo-level optimistic patterns
//     (RecordAffects rewrites a single column).
type SessionTicker struct {
	candidates CandidateSource
	fights     FightLookup
	chars      CharLoader
	bus        EventPublisher
	onDeath    DeathHook
	log        *slog.Logger
}

// SetDeathHook installs the cmd-layer's death callback. Pass nil to
// clear (tests). Safe to call before or after wiring the ticker into
// a bucket.
func (t *SessionTicker) SetDeathHook(h DeathHook) {
	if t == nil {
		return
	}
	t.onDeath = h
}

// NewSessionTicker constructs a ticker. log may be nil; default
// slog is used.
func NewSessionTicker(
	candidates CandidateSource,
	fights FightLookup,
	chars CharLoader,
	bus EventPublisher,
	log *slog.Logger,
) *SessionTicker {
	if log == nil {
		log = slog.Default()
	}
	return &SessionTicker{
		candidates: candidates,
		fights:     fights,
		chars:      chars,
		bus:        bus,
		log:        log,
	}
}

// Tick runs one pass over the candidate snapshot. Designed to be
// passed directly to tick.Bucket.Subscribe.
func (t *SessionTicker) Tick(ctx context.Context) {
	if t == nil || t.candidates == nil {
		return
	}
	for _, c := range t.candidates() {
		t.tickOne(ctx, c)
	}
}

func (t *SessionTicker) tickOne(ctx context.Context, c Candidate) {
	if t.fights != nil && t.fights.Active(c.RoomID) {
		// In-fight characters tick on the combat round-tick.
		return
	}
	ch, err := t.chars.GetByID(ctx, c.CharacterID)
	if err != nil {
		t.log.Warn("affects: load character failed",
			"char", c.CharacterID, "error", err)
		return
	}
	if len(ch.Affects) == 0 {
		return
	}
	// Run TickEffect dispatch BEFORE the duration tick so a 1-tick
	// poison still gets one HP delta on the same pulse it expires.
	tickCore := creature.Core{
		HPCurrent: ch.HPCurrent,
		HPMax:     ch.HPMax,
		Affects:   ch.Affects,
	}
	newHP, tickEvents := ApplyTickEffects(tickCore)
	if len(tickEvents) > 0 {
		if err := t.chars.RecordHP(ctx, c.CharacterID, newHP, ch.Subdual); err != nil {
			t.log.Warn("affects: tick HP write-back failed",
				"char", c.CharacterID, "error", err)
		} else if t.bus != nil {
			t.bus.Publish(ctx, TickDamaged{
				CharacterID: c.CharacterID,
				RoomID:      c.RoomID,
				Events:      tickEvents,
				NewHP:       newHP,
				HPMax:       ch.HPMax,
			})
		}
	}

	next, expired := Tick(ch.Affects)
	if err := t.chars.RecordAffects(ctx, c.CharacterID, next); err != nil {
		t.log.Warn("affects: write-back failed",
			"char", c.CharacterID, "error", err)
		return
	}
	if len(expired) > 0 && t.bus != nil {
		t.bus.Publish(ctx, Expired{
			CharacterID: c.CharacterID,
			RoomID:      c.RoomID,
			Names:       expired,
		})
	}

	// Death-from-DoT: if the tick drained HP to zero (or below), hand
	// off to the cmd-layer's combat death pipeline. Use the last
	// damaging event's Name as the cause label.
	if newHP <= 0 && len(tickEvents) > 0 && t.onDeath != nil {
		var cause string
		for i := len(tickEvents) - 1; i >= 0; i-- {
			if tickEvents[i].Delta < 0 {
				cause = tickEvents[i].Name
				break
			}
		}
		t.onDeath(ctx, c.CharacterID, cause)
	}
}
