package channeling

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// Candidate is a (characterID, roomID) tuple supplied by the cmd-layer
// adapter. Mirrors internal/affects.Candidate; both tickers consume
// the same in-world snapshot. RoomID is unused this slice — channeling
// timers are independent of combat pacing — but is carried for parity
// with the affects ticker so the cmd-layer adapter can return one
// slice both consumers share.
type Candidate struct {
	CharacterID int64
	RoomID      int64
}

// CandidateSource fetches the current snapshot of in-world characters
// to consider for channeling ticking. The cmd-layer adapter pulls
// these from the session.Registry by walking sessions.Snapshot() and
// calling Session.InWorld() on each — this package stays free of any
// telnet/session import.
type CandidateSource func() []Candidate

// CharLoader is the slim subset of repo.CharacterRepo the ticker
// needs. Defined here so tests can substitute a fake without a real
// repo / DB.
type CharLoader interface {
	GetByID(ctx context.Context, id int64) (Character, error)
	RecordChanneling(ctx context.Context, id int64, c *creature.Channeling) error
}

// Character is the projection the ticker reads. The full repo.Character
// type satisfies this implicitly via duck typing through the adapter
// in cmd/server/main.go.
type Character struct {
	Channeling *creature.Channeling
}

// SessionTicker walks every in-world character with a non-nil
// Channeling record, applies RefreshIfDue + AccrueMadness, and
// persists iff anything moved.
//
// Unlike affects, channeling does NOT skip in-fight rooms — slot
// refresh and madness are independent of combat pacing.
type SessionTicker struct {
	candidates CandidateSource
	chars      CharLoader
	now        func() time.Time
	log        *slog.Logger
}

// NewSessionTicker constructs a ticker. log may be nil; default
// slog is used. nowFn may be nil; time.Now is used.
func NewSessionTicker(
	candidates CandidateSource,
	chars CharLoader,
	nowFn func() time.Time,
	log *slog.Logger,
) *SessionTicker {
	if log == nil {
		log = slog.Default()
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	return &SessionTicker{
		candidates: candidates,
		chars:      chars,
		now:        nowFn,
		log:        log,
	}
}

// Tick runs one pass over the candidate snapshot. Designed to be
// passed directly to tick.Bucket.Subscribe.
func (t *SessionTicker) Tick(ctx context.Context) {
	if t == nil || t.candidates == nil {
		return
	}
	now := t.now()
	for _, c := range t.candidates() {
		t.tickOne(ctx, c, now)
	}
}

func (t *SessionTicker) tickOne(ctx context.Context, c Candidate, now time.Time) {
	ch, err := t.chars.GetByID(ctx, c.CharacterID)
	if err != nil {
		t.log.Warn("channeling: load character failed",
			"char", c.CharacterID, "error", err)
		return
	}
	if ch.Channeling == nil {
		return
	}
	refreshed := RefreshIfDue(ch.Channeling, now)
	maddened := AccrueMadness(ch.Channeling, now)
	if !refreshed && !maddened {
		return
	}
	if err := t.chars.RecordChanneling(ctx, c.CharacterID, ch.Channeling); err != nil {
		t.log.Warn("channeling: write-back failed",
			"char", c.CharacterID, "error", err)
	}
}
