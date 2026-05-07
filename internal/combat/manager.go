package combat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// ErrFightExists is returned by Start when the room already has an
// active Fight. Callers should call End first or join the existing
// fight (slice 2 will add a Join helper).
var ErrFightExists = errors.New("combat: fight already active in this room")

// ErrNoParticipants is returned by Start when the participant list
// is empty. A fight needs at least one combatant for Tick to make
// progress; in practice the caller will pass at least two.
var ErrNoParticipants = errors.New("combat: at least one participant required")

// ErrFightNotFound is returned by End / Get when the room has no
// active fight.
var ErrFightNotFound = errors.New("combat: no fight in this room")

// Manager owns the per-process map of active Fights. It is the
// single source of truth for combat state — every verb (slice 2+)
// reads and writes through this interface. Concurrent-safe.
type Manager struct {
	mu     sync.RWMutex
	fights map[int64]*Fight // roomID → fight

	bus       *eventbus.Bus
	chars     repo.CharacterRepo
	mobs      repo.MobInstanceRepo
	templates repo.MobTemplateRepo // optional; nil disables XP rewards
	items     repo.ItemRepo        // optional; nil disables weapon-stat lookup + corpse spawn
	decayer    *Decayer            // optional; nil leaves corpses lingering until admin purge
	fleeMover  FleeMover           // optional; nil makes ActionFlee fail with reason="no_mover"
	groupShare GroupResolver       // optional; nil = solo split (each kill credits the dealer only)

	rngMu sync.Mutex
	rng   *rand.Rand // injectable for tests
	now   func() time.Time
}

// New constructs a Manager. bus is required (events are how downstream
// systems learn about fights); chars / mobs are required for
// participant resolution at Start time and the auto-end check at
// Tick time. templates and items are optional; tests that don't
// exercise the death / corpse / XP path can pass nil and the
// resolver falls back to safe no-ops (no corpse, no XP, mob still
// despawns).
func New(bus *eventbus.Bus, chars repo.CharacterRepo, mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo, items repo.ItemRepo) *Manager {
	return &Manager{
		fights:    make(map[int64]*Fight),
		bus:       bus,
		chars:     chars,
		mobs:      mobs,
		templates: templates,
		items:     items,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
		now:       time.Now,
	}
}

// SetDecayer wires the corpse decay queue. Optional — when unset the
// death pipeline still spawns corpses but they linger forever (the
// pre-slice-2 behavior). cmd/server/main.go calls this after
// constructing the Decayer alongside the rest of the game-loop deps.
func (m *Manager) SetDecayer(d *Decayer) {
	m.mu.Lock()
	m.decayer = d
	m.mu.Unlock()
}

// SetRNG injects a deterministic random source. Tests use this to
// assert exact initiative orders. Production callers leave the
// time-seeded default.
func (m *Manager) SetRNG(r *rand.Rand) {
	m.rngMu.Lock()
	defer m.rngMu.Unlock()
	m.rng = r
}

// GroupResolver expands a damage-dealer's character id into the set
// of co-located group members (including the dealer themselves)
// who should share that dealer's slice of the kill XP. A nil resolver
// (or a resolver that returns a single-element slice) yields the
// pre-slice-4 solo split.
//
// roomID lets the resolver filter by where the kill happened so a
// teammate AFK in town doesn't piggy-back on a far-away fight.
type GroupResolver func(charID, roomID int64) []int64

// SetGroupResolver wires the group XP-split hook. Optional — when
// unset, kill XP goes 1:1 to the damage dealers. cmd/server/main.go
// passes group.Manager.MembersInRoom.
func (m *Manager) SetGroupResolver(fn GroupResolver) {
	m.mu.Lock()
	m.groupShare = fn
	m.mu.Unlock()
}

// SetClock injects a deterministic time source. Tests use this; the
// constructor wires time.Now by default. Acquires m.mu so a
// concurrent Start can't race the clock swap (the race window is
// test-path-only — production never re-clocks a live Manager — but
// the lock keeps -race quiet either way).
func (m *Manager) SetClock(now func() time.Time) {
	if now == nil {
		return
	}
	m.mu.Lock()
	m.now = now
	m.mu.Unlock()
}

