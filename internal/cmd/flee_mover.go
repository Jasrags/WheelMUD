package cmd

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// fleeDC is the static target the d20 + DexMod flee roll must meet.
// V1 keeps the difficulty trivial — slightly above a coin flip — so
// new players aren't trapped by an unlucky string of failures while
// the rest of the combat loop is still being built out. Future
// iterations can tune it per-foe (Spot vs Hide / opposed Reflex).
const fleeDC = 10

// FleeMover is the cmd-layer implementation of combat.FleeMover. It
// owns exit selection, the d20 roll, the room-transition broadcasts,
// and the session/repo write-back. Combat invokes it from
// resolveAction without holding the manager lock.
type FleeMover struct {
	rooms      repo.RoomRepo
	exits      repo.ExitRepo
	items      repo.ItemRepo
	mobs       repo.MobInstanceRepo
	characters repo.CharacterRepo
	sessions   *session.Registry
	bus        *eventbus.Bus
	clock      *world.Clock

	rngMu sync.Mutex
	rng   *rand.Rand
}

// NewFleeMover wires the dependencies. rng may be nil — a
// time-seeded source is used. Tests inject a deterministic source via
// SetRNG.
func NewFleeMover(
	rooms repo.RoomRepo,
	exits repo.ExitRepo,
	items repo.ItemRepo,
	mobs repo.MobInstanceRepo,
	characters repo.CharacterRepo,
	sessions *session.Registry,
	bus *eventbus.Bus,
	clock *world.Clock,
	rng *rand.Rand,
) *FleeMover {
	if rng == nil {
		// Mirror combat.Manager.New: time-seeded default so production
		// boots with unpredictable rolls rather than a hard-coded
		// sequence. Tests pass an explicit deterministic source.
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &FleeMover{
		rooms:      rooms,
		exits:      exits,
		items:      items,
		mobs:       mobs,
		characters: characters,
		sessions:   sessions,
		bus:        bus,
		clock:      clock,
		rng:        rng,
	}
}

// SetRNG injects a deterministic random source. Tests use this; the
// constructor wires a time-seeded default otherwise.
func (m *FleeMover) SetRNG(r *rand.Rand) {
	if r == nil {
		return
	}
	m.rngMu.Lock()
	defer m.rngMu.Unlock()
	m.rng = r
}

// AttemptFlee is combat.FleeMover.AttemptFlee. Steps:
//
//  1. List exits, drop Hidden/NoPass/Closed/Locked. Empty → no_exits.
//  2. Roll d20 + DexMod vs fleeDC. Failure → rolled_failure.
//  3. Pick a random eligible exit, perform the move (broadcasts,
//     session move for chars, repo write-back for chars+mobs, render
//     the new room for chars).
func (m *FleeMover) AttemptFlee(ctx context.Context, roomID int64, actor combat.ActorRef) combat.FleeResult {
	exits, err := m.exits.ListFrom(ctx, roomID)
	if err != nil {
		slog.Warn("flee: list exits failed", "room", roomID, "error", err)
		return combat.FleeResult{Reason: "no_exits"}
	}
	eligible := eligibleFleeExits(exits)
	if len(eligible) == 0 {
		return combat.FleeResult{Reason: "no_exits"}
	}

	dexMod, ok := m.actorDexMod(ctx, actor)
	if !ok {
		return combat.FleeResult{Reason: "rolled_failure"}
	}

	m.rngMu.Lock()
	raw := m.rng.Intn(20) + 1
	idx := m.rng.Intn(len(eligible))
	m.rngMu.Unlock()

	if raw+int(dexMod) < fleeDC {
		return combat.FleeResult{Reason: "rolled_failure"}
	}

	choice := eligible[idx]
	actorName := m.actorName(ctx, actor)

	// Source-room broadcast. WriteAsync via broadcastRoom — the actor
	// has no Session in their hand here (resolution runs in the tick
	// goroutine, not the dispatcher), so we exclude by character id
	// inside the helper for the character case. broadcastRoom takes
	// an `except *telnet.Session`; we pass the actor's session if
	// they have one bound, else nil (mob flee).
	actorSess := m.sessionForActor(actor)
	broadcastRoom(m.sessions, roomID, actorSess,
		"{{"+actorName+" flees "+choice.Direction+"!}}::yellow\r\n")

	switch actor.Kind {
	case combat.ActorKindCharacter:
		// Event ordering matches internal/cmd/move.go::moveDir:
		// PlayerLeft before SetCurrentRoom so a subscriber that reads
		// session.CurrentRoomID still sees the source room; then
		// SetCurrentRoom + RecordRoom; then PlayerEntered. Diverging
		// from move.go's order silently breaks any subscriber that
		// depends on the published-while-still-in-source-room beat.
		if m.bus != nil {
			m.bus.Publish(ctx, world.PlayerLeft{
				CharacterID: actor.ID,
				FromRoomID:  roomID,
				ToRoomID:    choice.ToRoomID,
			})
		}
		if actorSess != nil {
			actorSess.SetCurrentRoom(choice.ToRoomID)
			if err := actorSess.WriteAsync("{{You slip away to the " + choice.Direction + ".}}::yellow\r\n"); err != nil {
				slog.Debug("flee: actor write failed", "char", actor.ID, "error", err)
			}
		}
		if err := m.characters.RecordRoom(ctx, actor.ID, choice.ToRoomID); err != nil {
			slog.Warn("flee: char RecordRoom failed", "char", actor.ID, "to", choice.ToRoomID, "error", err)
		}
		if m.bus != nil {
			m.bus.Publish(ctx, world.PlayerEntered{
				CharacterID: actor.ID,
				FromRoomID:  roomID,
				ToRoomID:    choice.ToRoomID,
			})
		}
		if actorSess != nil {
			if err := RenderRoom(ctx, actorSess, m.rooms, m.exits, m.items, m.mobs, m.clock); err != nil {
				slog.Debug("flee: render dest failed", "char", actor.ID, "error", err)
			}
		}
	case combat.ActorKindMob:
		if err := m.mobs.UpdateRoom(ctx, actor.ID, choice.ToRoomID); err != nil {
			slog.Warn("flee: mob UpdateRoom failed", "mob", actor.ID, "to", choice.ToRoomID, "error", err)
			return combat.FleeResult{Reason: "rolled_failure"}
		}
	}

	// Destination-room arrival broadcast. Exclude the actor's session
	// so they don't get the third-person line on top of their own
	// "you slip away" feedback.
	broadcastRoom(m.sessions, choice.ToRoomID, actorSess,
		"{{"+actorName+" arrives, panting.}}::yellow\r\n")

	return combat.FleeResult{
		Success:   true,
		Direction: choice.Direction,
		ToRoomID:  choice.ToRoomID,
		Reason:    "moved",
	}
}

// eligibleFleeExits filters out exits the actor can't physically
// move through. Hidden exits stay hidden; locked / closed doors
// don't open under stress; NoPass means engineered impassability.
func eligibleFleeExits(exits []repo.Exit) []repo.Exit {
	out := make([]repo.Exit, 0, len(exits))
	for _, e := range exits {
		if e.Flags.Hidden || e.Flags.NoPass || e.Flags.Closed || e.Flags.Locked {
			continue
		}
		out = append(out, e)
	}
	return out
}

// actorDexMod loads the actor's creature.Core via the matching repo
// and returns its DexMod. Failure (logged-out / despawned / repo
// transient) is treated as "can't roll" so the fight can recover —
// the actor stays in the order and may try again next turn.
func (m *FleeMover) actorDexMod(ctx context.Context, actor combat.ActorRef) (int16, bool) {
	switch actor.Kind {
	case combat.ActorKindCharacter:
		ch, err := m.characters.GetByID(ctx, actor.ID)
		if err != nil {
			slog.Warn("flee: char lookup failed", "char", actor.ID, "error", err)
			return 0, false
		}
		return ch.Core.Abilities.DexMod(), true
	case combat.ActorKindMob:
		mob, err := m.mobs.GetByID(ctx, actor.ID)
		if err != nil {
			slog.Warn("flee: mob lookup failed", "mob", actor.ID, "error", err)
			return 0, false
		}
		return mob.Core.Abilities.DexMod(), true
	}
	return 0, false
}

// actorName returns a display name for room broadcasts. Falls back
// to "Someone" / "A creature" so a half-loaded actor never emits an
// empty {{...}} block.
func (m *FleeMover) actorName(ctx context.Context, actor combat.ActorRef) string {
	switch actor.Kind {
	case combat.ActorKindCharacter:
		ch, err := m.characters.GetByID(ctx, actor.ID)
		if err == nil {
			return safeName(ch.Core.Name, "Someone")
		}
		return "Someone"
	case combat.ActorKindMob:
		mob, err := m.mobs.GetByID(ctx, actor.ID)
		if err == nil {
			return safeName(mob.Core.Name, "A creature")
		}
		return "A creature"
	}
	return "Someone"
}

// sessionForActor returns the bound *telnet.Session for a character
// actor, or nil for mobs / unbound characters. Scans the registry by
// CharacterID — the registry is keyed by AccountID so there is no
// O(1) char-id lookup today; the session count is bounded by online
// players so this is fine for V1.
func (m *FleeMover) sessionForActor(actor combat.ActorRef) *telnet.Session {
	if actor.Kind != combat.ActorKindCharacter || m.sessions == nil {
		return nil
	}
	for _, peer := range m.sessions.Snapshot() {
		charID, _, _ := peer.InWorld()
		if charID == actor.ID {
			return peer
		}
	}
	return nil
}
