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
type CharLoader interface {
	GetByID(ctx context.Context, id int64) (Character, error)
	RecordAffects(ctx context.Context, id int64, affects []creature.Affect) error
}

// Character is the projection the ticker reads. The full repo.Character
// type satisfies this implicitly via duck typing through the adapter
// in cmd/server/main.go (see characterLoaderAdapter there).
type Character struct {
	Affects []creature.Affect
}

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
	log        *slog.Logger
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
}