// Get returns the active Fight in roomID, or (nil, false) when no
// fight is in progress. Returned pointer is the Manager's live
// state; callers must not mutate it.
func (m *Manager) Get(roomID int64) (*Fight, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.fights[roomID]
	return f, ok
}

// Active reports whether roomID has an in-progress fight.
func (m *Manager) Active(roomID int64) bool {
	_, ok := m.Get(roomID)
	return ok
}

// Start opens a new fight in roomID. Initiative is rolled for each
// participant — d20 + DexMod + InitMod, ties broken by raw d20 then
// by ActorRef.ID — and the resulting Order is sorted descending.
// Round=0 on return; the first Tick advances to Round=1 and starts
// the head of the order's turn.
//
// Returns ErrFightExists when roomID already has a fight,
// ErrNoParticipants when participants is empty, or a wrapped
// repo lookup error when a participant can't be resolved (the
// fight is not opened).
//
// Publishes CombatStarted on success.
func (m *Manager) Start(ctx context.Context, roomID int64, participants []ActorRef) (*Fight, error) {
	if len(participants) == 0 {
		return nil, ErrNoParticipants
	}

	// Resolve every participant before grabbing the write lock so a
	// slow repo call doesn't block other rooms. Lookup failure means
	// the participant has logged out / been despawned / mistyped — a
	// hard refusal here matches the ItemRepo.Transfer* contract.
	entries := make([]ActorEntry, 0, len(participants))
	for _, ref := range participants {
		core, err := m.resolveCore(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", ref, err)
		}
		entries = append(entries, m.rollInitiative(ref, core))
	}
	sortInitiative(entries)

	m.mu.Lock()
	if _, exists := m.fights[roomID]; exists {
		m.mu.Unlock()
		return nil, ErrFightExists
	}
	f := &Fight{
		RoomID:    roomID,
		Round:     0,
		Order:     entries,
		ActiveIdx: 0,
		StartedAt: m.now(),
	}
	m.fights[roomID] = f
	m.mu.Unlock()

	if m.bus != nil {
		// Snapshot the order so subscribers can't observe later
		// mid-fight rearrangements through this event.
		snapshot := append([]ActorEntry(nil), entries...)
		m.bus.Publish(ctx, CombatStarted{RoomID: roomID, Order: snapshot})
	}
	return f, nil
}

// End closes a fight and removes it from the map. reason is published
// on the CombatEnded event; callers should pass one of the Reason*
// constants. Returns ErrFightNotFound when no fight is active in the
// room.
func (m *Manager) End(ctx context.Context, roomID int64, reason string) error {
	m.mu.Lock()
	if _, ok := m.fights[roomID]; !ok {
		m.mu.Unlock()
		return ErrFightNotFound
	}
	delete(m.fights, roomID)
	m.mu.Unlock()

	if m.bus != nil {
		m.bus.Publish(ctx, CombatEnded{RoomID: roomID, Reason: reason})
	}
	return nil
}

// Tick advances every active fight by one round. Subscribed to
// tick.Buckets.Combat at boot. For each fight:
//
//  1. If fewer than 2 participants are still in the room (anyone
//     logged out, mob despawned, character moved), the fight
//     auto-ends with ReasonNoParticipants and no RoundStarted fires.
//  2. Otherwise Round increments, ActiveIdx wraps to the next
//     position, and RoundStarted publishes for the new active actor.
//
// The participant-presence check uses the same primitives `look`
// uses — RoomID match for characters via repo, ListInRoom for mobs.
// Slow repos delay every fight in the same Tick; subscribers should
// remain cheap (the dispatch is sequential per the
// tick.Bucket contract).
func (m *Manager) Tick(ctx context.Context) {
	m.mu.RLock()
	roomIDs := make([]int64, 0, len(m.fights))
	for id := range m.fights {
		roomIDs = append(roomIDs, id)
	}
	m.mu.RUnlock()

	for _, roomID := range roomIDs {
		m.tickRoom(ctx, roomID)
	}
}

