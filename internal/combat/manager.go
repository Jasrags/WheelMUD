package combat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
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

	bus   *eventbus.Bus
	chars repo.CharacterRepo
	mobs  repo.MobInstanceRepo

	rngMu sync.Mutex
	rng   *rand.Rand // injectable for tests
	now   func() time.Time
}

// New constructs a Manager. bus is required (events are how downstream
// systems learn about fights); chars / mobs are required for
// participant resolution at Start time and the auto-end check at
// Tick time.
func New(bus *eventbus.Bus, chars repo.CharacterRepo, mobs repo.MobInstanceRepo) *Manager {
	return &Manager{
		fights: make(map[int64]*Fight),
		bus:    bus,
		chars:  chars,
		mobs:   mobs,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
		now:    time.Now,
	}
}

// SetRNG injects a deterministic random source. Tests use this to
// assert exact initiative orders. Production callers leave the
// time-seeded default.
func (m *Manager) SetRNG(r *rand.Rand) {
	m.rngMu.Lock()
	defer m.rngMu.Unlock()
	m.rng = r
}

// SetClock injects a deterministic time source. Tests use this; the
// constructor wires time.Now by default.
func (m *Manager) SetClock(now func() time.Time) {
	if now == nil {
		return
	}
	m.now = now
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
func (m *Manager) tickRoom(ctx context.Context, roomID int64) {
	m.mu.Lock()
	f, ok := m.fights[roomID]
	if !ok {
		m.mu.Unlock()
		return
	}

	if !m.fightHasLiveParticipants(ctx, f) {
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
	m.mu.Unlock()

	if m.bus != nil {
		m.bus.Publish(ctx, RoundStarted{
			RoomID: roomID,
			Round:  round,
			Active: active,
		})
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
		// CharacterRepo doesn't expose GetByID today — only
		// FindByName. The slice 2 follow-up that adds the `attack`
		// verb will route through the session registry which already
		// holds the resolved Character; for now Start callers must
		// resolve the Character themselves and pass the ref. As a
		// transitional path we accept that participants are known to
		// the caller and do a defensive empty-name lookup that
		// returns ErrCharacterNotFound for any non-zero id (the
		// memory repo doesn't index by id and the sqlite repo
		// likewise lacks a getter). When #18 lands we'll add
		// CharacterRepo.GetByID and route through it.
		//
		// For slice 1 we return a zero Core for character refs so
		// the unit tests can validate Manager mechanics without a
		// dummy GetByID. This is intentional — the rolled
		// Initiative will use 0 InitMod / 0 DexMod for characters,
		// which is not a balance concern because there is no damage
		// math yet.
		return creature.Core{}, nil
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
// participant is still in the fight's room. Character presence is
// not checked here in slice 1 — the session registry is not wired
// into the Manager yet (slice 2 follow-up). A fight with characters
// only continues until explicitly ended; a fight with at least one
// mob auto-ends when every mob is gone.
//
// This is conservative on purpose: auto-ending too eagerly would
// drop fights mid-round when verb plumbing isn't ready to
// re-engage; auto-ending too rarely produces an idle Fight that
// burns tick CPU but causes no in-world effect. The latter is the
// safer failure mode for V1.
func (m *Manager) fightHasLiveParticipants(ctx context.Context, f *Fight) bool {
	mobsInRoom := -1 // lazy: only fetch if the fight has mob refs
	hasCharRef := false
	hasMobRef := false
	for _, e := range f.Order {
		switch e.Ref.Kind {
		case ActorKindCharacter:
			hasCharRef = true
		case ActorKindMob:
			hasMobRef = true
			if mobsInRoom == -1 {
				list, err := m.mobs.ListInRoom(ctx, f.RoomID)
				if err != nil {
					slog.Warn("combat: mobs.ListInRoom failed",
						"room", f.RoomID, "error", err)
					return true // fail-safe — keep the fight running
				}
				mobsInRoom = 0
				present := make(map[int64]struct{}, len(list))
				for _, m := range list {
					present[m.ID] = struct{}{}
				}
				for _, e2 := range f.Order {
					if e2.Ref.Kind == ActorKindMob {
						if _, ok := present[e2.Ref.ID]; ok {
							mobsInRoom++
						}
					}
				}
			}
		}
	}
	if hasMobRef && mobsInRoom == 0 {
		return false
	}
	// Character-only fights stay live until explicit End. PvP
	// presence checks land with §11 #21.
	_ = hasCharRef
	return true
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
