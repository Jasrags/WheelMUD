package combat

// handleCharacterDeath runs the Phase D §19 player-death pipeline:
//
//  1. Snapshot the dying character so we have their pre-death XP /
//     level / BoundRoomID without an extra repo round-trip later.
//  2. Compute the XP-debt delta via DeathDebt(curXP, curLevel).
//  3. Persist the new debt (RecordXPDebt) and the respawn vitals
//     (RecordCore: full HP, clear CondDying|CondUnconscious, clear
//     position_flags) and the room move (RecordRoom to BoundRoomID).
//     Per-step failures log + continue — the contract is "the player
//     respawns even if a piece fizzles", same posture as
//     handleMobDeath.
//  4. Mark the character Dead in the fight under m.mu so the next
//     tickRoom call prunes them from Order.
//  5. Publish CharacterDied (death room context) then
//     CharacterRespawned (bound room context) so cmd-layer
//     subscribers can broadcast peer messages and stamp the session's
//     in-world room id.
//
// All repo IO runs outside m.mu (same rule as handleMobDeath). The
// only critical section is the Fight.Dead map mark.

import (
	"context"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/progression"
)

// respawnConditionMask is the bitmask cleared on respawn. Death-
// related conditions go away; everything else (Fatigue, Staggered,
// etc.) survives so the player feels the lingering effects of being
// brought back. ^x0 inverts to "preserve everything but these".
const respawnConditionMask creature.Condition = creature.CondDying |
	creature.CondUnconscious

// HandleAffectDeath drives the §19 player-death pipeline for a victim
// killed by an out-of-combat affect tick (Phase E #25 slice 2 — DoT
// poison/bleed). Killer is empty (no attribution). Safe to call when
// no Fight covers the victim's room — the marker step no-ops.
func (m *Manager) HandleAffectDeath(ctx context.Context, characterID int64) {
	m.handleCharacterDeath(ctx, ActorRef{}, ActorRef{Kind: ActorKindCharacter, ID: characterID})
}

func (m *Manager) handleCharacterDeath(ctx context.Context, killer, victim ActorRef) {
	if m.chars == nil {
		// Defensive: a manager wired without a character repo can't
		// run this pipeline. Mark dead so pruneDead clears Order on
		// the next tick; the player just stays in the room.
		m.markDeadAllRooms(victim)
		return
	}

	ch, err := m.chars.GetByID(ctx, victim.ID)
	if err != nil {
		slog.Warn("combat: dead character lookup failed",
			"char", victim.ID, "error", err)
		m.markDeadAllRooms(victim)
		return
	}

	deathRoomID := ch.CurrentRoomID
	boundRoomID := ch.BoundRoomID
	if boundRoomID == 0 {
		// Defensive: chargen + 0009 default this to StarterRoomID,
		// but a corrupt row shouldn't strand the respawn at room 0.
		boundRoomID = deathRoomID
	}

	// Compute the new debt (delta added on top of any existing).
	curLevel := progression.LevelForXP(ch.XP)
	debtDelta := DeathDebt(ch.XP, curLevel)
	newDebt := ch.XPDebt + debtDelta

	// Persist debt first — it's the most player-visible side effect
	// and the cheapest UPDATE.
	if debtDelta > 0 {
		if err := m.chars.RecordXPDebt(ctx, victim.ID, newDebt); err != nil {
			slog.Warn("combat: xp debt write-back failed",
				"char", victim.ID, "error", err)
		}
	}

	// Heal + clear death conditions. Non-death conditions survive so
	// the player still feels Fatigue / Staggered / etc. Position flags
	// are reset to 0 (clears FlatFooted from the dying turn so the
	// respawned character isn't immediately combat-disadvantaged).
	newConditions := ch.Core.Conditions &^ respawnConditionMask
	if err := m.chars.RecordCore(ctx, victim.ID,
		ch.Core.HPMax, 0, newConditions, 0,
	); err != nil {
		slog.Warn("combat: respawn vitals write-back failed",
			"char", victim.ID, "error", err)
	}

	// Move the row to the bound room. Cmd-layer subscriber stamps
	// the live session via Session.SetInWorld off the
	// CharacterRespawned event below.
	if deathRoomID != boundRoomID {
		if err := m.chars.RecordRoom(ctx, victim.ID, boundRoomID); err != nil {
			slog.Warn("combat: respawn room write-back failed",
				"char", victim.ID, "error", err)
		}
	}

	// Mark dead in the fight + capture the active fight ref under
	// the lock so a parallel resolveAction can't observe a half-
	// cleared state. The fight that contained the victim is keyed
	// off the death room.
	m.mu.Lock()
	if f, ok := m.fights[deathRoomID]; ok {
		if f.Dead == nil {
			f.Dead = make(map[ActorRef]struct{})
		}
		f.Dead[victim] = struct{}{}
	}
	m.mu.Unlock()

	// Publish events. Death first (subscribers in the death room
	// broadcast "X falls dead!"), respawn second (subscribers in the
	// bound room broadcast "X appears, eyes hollow." and stamp the
	// victim's own session). The order matters for the dying
	// player's screen — they see "You die!" before the bound-room
	// look.
	if m.bus != nil {
		m.bus.Publish(ctx, CharacterDied{
			DeathRoomID: deathRoomID,
			Victim:      victim,
			Killer:      killer,
			BoundRoomID: boundRoomID,
			XPDebtAdded: debtDelta,
		})
		m.bus.Publish(ctx, CharacterRespawned{
			PrevRoomID: deathRoomID,
			RoomID:     boundRoomID,
			Character:  victim,
		})
	}
}