// tickRoom advances a single room's fight. Pulled out so the Tick
// dispatch loop is one-line readable and so a future per-room
// goroutine pool can drop in without restructuring the iteration.
//
// Lock discipline: read-snapshot the fight under RLock, run the
// participant-presence check (which calls into mobs.ListInRoom —
// potentially slow SQL) WITHOUT holding the manager lock, then
// re-acquire to mutate. This keeps one slow room from blocking
// every other Manager call and removes a deadlock hazard if a
// future eventbus subscriber ever calls back into Get/Active.
func (m *Manager) tickRoom(ctx context.Context, roomID int64) {
	m.mu.RLock()
	f, ok := m.fights[roomID]
	if !ok {
		m.mu.RUnlock()
		return
	}
	// Snapshot the order so the presence check can run lock-free.
	// RoomID is immutable on Fight; Order is only mutated inside the
	// write lock so this read is safe.
	roomIDCopy := f.RoomID
	orderCopy := append([]ActorEntry(nil), f.Order...)
	m.mu.RUnlock()

	if !m.fightHasLiveParticipants(ctx, roomIDCopy, orderCopy) {
		m.mu.Lock()
		// Re-check under the write lock — another goroutine may have
		// already ended the fight (e.g. an explicit End or Stop).
		if _, still := m.fights[roomID]; !still {
			m.mu.Unlock()
			return
		}
		delete(m.fights, roomID)
		m.mu.Unlock()
		if m.bus != nil {
			m.bus.Publish(ctx, CombatEnded{RoomID: roomID, Reason: ReasonNoParticipants})
		}
		return
	}

	m.mu.Lock()
	f, ok = m.fights[roomID]
	if !ok {
		// Ended between the unlock and re-lock.
		m.mu.Unlock()
		return
	}
	// Prune actors that died on the previous round before advancing.
	// Done up here (not inline in resolveAction) so ActiveIdx math
	// observes a stable Order for the duration of resolveAction.
	f.pruneDead()
	if len(f.Order) == 0 {
		// Last participant fell — close the fight cleanly. Mirrors
		// the auto-end-on-no-mobs path above; just driven by death
		// rather than presence.
		delete(m.fights, roomID)
		m.mu.Unlock()
		if m.bus != nil {
			m.bus.Publish(ctx, CombatEnded{RoomID: roomID, Reason: ReasonNoParticipants})
		}
		return
	}
	f.Round++
	if f.Round > 1 {
		f.ActiveIdx = (f.ActiveIdx + 1) % len(f.Order)
	}
	round := f.Round
	active := f.Order[f.ActiveIdx].Ref
	pending, hasAction := f.popAction(active)
	m.mu.Unlock()

	if m.bus != nil {
		m.bus.Publish(ctx, RoundStarted{
			RoomID: roomID,
			Round:  round,
			Active: active,
		})
	}

	if hasAction {
		m.resolveAction(ctx, roomID, round, active, pending)
	}
}

