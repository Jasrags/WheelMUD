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
	"errors"
	"fmt"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/progression"
	"github.com/Jasrags/WheelMUD/internal/repo"
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

	// §19 closer — drop-on-death pipeline. When the server flag is
	// enabled and the items repo is wired (production has it; some
	// tests don't), dump carried coin + top-level inventory + equipped
	// items into a player-corpse in the death room before the room
	// move. When the drop actually fires, the 10% XP-debt delta is
	// waived: gear/coin loss replaces XP debt as the death cost. The
	// flag read is fenced under m.mu so SetDropOnDeath's write is
	// observed without relying on the bool's set-once-at-boot pattern.
	m.mu.RLock()
	dropEnabled := m.dropOnDeath
	m.mu.RUnlock()
	var corpseID int64
	var dropped bool
	if dropEnabled && m.items != nil {
		corpseID, dropped = m.dropCharacterLoot(ctx, ch)
	}

	// Compute the new debt (delta added on top of any existing).
	// Skipped when the drop fired — gear/coin loss is the cost.
	curLevel := progression.LevelForXP(ch.XP)
	var debtDelta int64
	if !dropped {
		debtDelta = DeathDebt(ch.XP, curLevel)
	}
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

	// Mark dead in the fight + snapshot the per-attacker damage
	// tally under the lock so a parallel resolveAction can't observe
	// a half-cleared state. The tally drives the §19 PvP XP award
	// below; mirrors the same critical-section shape as
	// handleMobDeath:88-101.
	m.mu.Lock()
	var tallySnap map[ActorRef]int32
	if f, ok := m.fights[deathRoomID]; ok {
		if f.Dead == nil {
			f.Dead = make(map[ActorRef]struct{})
		}
		f.Dead[victim] = struct{}{}
		if len(f.DamageTally) > 0 {
			tallySnap = make(map[ActorRef]int32, len(f.DamageTally))
			for k, v := range f.DamageTally {
				tallySnap[k] = v
			}
		}
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
			CorpseID:    corpseID,
		})
		m.bus.Publish(ctx, CharacterRespawned{
			PrevRoomID: deathRoomID,
			RoomID:     boundRoomID,
			Character:  victim,
		})
	}

	// Phase D §19 PvP XP award. Non-combat deaths (HandleAffectDeath
	// passes ActorRef{} as killer) and empty-tally edges (no Fight
	// covered this room) skip the award path entirely. Verb-layer
	// gates (nopvp room, newbie cap, opt-in, same-group) have
	// already refused illegitimate kills before they could land
	// here, so the only anti-farm guard at this layer is the
	// level-differential clamp inside pvpXPForKill.
	if killer.Kind != ActorKindCharacter || len(tallySnap) == 0 {
		return
	}
	attacker, err := m.chars.GetByID(ctx, killer.ID)
	if err != nil {
		slog.Warn("combat: pvp xp attacker lookup failed",
			"attacker", killer.ID, "victim", victim.ID, "error", err)
		return
	}
	totalXP := pvpXPForKill(
		progression.LevelForXP(attacker.XP),
		progression.LevelForXP(ch.XP),
	)
	if totalXP <= 0 {
		return
	}
	// Strip the victim from the tally. A self-damage corner case
	// (poison reflect, channeled-while-injured, etc.) must not
	// credit XP back to the dying character.
	delete(tallySnap, victim)
	m.creditXPShares(ctx, deathRoomID, killer, victim, tallySnap, totalXP)
}

// dropCharacterLoot runs the §19 drop-on-death pipeline for ch: spawn a
// player-corpse in the death room, transfer top-level inventory +
// equipped items into the corpse, drop a coin pile for carried coin,
// then clear the character row's coin (bank preserved) and equipment.
// Returns the corpse id (0 if the spawn fizzled) and a "dropped" flag
// the caller uses to waive XP debt. Best-effort throughout — a repo
// hiccup logs and continues so the player still respawns.
//
// Bank coin is intentionally preserved (the safe-deposit escape hatch).
// Nested container items follow their container automatically because
// TransferOwnerToContainer moves only the top-level item; children
// keep their parent_item_id.
func (m *Manager) dropCharacterLoot(ctx context.Context, ch repo.Character) (int64, bool) {
	if m.items == nil || m.chars == nil || ch.CurrentRoomID == 0 {
		return 0, false
	}

	corpseID := m.spawnPlayerCorpse(ctx, ch)
	if corpseID == 0 {
		// Best-effort: if the corpse spawn fails, fall back to the
		// keep-inventory path rather than leaving loot orphaned.
		return 0, false
	}

	// Inventory: top-level only. Items inside containers move with
	// their container when we transfer the container itself.
	m.transferCharacterInventory(ctx, ch.ID, corpseID)

	// Equipment: clear the slot map after each equipped item id is
	// moved into the corpse. Slot bookkeeping is JSON metadata;
	// `owner_character_id` is the source of truth, so a single
	// RecordEquipment with the zero value is enough alongside the
	// per-item transfers.
	m.transferCharacterEquipment(ctx, ch.ID, ch.Equipment, corpseID)

	// Carried coin → trade-good pile inside the corpse. Bank coin is
	// preserved. RecordCoin uses CoinVersion optimistic-lock; one
	// retry on ErrCoinConflict mirrors the quest path.
	m.dropCarriedCoin(ctx, ch, corpseID)

	return corpseID, true
}

