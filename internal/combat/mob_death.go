package combat

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// corpseDecayDuration is the V1 lifetime of an empty corpse container
// before the Decayer sweeps it. Five minutes mirrors classic Diku-
// family servers and gives a player time to loot once looting verbs
// land. Held as a constant so the decay timing is grep-stable; future
// per-template / per-zone overrides can layer on without touching
// callers.
const corpseDecayDuration = 5 * time.Minute

// handleMobDeath runs the slice-1 mob death pipeline:
//
//  1. Resolve the mob row + template (under no lock).
//  2. Spawn a corpse, transfer mob inventory into it, drop a rolled
//     gold pile inside it. All three steps are best-effort.
//  3. Despawn the mob (clear room → delete).
//  4. Snapshot the per-attacker damage tally and mark the mob dead so
//     the next tickRoom call prunes it from Order.
//  5. Award XP weighted by damage to character attackers (group-aware,
//     debt-draining); publish CombatXPAwarded per share.
//  6. Publish CombatDeath last so subscribers see the corpse id.
//
// Failures inside any step are logged via slog but never abort the
// pipeline. The contract: once HP ≤ 0 lands, the mob is gone from
// the world by the time tickRoom next fires, even if the corpse or
// XP write-back fizzled.
func (m *Manager) handleMobDeath(ctx context.Context, killer, victim ActorRef) {
	// Resolve mob + template under no lock — repo IO must not run
	// under m.mu (same rule as tickRoom).
	mob, err := m.mobs.GetByID(ctx, victim.ID)
	if err != nil {
		slog.Warn("combat: dead mob lookup failed",
			"mob", victim.ID, "error", err)
		// Still flag it dead so Order prunes; the body just won't
		// have a corpse. Scope the mark to fights we know about by
		// scanning every room — the mob row is gone so we can't read
		// its CurrentRoomID.
		m.markDeadAllRooms(victim)
		return
	}

	var tmpl creature.MobTemplate
	if m.templates != nil && mob.TemplateID != 0 {
		t, err := m.templates.GetByID(ctx, mob.TemplateID)
		if err != nil {
			slog.Warn("combat: dead mob template lookup failed",
				"mob", victim.ID, "template", mob.TemplateID, "error", err)
		} else {
			tmpl = t
		}
	}

	corpseID := m.spawnCorpse(ctx, mob)
	m.transferLootIntoCorpse(ctx, mob, corpseID)
	m.dropGoldPile(ctx, corpseID, tmpl)

	// Despawn: clear room first (records a final trail row, frees
	// any presence-keyed lookups) then delete. Failure of either
	// path is logged but doesn't gate the rest of the kill.
	if err := m.mobs.UpdateRoom(ctx, victim.ID, 0); err != nil {
		slog.Warn("combat: mob room-clear failed",
			"mob", victim.ID, "error", err)
	}
	if err := m.mobs.Delete(ctx, victim.ID); err != nil {
		slog.Warn("combat: mob delete failed",
			"mob", victim.ID, "error", err)
	}

	// Snapshot the tally + mark dead under the lock so a parallel
	// resolveAction on the same fight doesn't see a half-cleared
	// state.
	roomID := mob.Core.CurrentRoomID
	m.mu.Lock()
	var tallySnap map[ActorRef]int32
	if f, ok := m.fights[roomID]; ok {
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

	m.awardKillXP(ctx, roomID, killer, victim, tallySnap, tmpl)

	if m.bus != nil {
		m.bus.Publish(ctx, CombatDeath{
			RoomID:                roomID,
			Victim:                victim,
			Killer:                killer,
			CorpseID:              corpseID,
			MobTemplateID:         tmpl.ID,
			MobTemplateExternalID: tmpl.ExternalID,
		})
	}
}

// transferLootIntoCorpse moves the mob's inventory into the freshly
// spawned corpse so `look in corpse` / `get from corpse` show real
// loot (Phase D §19 polish). Best-effort per item — a single SetParent
// failure logs and continues so the despawn still resolves cleanly.
// Skipped when the corpse spawn fizzled (corpseID == 0) or the item
// repo is unavailable.
func (m *Manager) transferLootIntoCorpse(ctx context.Context, mob creature.MobInstance, corpseID int64) {
	if corpseID == 0 || m.items == nil {
		return
	}
	for _, itemID := range mob.Inventory {
		if itemID == 0 {
			continue
		}
		if err := m.items.SetParent(ctx, itemID, corpseID); err != nil {
			slog.Warn("combat: corpse loot transfer failed",
				"mob", mob.ID, "item", itemID, "corpse", corpseID, "error", err)
		}
	}
}

// dropGoldPile rolls the mob's GoldDice and spawns a coin pile inside
// the corpse (Phase D §19 polish). Best-effort — empty / malformed
// dice strings just don't produce a pile. The roll holds rngMu so
// concurrent fights don't share the *rand.Rand mutably.
func (m *Manager) dropGoldPile(ctx context.Context, corpseID int64, tmpl creature.MobTemplate) {
	if corpseID == 0 || tmpl.GoldDice == "" {
		return
	}
	m.rngMu.Lock()
	amt, ok := rollDice(m.rng, tmpl.GoldDice)
	m.rngMu.Unlock()
	if !ok || amt <= 0 {
		return
	}
	m.spawnCoinPile(ctx, corpseID, currency.Amount(amt))
}

// awardKillXP runs the group-aware, debt-draining XP award loop for
// every character contributor in the damage tally and publishes one
// CombatXPAwarded per share.
//
// Phase D #22 slice 4: each character contributor's tally is expanded
// across their in-room group teammates so kill XP shares with the
// party. Mob and unknown-kind contributors pass through unchanged. A
// nil resolver short-circuits to the solo path.
//
// Phase D §19: outstanding XP debt is drained off the top of each
// gross share before the player is credited. `gain` is the net XP
// added to the row; `paid` is the share that went to debt (surfaced
// on the event for the audit / log line).
//
// TOCTOU: this is two non-atomic UPDATEs (RecordXP + RecordXPDebt)
// computed off a GetByID snapshot. Safe under single-session-per-
// account because no other path mutates these columns concurrently.
// When multi-session lands, swap to a single CAS-style UPDATE keyed
// off a version token (mirroring coin_version / 0032). Tracked in the
// existing optimistic_lock_followups / progression_24_followups memos.
func (m *Manager) awardKillXP(ctx context.Context, roomID int64, killer, victim ActorRef, tally map[ActorRef]int32, tmpl creature.MobTemplate) {
	m.mu.Lock()
	resolver := m.groupShare
	m.mu.Unlock()
	tally = expandTallyByGroup(tally, roomID, resolver)
	awards := allocateXP(tally, xpValueForTemplate(tmpl), killer)
	for ref, amount := range awards {
		if ref.Kind != ActorKindCharacter || amount <= 0 {
			continue
		}
		ch, err := m.chars.GetByID(ctx, ref.ID)
		if err != nil {
			slog.Warn("combat: xp recipient lookup failed",
				"char", ref.ID, "error", err)
			continue
		}
		gain, newDebt := ApplyXPAward(amount, ch.XPDebt)
		paid := ch.XPDebt - newDebt
		if gain > 0 {
			if err := m.chars.RecordXP(ctx, ref.ID, ch.XP+gain); err != nil {
				slog.Warn("combat: xp write-back failed",
					"char", ref.ID, "error", err)
				continue
			}
		}
		if paid > 0 {
			if err := m.chars.RecordXPDebt(ctx, ref.ID, newDebt); err != nil {
				slog.Warn("combat: xp debt write-back failed",
					"char", ref.ID, "error", err)
				// Continue: the gross gain (if any) already
				// landed; the debt counter just stays high. The
				// player gets the next chance to drain it.
			}
		}
		if m.bus != nil {
			m.bus.Publish(ctx, CombatXPAwarded{
				RoomID:    roomID,
				Awardee:   ref,
				Amount:    gain,
				DebtTaken: paid,
				Killed:    victim,
			})
		}
	}
}

// markDeadAllRooms is the lock-acquiring helper used when
// handleMobDeath can't load the mob row (so its CurrentRoomID is
// unknown) but still needs Order pruning to happen on the next tick.
// Scans every active fight; pruneDead is idempotent so over-marking
// is harmless. Prefer the room-scoped path inside the snapshot
// section of handleMobDeath when the room id is known.
func (m *Manager) markDeadAllRooms(victim ActorRef) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range m.fights {
		if f.Dead == nil {
			f.Dead = make(map[ActorRef]struct{})
		}
		f.Dead[victim] = struct{}{}
	}
}

