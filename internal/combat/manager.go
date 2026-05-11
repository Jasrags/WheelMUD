package combat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/Jasrags/WheelMUD/internal/affects"
	"github.com/Jasrags/WheelMUD/internal/chargen"
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

// ErrInsufficientStamina is returned by EnqueueAction when the actor's
// StaminaCurrent is below the action's DefaultActionStamina cost.
// Phase L slice 63 — verbs surface this as "you're too winded" rather
// than the generic "you can't engage right now" so the player knows
// to wait for the Regen tick.
var ErrInsufficientStamina = errors.New("combat: insufficient stamina")

// Manager owns the per-process map of active Fights. It is the
// single source of truth for combat state — every verb (slice 2+)
// reads and writes through this interface. Concurrent-safe.
type Manager struct {
	mu     sync.RWMutex
	fights map[int64]*Fight // roomID → fight

	bus        *eventbus.Bus
	chars      repo.CharacterRepo
	mobs       repo.MobInstanceRepo
	templates  repo.MobTemplateRepo // optional; nil disables XP rewards
	items      repo.ItemRepo        // optional; nil disables weapon-stat lookup + corpse spawn
	decayer    *Decayer             // optional; nil leaves corpses lingering until admin purge
	fleeMover  FleeMover            // optional; nil makes ActionFlee fail with reason="no_mover"
	groupShare GroupResolver        // optional; nil = solo split (each kill credits the dealer only)
	cat        *chargen.Catalog     // optional; nil disables feat-driven cadence modifiers (Phase L slice 65)

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

// SetCatalog wires the chargen catalog so the combat hot path can
// resolve cadence-modifier feats off a character's []int32 Feats list.
// Phase L slice 65 — optional; nil leaves the actor at the
// gear-driven baseline. cmd/server/main.go calls this after the
// catalog loads at boot.
func (m *Manager) SetCatalog(cat *chargen.Catalog) {
	m.mu.Lock()
	m.cat = cat
	m.mu.Unlock()
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
	startedAt := m.now()
	// Per-actor cadence: every entry is "ready" on the first pulse.
	// Initiative order resolves within-pulse fan-out ties; NextActAt
	// gates future pulses once each actor has acted at least once.
	for i := range entries {
		entries[i].NextActAt = startedAt
	}
	f := &Fight{
		RoomID:    roomID,
		Round:     0,
		Order:     entries,
		StartedAt: startedAt,
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

// Tick advances every active fight by one pulse. Subscribed to
// tick.Buckets.Combat at boot. For each fight:
//
//  1. If fewer than 2 participants are still in the room (anyone
//     logged out, mob despawned, character moved), the fight
//     auto-ends with ReasonNoParticipants and no RoundStarted fires.
//  2. Otherwise every actor whose NextActAt <= now resolves one
//     queued action (or a no-op when nothing is queued) in
//     initiative order. Round increments per resolution and a
//     RoundStarted event publishes for each acting actor.
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
	// Prune actors that died on the previous pulse before walking.
	// Done up here (not inline in resolveAction) so the per-actor
	// walk observes a stable Order across this pulse.
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

	// Snapshot the ready set under the lock. Snapshotting Refs (not
	// indices) decouples the per-actor resolution loop from any
	// mid-loop Order mutation (death prune lands next pulse, but
	// EnqueueAction can still race on f.Actions). Snapshot per-actor
	// rounds so each RoundStarted event reflects the pre-resolution
	// counter for that swing.
	now := m.now()
	type readyEntry struct {
		ref   ActorRef
		round int
		// iter is the per-swing iterative attack-roll bonuses for this
		// actor (Phase L #66). Pinned at Start; copied here under
		// m.mu so the drain loop can read it without re-locking.
		// Always non-empty (at minimum []int16{0}).
		iter []int16
	}
	ready := make([]readyEntry, 0, len(f.Order))
	for i := range f.Order {
		if f.Order[i].NextActAt.After(now) {
			continue
		}
		f.Round++
		bonuses := f.Order[i].IterativeBonuses
		if len(bonuses) == 0 {
			// Pre-#66 fights (e.g. tests that construct Fight directly)
			// observed a nil slice — default to one swing at no penalty
			// so the loop below still fires exactly once.
			bonuses = []int16{0}
		}
		ready = append(ready, readyEntry{ref: f.Order[i].Ref, round: f.Round, iter: bonuses})
	}
	if len(ready) == 0 {
		m.mu.Unlock()
		// Affects still tick — out-of-combat buffs would otherwise
		// freeze whenever a fight is open but nobody is acting on
		// this pulse.
		m.tickParticipantAffects(ctx, roomID)
		return
	}
	pendings := make(map[ActorRef]Action, len(ready))
	hasActions := make(map[ActorRef]bool, len(ready))
	for _, r := range ready {
		a, ok := f.popAction(r.ref)
		pendings[r.ref] = a
		hasActions[r.ref] = ok
	}
	m.mu.Unlock()

	for _, r := range ready {
		if m.bus != nil {
			m.bus.Publish(ctx, RoundStarted{
				RoomID: roomID,
				Round:  r.round,
				Active: r.ref,
			})
		}
		action := pendings[r.ref]

		// Phase L #66: iterative attacks via cadence drain. A
		// high-BAB attacker swings 1 + IterativeCount times back-to-
		// back on a single pulse; each successive swing pays an
		// attack-roll penalty (0/-5/-10/-15) and a fresh stamina
		// cost. The chain truncates when stamina runs dry or the
		// target dies/flees mid-chain. Costs accumulate so
		// NextActAt is set after the *last* swing.
		//
		// Non-Attack kinds (Parry/Dodge/Flee/Throw/Sidestep) stay
		// single-resolution — iteratives are an attack mechanic
		// only.
		swings := 1
		if hasActions[r.ref] && action.Kind == ActionAttack {
			swings = len(r.iter)
		}

		totalCost := time.Duration(0)
		// Phase L #62: cadence is gear-driven. actorActionCost reads
		// the actor's wielded weapon weight + worn armor weight class
		// at "now", so a `wield` mid-fight changes the cadence of the
		// next swing. Lookup happens outside the manager lock to keep
		// the SQL hit off the critical section. Re-evaluated each
		// swing so a mid-chain gear swap (shouldn't happen — actor
		// is busy — but cheap insurance) is observable.
		if hasActions[r.ref] {
			for i := 0; i < swings; i++ {
				if i > 0 {
					// Pre-swing truncation gate: stamina dry or target
					// no longer in the fight (died/fled on the prior
					// swing).
					if !m.hasStaminaFor(ctx, r.ref, action) {
						break
					}
					if m.iterativeTargetGone(roomID, action.Target) {
						break
					}
				}
				bonus := 0
				if action.Kind == ActionAttack && i < len(r.iter) {
					bonus = int(r.iter[i])
				}
				m.resolveAction(ctx, roomID, r.round, r.ref, action, bonus)
				totalCost += m.actorActionCost(ctx, r.ref, action)
			}
		} else {
			// No queued action — still advance the clock so the actor
			// doesn't busy-spin on this slot. Single base cost.
			totalCost = m.actorActionCost(ctx, r.ref, action)
		}

		// Stamp the actor's next-ready time under the lock so a
		// concurrent EnqueueAction can't observe a stale schedule.
		// resolveAction may have mutated Order (death prune is
		// deferred to next pulse, but Fled is queued during flee
		// resolution) so re-resolve the entry by Ref.
		m.mu.Lock()
		if f2, ok := m.fights[roomID]; ok {
			for i := range f2.Order {
				if f2.Order[i].Ref == r.ref {
					f2.Order[i].LastActedAt = now
					f2.Order[i].NextActAt = now.Add(totalCost)
					break
				}
			}
		}
		m.mu.Unlock()
	}

	// Phase E #26: tick every participant's affects once per pulse
	// so in-fight buffs/debuffs count down. Out-of-combat ticking
	// lives in the Affects bucket subscriber. Pruning runs at the
	// top of the next pulse, so a death this pulse still ticks here
	// (harmless — the row is gone next pulse). Mob affects are
	// in-memory only and are dropped on despawn anyway.
	m.tickParticipantAffects(ctx, roomID)
}

// tickParticipantAffects decrements affect durations for every
// participant of the active fight in roomID and persists changes for
// character actors. Mob affects mutate the in-memory snapshot only —
// MobInstanceRepo doesn't have an affects column and dead mobs drop
// the slice anyway. Errors per-participant log-and-continue (mirrors
// the §19 mob-death rule).
func (m *Manager) tickParticipantAffects(ctx context.Context, roomID int64) {
	m.mu.RLock()
	f, ok := m.fights[roomID]
	if !ok {
		m.mu.RUnlock()
		return
	}
	order := append([]ActorEntry(nil), f.Order...)
	m.mu.RUnlock()

	for _, entry := range order {
		ref := entry.Ref
		if ref.Kind != ActorKindCharacter {
			// Mob affects: skip — no persistence, the in-memory Core
			// goes away on despawn. When a content source starts
			// applying mob affects in V2 this is the natural slot.
			continue
		}
		core, err := m.resolveCore(ctx, ref)
		if err != nil {
			slog.Warn("combat: affects tick resolve failed",
				"room", roomID, "actor", ref, "error", err)
			continue
		}
		if len(core.Affects) == 0 {
			continue
		}
		next, expired := affects.Tick(core.Affects)
		// Always write back: Tick decrements every entry's
		// DurationTicks, so even when no affect expires the row needs
		// the new durations or next round reloads the original values
		// and the affect never counts down. Cheap single-column
		// UPDATE; the optimisation slot if write pressure shows up at
		// scale is an in-memory affects cache, not a write skip.
		if err := m.chars.RecordAffects(ctx, ref.ID, next); err != nil {
			slog.Warn("combat: affects write-back failed",
				"room", roomID, "char", ref.ID, "error", err)
			continue
		}
		if len(expired) > 0 && m.bus != nil {
			entries := make([]affects.ExpiredEntry, len(expired))
			for i, a := range expired {
				entries[i] = affects.ExpiredEntry{Name: a.Name, Message: a.ExpireMessage}
			}
			m.bus.Publish(ctx, affects.Expired{
				CharacterID: ref.ID,
				RoomID:      roomID,
				Entries:     entries,
			})
		}
	}
}

// resolveAction is the active actor's slice-1 turn handler. Today it
// handles ActionAttack only; future kinds (flee / weave / kick) slot
// in as additional cases. Errors loading participants or items are
// logged and swallowed — combat continues, the swing just doesn't
// land. This mirrors the spawn / item-transfer "fail-safe, log
// loudly" stance.
//
// extraAttackBonus is the per-swing iterative penalty (Phase L #66)
// added to the attack roll on the ActionAttack branch — 0 on the
// first swing, -5 / -10 / -15 on successive swings inside the same
// pulse. Defensive / movement kinds (Parry, Dodge, Flee, Throw,
// Sidestep) ignore the parameter.
func (m *Manager) resolveAction(ctx context.Context, roomID int64, round int, actor ActorRef, action Action, extraAttackBonus int) {
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
	// Phase L slice 63: stamina drain. Runs ahead of the action-kind
	// dispatch so a parry / flee pays its cost on resolution even
	// when those branches return early. Mob actors and zero-cost
	// kinds short-circuit; repo failures log-and-continue (the swing
	// still resolves — we'd rather pay the action than refuse on a
	// transient SQL hiccup mid-fight).
	m.drainStamina(ctx, actor, action)
	switch action.Kind {
	case ActionParry:
		m.resolveParry(ctx, roomID, round, actor)
		return
	case ActionFlee:
		m.resolveFlee(ctx, roomID, actor)
		return
	case ActionDodge:
		m.resolveDodge(ctx, roomID, round, actor)
		return
	case ActionSidestep:
		m.resolveSidestep(ctx, roomID, round, actor, action.Target)
		return
	case ActionThrow:
		m.resolveThrow(ctx, roomID, round, actor, action)
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
	defenderDodging := false
	if f, ok := m.fights[roomID]; ok {
		if v, has := f.FlatFootedUntil[action.Target]; has && v >= round {
			defenderFlatFooted = true
		}
		if v, has := f.ParryingUntil[action.Target]; has && v >= round {
			defenderParrying = true
		}
		if v, has := f.DodgeUntil[action.Target]; has && v >= round {
			defenderDodging = true
			// Dodge grants flat-foot immunity for this one swing —
			// the attack rolls against full Dex Defense even when
			// FlatFootedUntil also applies (e.g. a sidestepper on
			// the same defender).
			defenderFlatFooted = false
		}
	}
	m.mu.Unlock()

	// Phase E #26: combat math reads through-affect values so timed
	// buffs/debuffs (Defense+, Str+, Dex-, Saves, Speed, BAB) influence
	// the roll. Effective returns a copy; original Cores are untouched
	// so the HP write-back below uses unfolded values. Effective does
	// not modify DR/Resists slices, so applyDamage's reads through the
	// original defCore observe the same DR/Resists either way.
	atkEff := affects.Effective(atkCore)
	defEff := affects.Effective(defCore)

	// Dodge grants the defender +4 to effective Defense for this one
	// swing. Applied via a transient Core copy so RollAttack's pure
	// signature stays intact and the original defEff remains the
	// canonical view for damage / write-back below.
	defForRoll := defEff
	if defenderDodging {
		defForRoll.Defense += 4
	}
	m.rngMu.Lock()
	roll := RollAttack(m.rng, atkEff, defForRoll, stats, defenderFlatFooted, VariantAttackBonus(action.Variant)+extraAttackBonus)
	parried := false
	parryTotal := 0
	if roll.Hit && defenderParrying {
		parryTotal = RollParry(m.rng, defEff)
		if parryTotal > roll.Total {
			parried = true
		}
	}
	var dealt int32
	if roll.Hit && !parried {
		raw := RollDamage(m.rng, atkEff, stats, roll.IsCrit, action.Variant)
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

	// Consume the dodge stance now — one swing, one dodge, regardless
	// of hit/miss branch. Single delete-point keeps the consumption
	// invariant trivially auditable (vs. the prior two-site pattern,
	// which only worked under single-goroutine-per-room semantics).
	// dodgeTriggered captures the "swing would have landed without the
	// +4 bonus" pre-condition so the miss-branch publish can pick the
	// CombatDodgeAvoided event over a plain CombatMiss.
	dodgeTriggered := defenderDodging && !roll.Hit && roll.Total >= int(defEff.Defense)
	if defenderDodging {
		m.mu.Lock()
		if f, ok := m.fights[roomID]; ok {
			delete(f.DodgeUntil, action.Target)
		}
		m.mu.Unlock()
	}

	if !roll.Hit {
		if m.bus != nil {
			if dodgeTriggered {
				m.bus.Publish(ctx, CombatDodgeAvoided{
					RoomID:   roomID,
					Defender: action.Target,
					Attacker: actor,
					Roll:     roll.Total,
					Defense:  defForRoll.Defense,
				})
			} else {
				// Fumble side-effect: nat-1 from a character attacker
				// drops their primary-wield weapon to the floor. Mob
				// fumble is a flavor-only flag (no persisted equipment
				// in V1). Resolved before the publish so the event
				// carries the dropped weapon id.
				droppedID := int64(0)
				if roll.Fumble {
					droppedID = m.handleFumble(ctx, roomID, actor)
				}
				m.bus.Publish(ctx, CombatMiss{
					RoomID:          roomID,
					Attacker:        actor,
					Defender:        action.Target,
					RollTotal:       roll.Total,
					Defense:         defEff.Defense,
					Variant:         action.Variant,
					Fumble:          roll.Fumble,
					WeaponDroppedID: droppedID,
				})
			}
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
			Threat:   roll.Threat,
			IsCrit:   roll.IsCrit,
			Variant:  action.Variant,
		})
	}

	// Death dispatch. Mob death follows the corpse + XP pipeline
	// (#19 slice 1 + looting); character death follows the
	// respawn + XP-debt pipeline (#19 slice 2). Both run outside
	// any caller-held lock; m.mu is taken inside each handler.
	if defCore.HPCurrent <= 0 {
		switch action.Target.Kind {
		case ActorKindMob:
			m.handleMobDeath(ctx, actor, action.Target)
		case ActorKindCharacter:
			m.handleCharacterDeath(ctx, actor, action.Target)
		}
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
	bonuses := IterativeBonusesFor(core.BAB)
	return ActorEntry{
		Ref:              ref,
		Initiative:       init,
		Tiebreak:         int32(roll),
		PendingSwings:    len(bonuses),
		IterativeBonuses: bonuses,
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

// resolveParry applies the parrying stance for the actor. The stance
// is round-keyed and consumed by the next incoming attack against
// the actor (see the parry gate in resolveAction). Under per-actor
// cadence Round ++'s on every actor-act, so the stamp value
// `round + 1` means "good for the very next incoming swing" — once
// another actor-act elapses the stance expires. A no-op when the
// fight has been ended between Tick's snapshot and this call.
func (m *Manager) resolveParry(ctx context.Context, roomID int64, round int, actor ActorRef) {
	m.mu.Lock()
	if f, ok := m.fights[roomID]; ok {
		if f.ParryingUntil == nil {
			f.ParryingUntil = make(map[ActorRef]int)
		}
		f.ParryingUntil[actor] = round + 1
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

// resolveDodge applies the dodge stance for actor. Mirror of
// resolveParry but no opposed-roll dependency — the attack resolver
// adds +4 Defense and grants flat-foot immunity for the next swing
// against actor, consuming the stance on first trigger. Published as
// CombatStance{Kind:"dodge"} so cmd-layer broadcast subscribers
// don't need to know about ParryingUntil / DodgeUntil layout.
func (m *Manager) resolveDodge(ctx context.Context, roomID int64, round int, actor ActorRef) {
	m.mu.Lock()
	if f, ok := m.fights[roomID]; ok {
		if f.DodgeUntil == nil {
			f.DodgeUntil = make(map[ActorRef]int)
		}
		f.DodgeUntil[actor] = round + 1
	}
	m.mu.Unlock()
	if m.bus != nil {
		m.bus.Publish(ctx, CombatStance{
			RoomID: roomID,
			Actor:  actor,
			Kind:   "dodge",
		})
	}
}

// resolveSidestep stamps FlatFootedUntil[target] for one round.
// target is the attacker the sidestepping actor wants to expose;
// the existing FlatFootedUntil read in resolveAction will drop the
// attacker's Dex bonus to Defense on the next swing landed against
// them. Silently no-ops when target isn't a participant — the verb
// layer pre-validates this but defense in depth.
func (m *Manager) resolveSidestep(ctx context.Context, roomID int64, round int, actor ActorRef, target ActorRef) {
	m.mu.Lock()
	if f, ok := m.fights[roomID]; ok {
		inOrder := false
		for _, e := range f.Order {
			if e.Ref == target {
				inOrder = true
				break
			}
		}
		if inOrder {
			if f.FlatFootedUntil == nil {
				f.FlatFootedUntil = make(map[ActorRef]int)
			}
			f.FlatFootedUntil[target] = round + 1
		}
	}
	m.mu.Unlock()
	if m.bus != nil {
		m.bus.Publish(ctx, CombatStance{
			RoomID: roomID,
			Actor:  actor,
			Kind:   "sidestep",
			Target: target,
		})
	}
}

// resolveThrow resolves a one-shot ranged attack. Rolls a Normal
// variant attack with the thrown weapon's stats, publishes a
// CombatThrow lead-in alongside the standard CombatHit/CombatMiss,
// then unequips the thrown item from SlotPrimaryWield and drops it —
// to the room on miss, into the corpse on kill. Errors persisting
// equipment or moving the item log-and-continue; the swing is
// authoritative either way.
func (m *Manager) resolveThrow(ctx context.Context, roomID int64, round int, actor ActorRef, action Action) {
	if action.Target.Kind == ActorKindUnknown || action.WeaponID == 0 {
		return
	}
	atkCore, err := m.resolveCore(ctx, actor)
	if err != nil {
		slog.Warn("combat: resolve attacker failed (throw)",
			"room", roomID, "actor", actor, "error", err)
		return
	}
	defCore, err := m.resolveCore(ctx, action.Target)
	if err != nil {
		if m.bus != nil {
			m.bus.Publish(ctx, CombatMiss{
				RoomID: roomID, Attacker: actor, Defender: action.Target,
			})
		}
		return
	}
	var item repo.Item
	stats := unarmedDamage
	itemName := "the weapon"
	if m.items != nil {
		it, err := m.items.GetByID(ctx, action.WeaponID)
		if err == nil {
			item = it
			stats = weaponStatsFor(&it)
			if it.Name != "" {
				itemName = it.Name
			}
		}
	}

	m.mu.Lock()
	defenderFlatFooted := false
	if f, ok := m.fights[roomID]; ok {
		if v, has := f.FlatFootedUntil[action.Target]; has && v >= round {
			defenderFlatFooted = true
		}
	}
	m.mu.Unlock()

	atkEff := affects.Effective(atkCore)
	defEff := affects.Effective(defCore)

	m.rngMu.Lock()
	roll := RollAttack(m.rng, atkEff, defEff, stats, defenderFlatFooted, 0)
	var dealt int32
	if roll.Hit {
		raw := RollDamage(m.rng, atkEff, stats, roll.IsCrit, VariantNormal)
		dealt = applyDamage(&defCore, raw, weaponPrimaryDamageType(stats))
	}
	m.rngMu.Unlock()

	// Consume the wielded weapon first: clear the primary wield slot
	// and persist the updated equipment_json. Character actors only —
	// mob actors don't have an Equipment surface today (deferred).
	// Ordering matters: subscribers to the events published below must
	// observe the actor's slot already empty, so the equipment-clear
	// runs before any publish. A repo failure here log-and-continues
	// (the swing has already resolved); leaks an in-hand item until
	// the operator fixes it.
	m.clearThrownWeapon(ctx, roomID, actor, action.WeaponID)

	if !roll.Hit {
		if m.bus != nil {
			m.bus.Publish(ctx, CombatMiss{
				RoomID:    roomID,
				Attacker:  actor,
				Defender:  action.Target,
				RollTotal: roll.Total,
				Defense:   defEff.Defense,
				Variant:   VariantNormal,
			})
			m.bus.Publish(ctx, CombatThrow{
				RoomID:   roomID,
				Attacker: actor,
				Defender: action.Target,
				ItemID:   action.WeaponID,
				ItemName: itemName,
				RollHit:  false,
			})
		}
		// Item lands on the floor at the target's feet (== actor's room).
		m.dropThrownItem(ctx, action.WeaponID, roomID, 0, item)
		return
	}

	// Hit path: persist damage, bump tally + threat, publish CombatHit.
	switch action.Target.Kind {
	case ActorKindMob:
		if err := m.mobs.UpdateLive(ctx, action.Target.ID,
			defCore.HPCurrent, defCore.Subdual, defCore.Conditions, defCore.Position); err != nil {
			slog.Warn("combat: mob hp write-back failed (throw)",
				"room", roomID, "mob", action.Target.ID, "error", err)
		}
	case ActorKindCharacter:
		if err := m.chars.RecordCore(ctx, action.Target.ID,
			defCore.HPCurrent, defCore.Subdual, defCore.Conditions, defCore.Position); err != nil {
			slog.Warn("combat: char hp write-back failed (throw)",
				"room", roomID, "char", action.Target.ID, "error", err)
		}
	}

	m.mu.Lock()
	if f, ok := m.fights[roomID]; ok {
		if f.DamageTally == nil {
			f.DamageTally = make(map[ActorRef]int32)
		}
		f.DamageTally[actor] += dealt
		if dealt > 0 {
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
			Threat:   roll.Threat,
			IsCrit:   roll.IsCrit,
			Variant:  VariantNormal,
		})
		m.bus.Publish(ctx, CombatThrow{
			RoomID:   roomID,
			Attacker: actor,
			Defender: action.Target,
			ItemID:   action.WeaponID,
			ItemName: itemName,
			RollHit:  true,
		})
	}

	if defCore.HPCurrent <= 0 {
		switch action.Target.Kind {
		case ActorKindMob:
			m.handleMobDeath(ctx, actor, action.Target)
			// Drop the thrown weapon into the corpse if one was
			// spawned by handleMobDeath. The corpse item id is on
			// the published CombatDeath, but we can't subscribe to
			// our own bus inline; instead, look up the freshest
			// corpse in the room by deriving from the mobs.GetCorpse
			// pattern. V1 simplification: re-list room items and
			// parent into the most recent corpse owned by this room.
			m.dropThrownItem(ctx, action.WeaponID, roomID, m.latestCorpseInRoom(ctx, roomID), item)
		case ActorKindCharacter:
			m.handleCharacterDeath(ctx, actor, action.Target)
			m.dropThrownItem(ctx, action.WeaponID, roomID, 0, item)
		}
		return
	}
	// Survived the hit — item drops at the target's feet.
	m.dropThrownItem(ctx, action.WeaponID, roomID, 0, item)
}

// handleFumble drops the attacker's primary-wield weapon to the room
// floor on a natural-1 attack roll. Character actors only — mob
// equipment has no persisted layer in V1 (Phase L #62 followup).
// Returns the dropped item id, or 0 when nothing was dropped
// (unarmed, mob attacker, or repo error). Best-effort: equipment-
// persist or SetRoom failures slog.Warn and surface as a 0 return
// so the miss still publishes and combat doesn't desync.
func (m *Manager) handleFumble(ctx context.Context, roomID int64, actor ActorRef) int64 {
	if actor.Kind != ActorKindCharacter || m.chars == nil || m.items == nil {
		return 0
	}
	ch, err := m.chars.GetByID(ctx, actor.ID)
	if err != nil {
		slog.Warn("combat: fumble equipment lookup failed",
			"room", roomID, "char", actor.ID, "error", err)
		return 0
	}
	weaponID := ch.Equipment.Get(creature.SlotPrimaryWield)
	if weaponID == 0 {
		return 0
	}
	ch.Equipment.Set(creature.SlotPrimaryWield, 0)
	if err := m.chars.RecordEquipment(ctx, actor.ID, ch.Equipment); err != nil {
		slog.Warn("combat: fumble equipment persist failed",
			"room", roomID, "char", actor.ID, "error", err)
		return 0
	}
	if err := m.items.SetRoom(ctx, weaponID, roomID); err != nil {
		slog.Warn("combat: fumble item-to-floor failed",
			"room", roomID, "item", weaponID, "error", err)
		return 0
	}
	return weaponID
}

// clearThrownWeapon empties the actor's primary wield slot when it
// still holds weaponID, then persists the updated equipment_json.
// Character actors only — mob equipment is deferred. Best-effort:
// repo lookup or persist errors log-and-continue so the swing still
// resolves. Idempotent: re-running on a slot that already changed is
// a no-op.
func (m *Manager) clearThrownWeapon(ctx context.Context, roomID int64, actor ActorRef, weaponID int64) {
	if actor.Kind != ActorKindCharacter || m.chars == nil || weaponID == 0 {
		return
	}
	ch, err := m.chars.GetByID(ctx, actor.ID)
	if err != nil {
		slog.Warn("combat: throw equipment lookup failed",
			"room", roomID, "char", actor.ID, "error", err)
		return
	}
	if ch.Equipment.Get(creature.SlotPrimaryWield) != weaponID {
		return
	}
	ch.Equipment.Set(creature.SlotPrimaryWield, 0)
	if err := m.chars.RecordEquipment(ctx, actor.ID, ch.Equipment); err != nil {
		slog.Warn("combat: throw equipment persist failed",
			"room", roomID, "char", actor.ID, "error", err)
	}
}

// dropThrownItem moves a thrown weapon into a corpse (when corpseID
// is non-zero) or onto the room floor (when corpseID is zero). Errors
// log-and-continue — the swing has already published, so failing the
// drop just leaks the item in inventory (which the operator can
// inspect via admin tools). Caller should have already cleared the
// equipment slot.
func (m *Manager) dropThrownItem(ctx context.Context, itemID, roomID, corpseID int64, _ repo.Item) {
	if m.items == nil || itemID == 0 {
		return
	}
	if corpseID != 0 {
		if err := m.items.SetParent(ctx, itemID, corpseID); err != nil {
			slog.Warn("combat: throw item-into-corpse failed",
				"item", itemID, "corpse", corpseID, "error", err)
		}
		return
	}
	if err := m.items.SetRoom(ctx, itemID, roomID); err != nil {
		slog.Warn("combat: throw item-to-floor failed",
			"item", itemID, "room", roomID, "error", err)
	}
}

// latestCorpseInRoom returns the id of the most recently created
// corpse in roomID, or 0 if none. Used by resolveThrow to parent
// the thrown weapon into a freshly-spawned corpse. Best-effort —
// returns 0 on any repo error so the throw degrades to a floor
// drop rather than failing the swing.
func (m *Manager) latestCorpseInRoom(ctx context.Context, roomID int64) int64 {
	if m.items == nil {
		return 0
	}
	list, err := m.items.ListInRoom(ctx, roomID)
	if err != nil {
		return 0
	}
	var latest int64
	for _, it := range list {
		if it.Type != repo.ItemTypeContainer {
			continue
		}
		// Corpses use the "corpse-" external_id prefix (see
		// mob_death.go::corpseExternalID). Match on that so we don't
		// accidentally parent the thrown weapon into a player-placed
		// chest that happens to share the room.
		if len(it.ExternalID) >= 7 && it.ExternalID[:7] == "corpse-" && it.ID > latest {
			latest = it.ID
		}
	}
	return latest
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