// resolveAction is the active actor's slice-1 turn handler. Today it
// handles ActionAttack only; future kinds (flee / weave / kick) slot
// in as additional cases. Errors loading participants or items are
// logged and swallowed — combat continues, the swing just doesn't
// land. This mirrors the spawn / item-transfer "fail-safe, log
// loudly" stance.
func (m *Manager) resolveAction(ctx context.Context, roomID int64, round int, actor ActorRef, action Action) {
	defer func() {
		if m.bus != nil {
			m.bus.Publish(ctx, ActionResolved{
				RoomID: roomID,
				Round:  round,
				Actor:  actor,
				Kind:   action.Kind,
			})
		}
	}()
	switch action.Kind {
	case ActionParry:
		m.resolveParry(ctx, roomID, round, actor)
		return
	case ActionFlee:
		m.resolveFlee(ctx, roomID, actor)
		return
	case ActionAttack:
		// fall through
	default:
		return
	}
	atkCore, err := m.resolveCore(ctx, actor)
	if err != nil {
		slog.Warn("combat: resolve attacker failed",
			"room", roomID, "actor", actor, "error", err)
		return
	}
	defCore, err := m.resolveCore(ctx, action.Target)
	if err != nil {
		// Target gone (fled, despawned, logged out).
		if m.bus != nil {
			m.bus.Publish(ctx, CombatMiss{
				RoomID: roomID, Attacker: actor, Defender: action.Target,
			})
		}
		return
	}
	stats := unarmedDamage
	if action.WeaponID != 0 && m.items != nil {
		it, err := m.items.GetByID(ctx, action.WeaponID)
		if err == nil {
			stats = weaponStatsFor(&it)
		}
	}

	// Snapshot defender FlatFooted flag and parrying stance under the
	// manager lock so the gates observe a stable view; mutations below
	// re-acquire to flip flat-footed / clear parry. Safe to drop the
	// lock before the RNG section because ParryingUntil and
	// FlatFootedUntil are only written by the Manager itself under
	// m.mu.Lock(), so no concurrent writer can tear the snapshot.
	m.mu.Lock()
	defenderFlatFooted := false
	defenderParrying := false
	if f, ok := m.fights[roomID]; ok {
		if v, has := f.FlatFootedUntil[action.Target]; has && v >= round {
			defenderFlatFooted = true
		}
		if v, has := f.ParryingUntil[action.Target]; has && v >= round {
			defenderParrying = true
		}
	}
	m.mu.Unlock()

	m.rngMu.Lock()
	roll := RollAttack(m.rng, atkCore, defCore, stats, defenderFlatFooted)
	parried := false
	parryTotal := 0
	if roll.Hit && defenderParrying {
		parryTotal = RollParry(m.rng, defCore)
		if parryTotal > roll.Total {
			parried = true
		}
	}
	var dealt int32
	if roll.Hit && !parried {
		raw := RollDamage(m.rng, atkCore, stats, roll.IsCrit)
		dealt = applyDamage(&defCore, raw, weaponPrimaryDamageType(stats))
	}
	m.rngMu.Unlock()

	if parried {
		// Stance consumed; attacker is flat-footed for one round.
		m.mu.Lock()
		if f, ok := m.fights[roomID]; ok {
			delete(f.ParryingUntil, action.Target)
			if f.FlatFootedUntil == nil {
				f.FlatFootedUntil = make(map[ActorRef]int)
			}
			f.FlatFootedUntil[actor] = round + 1
		}
		m.mu.Unlock()
		if m.bus != nil {
			m.bus.Publish(ctx, CombatParry{
				RoomID:   roomID,
				Defender: action.Target,
				Attacker: actor,
				Parry:    parryTotal,
				Attack:   roll.Total,
			})
		}
		return
	}

	if !roll.Hit {
		if m.bus != nil {
			m.bus.Publish(ctx, CombatMiss{
				RoomID:    roomID,
				Attacker:  actor,
				Defender:  action.Target,
				RollTotal: roll.Total,
				Defense:   defCore.Defense,
			})
		}
		return
	}

	// Persist mutated HP/subdual back to the live row so subsequent
	// attacks observe the new value. RecordCore is the same path
	// status / regen ticks use.
	switch action.Target.Kind {
	case ActorKindMob:
		if err := m.mobs.UpdateLive(ctx, action.Target.ID,
			defCore.HPCurrent, defCore.Subdual, defCore.Conditions, defCore.Position); err != nil {
			slog.Warn("combat: mob hp write-back failed",
				"room", roomID, "mob", action.Target.ID, "error", err)
		}
	case ActorKindCharacter:
		if err := m.chars.RecordCore(ctx, action.Target.ID,
			defCore.HPCurrent, defCore.Subdual, defCore.Conditions, defCore.Position); err != nil {
			slog.Warn("combat: char hp write-back failed",
				"room", roomID, "char", action.Target.ID, "error", err)
		}
	}

	// Bump the per-attacker damage tally. Used by handleMobDeath to
	// allocate XP. Held under m.mu so a concurrent fight-end can't
	// see a half-mutated map.
	m.mu.Lock()
	if f, ok := m.fights[roomID]; ok {
		if f.DamageTally == nil {
			f.DamageTally = make(map[ActorRef]int32)
		}
		f.DamageTally[actor] += dealt
		if dealt > 0 {
			// #20: per-defender threat row indexed by attacker.
			// Damage adds 1:1; healing/taunt/feign-death extend this
			// in later slices.
			if f.Threat == nil {
				f.Threat = make(map[ActorRef]map[ActorRef]int32)
			}
			row := f.Threat[action.Target]
			if row == nil {
				row = make(map[ActorRef]int32)
				f.Threat[action.Target] = row
			}
			row[actor] += dealt
		}
	}
	m.mu.Unlock()

	if m.bus != nil {
		m.bus.Publish(ctx, CombatHit{
			RoomID:   roomID,
			Attacker: actor,
			Defender: action.Target,
			Damage:   dealt,
			Weapon:   action.WeaponID,
			IsCrit:   roll.IsCrit,
		})
	}

	// Slice 1 of #19: only mob death triggers the corpse + XP path.
	// Player death (#19 slice 2) sticks with the existing HP write-
	// back; the character is unconscious / disabled but still in the
	// fight order until respawn lands.
	if defCore.HPCurrent <= 0 && action.Target.Kind == ActorKindMob {
		m.handleMobDeath(ctx, actor, action.Target)
	}
}