// spawnCorpse creates a container item in the mob's room and returns
// its id. Returns 0 when the item repo is unavailable or Create
// fails — the death pipeline tolerates a body-less kill rather than
// rolling back the despawn.
func (m *Manager) spawnCorpse(ctx context.Context, mob creature.MobInstance) int64 {
	if m.items == nil || mob.Core.CurrentRoomID == 0 {
		return 0
	}
	name := mob.Core.Name
	if name == "" {
		name = "creature"
	}
	corpse := repo.Item{
		ExternalID: m.corpseExternalID(mob),
		Name:       "corpse of " + name,
		ShortDesc:  "The corpse of " + name + " lies here.",
		RoomID:     mob.Core.CurrentRoomID,
		Type:       repo.ItemTypeContainer,
		Stats: &repo.ContainerStats{
			CapacityLbs:  500,
			CapacityCuFt: 50,
		},
	}
	created, err := m.items.Create(ctx, corpse)
	if err != nil {
		slog.Warn("combat: corpse spawn failed",
			"mob", mob.ID, "room", mob.Core.CurrentRoomID, "error", err)
		return 0
	}
	if m.decayer != nil {
		m.decayer.Schedule(created.ID, created.RoomID, m.now().Add(corpseDecayDuration))
	}
	return created.ID
}