// spawnPlayerCorpse mirrors spawnCorpse's structure for a character.
// Uses the same corpseDecayDuration so player and mob bodies behave
// identically in the Decayer queue.
func (m *Manager) spawnPlayerCorpse(ctx context.Context, ch repo.Character) int64 {
	name := ch.Name
	if name == "" {
		name = "an unknown soul"
	}
	// Capture m.now() once so deadline and ExternalID share the same
	// timestamp — matches the corpseExternalID pattern in mob_death.go
	// and keeps tests with an injected fixed clock deterministic.
	now := m.now()
	deadline := now.Add(corpseDecayDuration)
	corpse := repo.Item{
		ExternalID: fmt.Sprintf("pcorpse-%d-%d", ch.ID, now.UnixNano()),
		Name:       "corpse of " + name,
		ShortDesc:  "The corpse of " + name + " lies here.",
		RoomID:     ch.CurrentRoomID,
		Type:       repo.ItemTypeContainer,
		Stats: &repo.ContainerStats{
			CapacityLbs:  500,
			CapacityCuFt: 50,
		},
		DecayExpiresAt: &deadline,
	}
	created, err := m.items.Create(ctx, corpse)
	if err != nil {
		slog.Warn("combat: player corpse spawn failed",
			"char", ch.ID, "room", ch.CurrentRoomID, "error", err)
		return 0
	}
	if m.decayer != nil {
		m.decayer.Schedule(created.ID, created.RoomID, deadline)
	}
	return created.ID
}

// transferCharacterInventory walks ch's top-level inventory (filtering
// out nested items, which follow their parent container automatically)
// and TransferOwnerToContainer-moves each one into the corpse. Best-
// effort: an ErrItemMoved (concurrent get/give) on a single item logs
// and continues.
func (m *Manager) transferCharacterInventory(ctx context.Context, charID, corpseID int64) {
	owned, err := m.items.ListAllOwnedTransitive(ctx, charID)
	if err != nil {
		slog.Warn("combat: drop-on-death inventory list failed",
			"char", charID, "error", err)
		return
	}
	for _, it := range owned {
		if it.ParentItemID != 0 {
			// Nested inside a container; will follow its parent.
			continue
		}
		if err := m.items.TransferOwnerToContainer(ctx, it.ID, charID, corpseID); err != nil {
			slog.Warn("combat: drop-on-death inventory transfer failed",
				"char", charID, "item", it.ID, "corpse", corpseID, "error", err)
		}
	}
}

// transferCharacterEquipment moves each equipped item id into the
// corpse and clears the character's slot map. Equipped items are
// owned by the character (the slot map is metadata), so the same
// TransferOwnerToContainer call as inventory applies.
//
// Note: a previously-equipped item may have already been transferred
// by transferCharacterInventory (it's owned by the character). In
// that case TransferOwnerToContainer returns ErrItemMoved because the
// guard sees parent_item_id already set; we log + ignore. The slot
// clear via RecordEquipment is what actually drops the metadata.
func (m *Manager) transferCharacterEquipment(ctx context.Context, charID int64, eq creature.Equipment, corpseID int64) {
	for _, slotID := range equippedItemIDs(eq) {
		if slotID == 0 {
			continue
		}
		if err := m.items.TransferOwnerToContainer(ctx, slotID, charID, corpseID); err != nil {
			// ErrItemMoved is expected when the inventory pass already
			// transferred this id — slot map duplicates the ownership.
			// Anything else is a real failure worth logging.
			if !errors.Is(err, repo.ErrItemMoved) {
				slog.Warn("combat: drop-on-death equipment transfer failed",
					"char", charID, "item", slotID, "corpse", corpseID, "error", err)
			}
		}
	}
	if err := m.chars.RecordEquipment(ctx, charID, creature.Equipment{}); err != nil {
		slog.Warn("combat: drop-on-death equipment clear failed",
			"char", charID, "error", err)
	}
}

// equippedItemIDs flattens the Equipment struct into the union of
// scalar slots + array slots, skipping zero ids.
func equippedItemIDs(eq creature.Equipment) []int64 {
	ids := []int64{
		eq.Armor, eq.Shield, eq.PrimaryWield, eq.OffHand,
		eq.Outfit, eq.Cloak, eq.Backpack, eq.HeldInHand, eq.Mount,
	}
	ids = append(ids, eq.BeltPouches...)
	ids = append(ids, eq.WornMisc...)
	return ids
}

// dropCarriedCoin zeroes the character's carried coin (bank preserved)
// and, only on a successful write, spawns a matching TradeGood pile
// inside the corpse. Ordering matters: spawning the pile first and
// then failing to zero the row would duplicate value (player keeps the
// coin AND the pile exists). One retry on ErrCoinConflict — mirrors
// the quest reward path documented in CLAUDE.md.
func (m *Manager) dropCarriedCoin(ctx context.Context, ch repo.Character, corpseID int64) {
	if ch.Coin <= 0 {
		return
	}
	pileAmount := ch.Coin
	err := m.chars.RecordCoin(ctx, ch.ID, 0, ch.BankBalance, ch.CoinVersion)
	if errors.Is(err, repo.ErrCoinConflict) {
		// A concurrent transfer changed the version; re-read so the
		// pile reflects the actual coin we're about to remove.
		fresh, ferr := m.chars.GetByID(ctx, ch.ID)
		if ferr != nil {
			slog.Warn("combat: drop-on-death coin reread failed",
				"char", ch.ID, "error", ferr)
			return
		}
		pileAmount = fresh.Coin
		if pileAmount <= 0 {
			return
		}
		err = m.chars.RecordCoin(ctx, ch.ID, 0, fresh.BankBalance, fresh.CoinVersion)
	}
	if err != nil {
		slog.Warn("combat: drop-on-death coin clear failed",
			"char", ch.ID, "error", err)
		return
	}
	m.spawnCoinPile(ctx, corpseID, pileAmount)
}