// rollInitiative resolves one entry's d20 + modifiers. Locks the
// rng so concurrent Starts (multiple rooms opening fights at the
// same instant) don't race the source.
func (m *Manager) rollInitiative(ref ActorRef, core creature.Core) ActorEntry {
	m.rngMu.Lock()
	defer m.rngMu.Unlock()
	roll := m.rng.Intn(20) + 1 // 1..20
	dexMod := int(core.Abilities.DexMod())
	init := roll + dexMod + int(core.InitMod)
	return ActorEntry{
		Ref:        ref,
		Initiative: init,
		Tiebreak:   int32(roll),
	}
}

// resolveCore fetches the participant's creature.Core via the
// matching repo. Returns a repo lookup error wrapped so Start can
// surface it cleanly. Unknown ActorKind returns a fixed sentinel.
var errUnknownActorKind = errors.New("unknown actor kind")

func (m *Manager) resolveCore(ctx context.Context, ref ActorRef) (creature.Core, error) {
	switch ref.Kind {
	case ActorKindCharacter:
		ch, err := m.chars.GetByID(ctx, ref.ID)
		if err != nil {
			return creature.Core{}, err
		}
		return ch.Core, nil
	case ActorKindMob:
		mob, err := m.mobs.GetByID(ctx, ref.ID)
		if err != nil {
			return creature.Core{}, err
		}
		return mob.Core, nil
	}
	return creature.Core{}, errUnknownActorKind
}

// fightHasLiveParticipants reports whether at least one mob
// participant is still in roomID. Character presence is not checked
// here in slice 1 — the session registry is not wired into the
// Manager yet (slice 2 follow-up). A fight with characters only
// continues until explicitly ended; a fight with at least one mob
// auto-ends when every mob is gone.
//
// Called WITHOUT the Manager lock so the mobs.ListInRoom SQL call
// can't stall every other Manager operation. Caller must pass a
// snapshot of the order rather than the live Fight pointer.
//
// This is conservative on purpose: auto-ending too eagerly would
// drop fights mid-round when verb plumbing isn't ready to
// re-engage; auto-ending too rarely produces an idle Fight that
// burns tick CPU but causes no in-world effect. The latter is the
// safer failure mode for V1.
func (m *Manager) fightHasLiveParticipants(ctx context.Context, roomID int64, order []ActorEntry) bool {
	hasMobRef := false
	for _, e := range order {
		if e.Ref.Kind == ActorKindMob {
			hasMobRef = true
			break
		}
	}
	if !hasMobRef {
		// Character-only fights stay live until explicit End. PvP
		// presence checks land with §11 #21.
		return true
	}
	list, err := m.mobs.ListInRoom(ctx, roomID)
	if err != nil {
		slog.Warn("combat: mobs.ListInRoom failed",
			"room", roomID, "error", err)
		return true // fail-safe — keep the fight running
	}
	present := make(map[int64]struct{}, len(list))
	for _, mob := range list {
		present[mob.ID] = struct{}{}
	}
	for _, e := range order {
		if e.Ref.Kind == ActorKindMob {
			if _, ok := present[e.Ref.ID]; ok {
				return true
			}
		}
	}
	return false
}

