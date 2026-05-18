package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jasrags/WheelMUD/internal/affects"
	"github.com/Jasrags/WheelMUD/internal/channeling"
	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/tick"
)

// tickerDeps bundles the per-tick subscribers' shared dependencies.
// Built once in main() and passed to setupTickers — corpse decay,
// affects, channeling, and stamina regen all walk the session snapshot
// each pulse, so they share the same dep set.
type tickerDeps struct {
	sessions       *session.Registry
	combatMgr      *combat.Manager
	characters     repo.CharacterRepo
	items          repo.ItemRepo
	chargenCatalog *chargen.Catalog
	bus            *eventbus.Bus
	buckets        *tick.Buckets
}

// setupTickers wires corpse decay, affects, channeling, and stamina
// regen onto their tick buckets. Returns nothing — every subscriber's
// state lives behind its own ticker/manager constructor.
//
// Boot-time corpse-decay rearm runs here too: items.decay_expires_at
// rows from a previous boot get past-deadline sweeps or re-queued
// based on time.Now().
func setupTickers(d tickerDeps) {
	// Phase D #19 slice 2: corpse decay. Sweeper deletes corpse rows
	// 5 min after they spawn (constant lives in internal/combat) and
	// emits a "crumble" line via WriteAsync to room peers.
	corpseDecay := combat.NewDecayer(d.items, func(roomID int64, msg string) {
		for _, peer := range d.sessions.Snapshot() {
			if peer == nil || peer.CurrentRoomID != roomID {
				continue
			}
			if err := peer.WriteAsync(msg); err != nil {
				slog.Debug("decay: write peer failed", "error", err)
			}
		}
	}, d.bus)
	d.combatMgr.SetDecayer(corpseDecay)
	d.buckets.Decay.Subscribe(corpseDecay.Tick)
	// Boot-time rearm: replay persisted corpse decay deadlines from
	// items.decay_expires_at (migration 0050). Past-deadline rows are
	// swept on the spot; future rows are reinserted into the queue so
	// the Decay bucket will resolve them at their original schedule.
	if rescheduled, swept, err := corpseDecay.RearmFromRepo(context.Background(), d.items, time.Now()); err != nil {
		slog.Warn("corpse decay rearm failed", "error", err)
	} else if rescheduled > 0 || swept > 0 {
		slog.Info("corpse decay rearmed",
			"rescheduled", rescheduled, "swept", swept)
	}

	// Phase E #26: out-of-combat affects ticker. Walks every in-world
	// session, skips characters already in a fight (combat's end-of-
	// round tick handles those), and decrements affect durations on
	// the rest. Cadence is 6 s — just slow enough that scanning the
	// session map per pulse stays cheap, fast enough that short buffs
	// have visible feedback.
	affectsCandidates := func() []affects.Candidate {
		snap := d.sessions.Snapshot()
		out := make([]affects.Candidate, 0, len(snap))
		for _, s := range snap {
			if s == nil {
				continue
			}
			charID, _, roomID := s.InWorld()
			if charID == 0 || roomID == 0 {
				continue
			}
			out = append(out, affects.Candidate{CharacterID: charID, RoomID: roomID})
		}
		return out
	}
	affectsTicker := affects.NewSessionTicker(
		affectsCandidates,
		d.combatMgr,
		characterAffectsLoader{d.characters},
		eventbusAdapter{d.bus},
		slog.Default(),
	)
	affectsTicker.SetDeathHook(func(ctx context.Context, charID int64, _ string) {
		// DoT-death entrypoint into the §19 death pipeline. Cause is
		// surfaced via the TickDamaged event the ticker also publishes,
		// so the handler doesn't need it.
		d.combatMgr.HandleAffectDeath(ctx, charID)
	})
	d.buckets.Affects.Subscribe(affectsTicker.Tick)

	// Phase E #27: channeling ticker. Refills slot pools 8h after the
	// last refresh and accrues Madness on every pulse for embraced
	// male channelers. Subscribed to the Regen bucket (30s) — slow
	// enough that the 8h gate adds negligible load and fast enough
	// that Madness accrual is observable in tests with a tightened
	// TickInterval.
	channelingCandidates := func() []channeling.Candidate {
		snap := d.sessions.Snapshot()
		out := make([]channeling.Candidate, 0, len(snap))
		for _, s := range snap {
			if s == nil {
				continue
			}
			charID, _, roomID := s.InWorld()
			if charID == 0 {
				continue
			}
			out = append(out, channeling.Candidate{CharacterID: charID, RoomID: roomID})
		}
		return out
	}
	channelingTicker := channeling.NewSessionTicker(
		channelingCandidates,
		characterChannelingLoader{d.characters},
		nil, // time.Now
		slog.Default(),
	)
	d.buckets.Regen.Subscribe(channelingTicker.Tick)

	// Phase L slice 63: stamina regen ticker. Tops StaminaCurrent
	// toward StaminaMax at the racial StaminaRegen rate (halved by
	// heavy armor) on every Regen pulse. Subscribed alongside the
	// channeling ticker so the action-cost pool refills at the same
	// cadence the channeling pools refresh.
	staminaCandidates := func() []combat.StaminaCandidate {
		snap := d.sessions.Snapshot()
		out := make([]combat.StaminaCandidate, 0, len(snap))
		for _, s := range snap {
			if s == nil {
				continue
			}
			charID, _, roomID := s.InWorld()
			if charID == 0 {
				continue
			}
			out = append(out, combat.StaminaCandidate{CharacterID: charID, RoomID: roomID})
		}
		return out
	}
	staminaTicker := combat.NewStaminaTicker(
		staminaCandidates,
		d.characters,
		d.items,
		d.chargenCatalog,
		slog.Default(),
	)
	d.buckets.Regen.Subscribe(staminaTicker.Tick)
}