// spawnCoinPile drops a TradeGood "coin pile" inside a corpse worth
// the rolled coin amount (Phase D §19 polish). Best-effort: a repo
// failure logs and returns 0 — the kill still resolves and the
// corpse just doesn't have a coin entry. ExternalID embeds the
// corpse id and a nano timestamp so a kill that re-uses a recycled
// corpse id doesn't collide.
func (m *Manager) spawnCoinPile(ctx context.Context, corpseID int64, amount currency.Amount) int64 {
	if m.items == nil || corpseID == 0 || amount <= 0 {
		return 0
	}
	pile := repo.Item{
		ExternalID:   fmt.Sprintf("coin-pile-%d-%d", corpseID, m.now().UnixNano()),
		Name:         "a small pile of coins",
		ShortDesc:    "A small pile of coins lies here.",
		ParentItemID: corpseID,
		Type:         repo.ItemTypeTradeGood,
		Value:        amount,
		Flags:        repo.FlagTradeGood,
	}
	created, err := m.items.Create(ctx, pile)
	if err != nil {
		slog.Warn("combat: coin pile spawn failed",
			"corpse", corpseID, "amount", amount, "error", err)
		return 0
	}
	return created.ID
}

// corpseExternalID builds a unique ExternalID for the corpse so the
// items.external_id UNIQUE constraint can't bite a back-to-back kill
// of the same mob name. Format: corpse-<mobid>-<unix-nano>. The mob
// id pins the lineage; the timestamp suffix dedupes within a single
// mob's instance lifetime if the row was somehow reused. Uses m.now
// (rather than time.Now directly) so tests with an injected clock
// produce deterministic ids — same pattern as Fight.StartedAt.
func (m *Manager) corpseExternalID(mob creature.MobInstance) string {
	return "corpse-" + strconv.FormatInt(mob.ID, 10) + "-" +
		strconv.FormatInt(m.now().UTC().UnixNano(), 10)
}