// handleMobDeath runs the slice-1 mob death pipeline:
//
//  1. Snapshot the per-attacker damage tally and mark the mob dead
//     so the next tickRoom call prunes it from Order.
//  2. Spawn a corpse container item in the mob's room (best-effort —
//     a repo failure logs and continues so the kill still resolves).
//  3. Despawn the mob: clear room then Delete. Trail rows cascade.
//  4. Award XP weighted by damage to character attackers; publish
//     CombatXPAwarded per share. Mob attackers (NPC vs NPC) earn
//     no XP — characters only.
//  5. Publish CombatDeath last so subscribers see the corpse id.
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

	// Phase D #22 slice 4: expand each character contributor's tally
	// across their in-room group teammates so kill XP shares with
	// the party. Mob and unknown-kind contributors pass through
	// unchanged. A nil resolver short-circuits to the solo path.
	m.mu.Lock()
	resolver := m.groupShare
	m.mu.Unlock()
	tallySnap = expandTallyByGroup(tallySnap, roomID, resolver)
	awards := allocateXP(tallySnap, xpValueForChallenge(tmpl.ChallengeCode), killer)
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
		newTotal := ch.XP + amount
		if err := m.chars.RecordXP(ctx, ref.ID, newTotal); err != nil {
			slog.Warn("combat: xp write-back failed",
				"char", ref.ID, "error", err)
			continue
		}
		if m.bus != nil {
			m.bus.Publish(ctx, CombatXPAwarded{
				RoomID:  roomID,
				Awardee: ref,
				Amount:  amount,
				Killed:  victim,
			})
		}
	}

	if m.bus != nil {
		m.bus.Publish(ctx, CombatDeath{
			RoomID:   roomID,
			Victim:   victim,
			Killer:   killer,
			CorpseID: corpseID,
		})
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

// corpseDecayDuration is the V1 lifetime of an empty corpse container
// before the Decayer sweeps it. Five minutes mirrors classic Diku-
// family servers and gives a player time to loot once looting verbs
// land. Held as a constant so the decay timing is grep-stable; future
// per-template / per-zone overrides can layer on without touching
// callers.
const corpseDecayDuration = 5 * time.Minute

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

// resolveParry applies the parrying stance for the actor. The stance
// is round-keyed and consumed by the next incoming attack against
// the actor (see the parry gate in resolveAction). A no-op when the
// fight has been ended between Tick's snapshot and this call.
func (m *Manager) resolveParry(ctx context.Context, roomID int64, round int, actor ActorRef) {
	m.mu.Lock()
	if f, ok := m.fights[roomID]; ok {
		if f.ParryingUntil == nil {
			f.ParryingUntil = make(map[ActorRef]int)
		}
		f.ParryingUntil[actor] = round
	}
	m.mu.Unlock()
	if m.bus != nil {
		m.bus.Publish(ctx, CombatStance{
			RoomID: roomID,
			Actor:  actor,
			Kind:   "parry",
		})
	}
}

// resolveFlee delegates the actual room transition + roll to the
// injected FleeMover and records the outcome on the fight. On
// success the actor is added to f.Fled so pruneDead clears them on
// the next pulse; the fight then auto-ends naturally if no
// participants remain. CombatFlee always publishes — failure is part
// of the contract so the verb can give the player feedback.
func (m *Manager) resolveFlee(ctx context.Context, roomID int64, actor ActorRef) {
	mover := m.snapFleeMover()
	var res FleeResult
	if mover == nil {
		res = FleeResult{Reason: "no_mover"}
	} else {
		res = mover.AttemptFlee(ctx, roomID, actor)
	}
	if res.Success {
		m.mu.Lock()
		if f, ok := m.fights[roomID]; ok {
			if f.Fled == nil {
				f.Fled = make(map[ActorRef]struct{})
			}
			f.Fled[actor] = struct{}{}
		}
		m.mu.Unlock()
	}
	if m.bus != nil {
		m.bus.Publish(ctx, CombatFlee{
			RoomID:    roomID,
			Actor:     actor,
			Success:   res.Success,
			Direction: res.Direction,
			ToRoomID:  res.ToRoomID,
			Reason:    res.Reason,
		})
	}
}

// snapFleeMover snapshots the mover under the lock so callers don't
// race a concurrent SetFleeMover.
func (m *Manager) snapFleeMover() FleeMover {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.fleeMover
}

// Stop ends every active fight (publishing CombatEnded with
// ReasonManagerStop) and clears the map. Called from the server
// shutdown path after the listener stops accepting but before the
// persist.Manager flush so no fight survives a graceful restart.
func (m *Manager) Stop(ctx context.Context) {
	m.mu.Lock()
	rooms := make([]int64, 0, len(m.fights))
	for id := range m.fights {
		rooms = append(rooms, id)
	}
	m.fights = make(map[int64]*Fight)
	m.mu.Unlock()

	if m.bus == nil {
		return
	}
	for _, id := range rooms {
		m.bus.Publish(ctx, CombatEnded{RoomID: id, Reason: ReasonManagerStop})
	}
}
