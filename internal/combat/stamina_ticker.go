package combat

import (
	"context"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// Phase L slice 63 — stamina regen ticker.
//
// Mirrors affects.SessionTicker shape: an external CandidateSource
// (cmd-layer adapter walking session.Registry) supplies the
// (charID, roomID) tuples; the ticker fetches each character, applies
// the per-pulse refill, and writes back via RecordStamina. Lives in
// the combat package because the drain side does too — keeping both
// sides next to each other keeps the action-cost numbers honest.

// StaminaCandidate is the (characterID, roomID) tuple supplied by
// the cmd-layer adapter. RoomID is informational only today (PLAN
// reserves it for a future "regen halts in noisy / dangerous rooms"
// gate); regen runs everywhere right now, even mid-fight.
type StaminaCandidate struct {
	CharacterID int64
	RoomID      int64
}

// StaminaCandidateSource is the cmd-layer-supplied snapshot fn.
type StaminaCandidateSource func() []StaminaCandidate

// StaminaCharLoader is the slim subset of repo.CharacterRepo the
// stamina ticker needs. Defined locally so tests can substitute a
// fake without standing up a full repo.
type StaminaCharLoader interface {
	GetByID(ctx context.Context, id int64) (repo.Character, error)
	RecordStamina(ctx context.Context, id int64, current int32) error
}

// StaminaItemLoader is the slim subset of repo.ItemRepo the regen
// ticker needs to look up the worn body armor's weight class for
// the heavy-armor regen penalty. nil disables the lookup, which
// degrades cleanly to "always full regen" (the floor any future
// fixture would land on if it forgot to wire items).
type StaminaItemLoader interface {
	GetByID(ctx context.Context, id int64) (repo.Item, error)
}

// StaminaTicker walks every in-world character on each Regen pulse
// and tops up StaminaCurrent toward StaminaMax. Heavy body armor
// halves the regen delta. The ticker takes no locks — concurrency
// safety relies on:
//
//   - candidates returning a frozen slice copy.
//   - the load + write going through repo methods that own their
//     own locking (RecordStamina is a single-column UPDATE).
//   - the read-modify-write window being narrow enough that a
//     concurrent combat drain landing between load and write at
//     worst over-credits one tick's worth of regen — small enough
//     to ignore for V1.
type StaminaTicker struct {
	candidates StaminaCandidateSource
	chars      StaminaCharLoader
	items      StaminaItemLoader
	log        *slog.Logger
}

// NewStaminaTicker constructs the ticker. items may be nil — the
// heavy-armor penalty path skips when items is unavailable so a test
// can run the ticker without a full ItemRepo. log defaults to the
// global slog when nil.
func NewStaminaTicker(
	candidates StaminaCandidateSource,
	chars StaminaCharLoader,
	items StaminaItemLoader,
	log *slog.Logger,
) *StaminaTicker {
	if log == nil {
		log = slog.Default()
	}
	return &StaminaTicker{
		candidates: candidates,
		chars:      chars,
		items:      items,
		log:        log,
	}
}

// Tick runs one pass over the candidate snapshot. Designed to be
// passed directly to tick.Bucket.Subscribe(...).
func (t *StaminaTicker) Tick(ctx context.Context) {
	if t == nil || t.candidates == nil {
		return
	}
	for _, c := range t.candidates() {
		t.tickOne(ctx, c)
	}
}

func (t *StaminaTicker) tickOne(ctx context.Context, c StaminaCandidate) {
	ch, err := t.chars.GetByID(ctx, c.CharacterID)
	if err != nil {
		t.log.Warn("stamina: load character failed",
			"char", c.CharacterID, "error", err)
		return
	}
	// Halt regen on dead/unconscious characters — matches the affects
	// ticker's stance for HoT effects: no recovery while at 0 HP.
	if ch.Core.HPCurrent <= 0 {
		return
	}
	if ch.Core.StaminaMax <= 0 {
		return
	}
	if ch.Core.StaminaCurrent >= ch.Core.StaminaMax {
		return
	}
	regen := EffectiveStaminaRegen(ch.Core.StaminaRegen, t.armorWeightClass(ctx, ch.Equipment))
	if regen <= 0 {
		return
	}
	next := ch.Core.StaminaCurrent + regen
	if next > ch.Core.StaminaMax {
		next = ch.Core.StaminaMax
	}
	if next == ch.Core.StaminaCurrent {
		return
	}
	if err := t.chars.RecordStamina(ctx, c.CharacterID, next); err != nil {
		t.log.Warn("stamina: write-back failed",
			"char", c.CharacterID, "error", err)
	}
}

// armorWeightClass resolves the body-armor weight-class string for
// the actor's currently-worn armor. Returns "" when no armor is
// worn, when items is nil, or when the lookup fails — all of which
// degrade to "no penalty" inside EffectiveStaminaRegen.
func (t *StaminaTicker) armorWeightClass(ctx context.Context, eq creature.Equipment) string {
	if t.items == nil {
		return ""
	}
	aid := eq.Get(creature.SlotArmor)
	if aid == 0 {
		return ""
	}
	it, err := t.items.GetByID(ctx, aid)
	if err != nil {
		return ""
	}
	if as, ok := it.Stats.(*repo.ArmorStats); ok {
		return as.WeightClass
	}
	return ""
}
