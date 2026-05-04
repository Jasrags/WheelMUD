// Package mob holds the gameplay-driven mob runners that sit between
// the persistence layer (repo) and the game loop (tick). Today this
// is just the wander tick; combat AI, aggro, and corpse decay will
// land alongside it as §11 / §12 progress.
package mob

import (
	"context"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
)

// Defaults for the wander handler. Surfaced as constants so tests
// (and a future config) can refer to them by name.
//
// The per-template wander chance lives on creature.MobTemplate
// (creature.DefaultWanderChance is the default applied to YAML
// templates without an explicit value); the handler's chance is a
// global *multiplier* against that, so an admin can dial wander
// activity up or down without rewriting every template.
const (
	DefaultWanderMultiplier = 1.0
	DefaultWanderCap        = 200
)

// Option mutates the wander handler at construction.
type Option func(*WanderHandler)

// WithChance sets the global multiplier applied to every template's
// WanderChance. 1.0 (the default) honors the template values as-is.
// 0.0 disables wandering entirely. Values > 1.0 are pinned to 1.0
// at the per-mob roll site so a misconfigured multiplier can't
// over-saturate a template.
func WithChance(p float64) Option { return func(h *WanderHandler) { h.chance = p } }

// WithCap caps how many mobs the handler considers per pulse. Values
// <= 0 disable the handler (no work done per pulse).
func WithCap(n int) Option { return func(h *WanderHandler) { h.cap = n } }

// WithRand injects a deterministic RNG for tests. Production callers
// can omit this; the constructor seeds one from the wall clock.
func WithRand(r *rand.Rand) Option { return func(h *WanderHandler) { h.rng = r } }

// WanderHandler relocates non-Sentinel mobs into adjacent rooms once
// per pulse. The pulse cadence is owned by the caller (typically
// tick.Buckets.Wander); this type only decides who moves and where.
//
// Trail recording happens inside MobInstanceRepo.UpdateRoom — the
// handler is just the trigger.
type WanderHandler struct {
	mobs      repo.MobInstanceRepo
	rooms     repo.RoomRepo
	exits     repo.ExitRepo
	templates repo.MobTemplateRepo
	sessions  *session.Registry

	chance float64
	cap    int

	// rngMu guards rng. *math/rand.Rand is not concurrent-safe, and
	// scheduler.tick() spawns a fresh goroutine per pulse — so two
	// Tick goroutines can overlap when one pulse runs long.
	rngMu sync.Mutex
	rng   *rand.Rand
}

// rollFloat returns rng.Float64() under rngMu.
func (h *WanderHandler) rollFloat() float64 {
	h.rngMu.Lock()
	defer h.rngMu.Unlock()
	return h.rng.Float64()
}

// rollIntn returns rng.Intn(n) under rngMu.
func (h *WanderHandler) rollIntn(n int) int {
	h.rngMu.Lock()
	defer h.rngMu.Unlock()
	return h.rng.Intn(n)
}

// NewWanderHandler builds a handler bound to the given repos. The
// session registry may be nil; broadcasts are skipped in that case
// (useful in tests that don't care about peer notifications).
func NewWanderHandler(
	mobs repo.MobInstanceRepo,
	rooms repo.RoomRepo,
	exits repo.ExitRepo,
	templates repo.MobTemplateRepo,
	sessions *session.Registry,
	opts ...Option,
) *WanderHandler {
	h := &WanderHandler{
		mobs:      mobs,
		rooms:     rooms,
		exits:     exits,
		templates: templates,
		sessions:  sessions,
		chance:    DefaultWanderMultiplier,
		cap:       DefaultWanderCap,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Tick is the tick.HandlerFunc the bucket invokes each pulse. Iterates
// every spawned mob (capped), evaluates eligibility, rolls the wander
// chance, and on success moves the mob through a random legal exit
// inside its home zone.
//
// The handler is synchronous on purpose — tick.Bucket.fire runs every
// subscriber sequentially on its own goroutine, so spawning per-mob
// goroutines here would multiply contention on the repo without
// improving throughput.
func (h *WanderHandler) Tick(ctx context.Context) {
	if h == nil || h.cap <= 0 || h.chance <= 0 {
		return
	}
	mobs, err := h.mobs.ListSpawned(ctx, h.cap)
	if err != nil {
		slog.Warn("wander: list spawned failed", "error", err)
		return
	}
	// Cache (TemplateID -> wanderProfile) inside one pulse so a room
	// crowded with one species doesn't re-fetch the template per mob.
	profiles := make(map[int64]wanderProfile, 8)
	// Cache (RoomID -> ZoneID) for the same reason — a wandering pack
	// shares the same source/dest rooms across iterations.
	zone := make(map[int64]int64, 16)

	for _, m := range mobs {
		if ctx.Err() != nil {
			return
		}
		h.consider(ctx, m, profiles, zone)
	}
}

// wanderProfile is the slice of a MobTemplate the wander tick reads
// per pulse. Cached by TemplateID inside Tick so a swarm of the same
// species doesn't hit the template repo for every mob.
type wanderProfile struct {
	flags  creature.BehaviorFlags
	chance float64
}

func (h *WanderHandler) consider(
	ctx context.Context,
	m creature.MobInstance,
	profiles map[int64]wanderProfile,
	zone map[int64]int64,
) {
	if m.Core.CurrentRoomID == 0 {
		return
	}
	prof, ok := profiles[m.TemplateID]
	if !ok {
		tpl, err := h.templates.GetByID(ctx, m.TemplateID)
		if err != nil {
			slog.Debug("wander: template lookup failed", "mob", m.ID, "tpl", m.TemplateID, "error", err)
			return
		}
		prof = wanderProfile{flags: tpl.BehaviorFlags, chance: tpl.WanderChance}
		profiles[m.TemplateID] = prof
	}
	if prof.flags&creature.BehavSentinel != 0 {
		return
	}
	// Final per-pulse chance is the template's value scaled by the
	// handler's global multiplier, clamped at 1.0. A template with
	// chance 0 never wanders regardless of multiplier.
	final := prof.chance * h.chance
	if final <= 0 {
		return
	}
	if final > 1 {
		final = 1
	}
	if h.rollFloat() >= final {
		return
	}
	srcZone, ok := h.zoneOf(ctx, m.Core.CurrentRoomID, zone)
	if !ok {
		return
	}
	exits, err := h.exits.ListFrom(ctx, m.Core.CurrentRoomID)
	if err != nil {
		slog.Debug("wander: list exits failed", "room", m.Core.CurrentRoomID, "error", err)
		return
	}
	var candidates []repo.Exit
	for _, e := range exits {
		if !exitWalkable(e) {
			continue
		}
		dz, ok := h.zoneOf(ctx, e.ToRoomID, zone)
		if !ok || dz != srcZone {
			continue
		}
		candidates = append(candidates, e)
	}
	if len(candidates) == 0 {
		return
	}
	chosen := candidates[h.rollIntn(len(candidates))]
	from := m.Core.CurrentRoomID
	if err := h.mobs.UpdateRoom(ctx, m.ID, chosen.ToRoomID); err != nil {
		slog.Warn("wander: UpdateRoom failed", "mob", m.ID, "to", chosen.ToRoomID, "error", err)
		return
	}
	h.broadcast(m, from, chosen)
}

// exitWalkable mirrors the player move gate: a mob will not pass a
// hidden / closed / locked / nopass door. Mob door-handling is a
// future hook (would need a Smart behavior flag and key inventory).
func exitWalkable(e repo.Exit) bool {
	if e.ToRoomID == 0 {
		return false
	}
	if e.Flags.Hidden || e.Flags.Closed || e.Flags.Locked || e.Flags.NoPass {
		return false
	}
	return true
}

func (h *WanderHandler) zoneOf(ctx context.Context, roomID int64, cache map[int64]int64) (int64, bool) {
	if roomID == 0 {
		return 0, false
	}
	if z, ok := cache[roomID]; ok {
		return z, true
	}
	room, err := h.rooms.FindByID(ctx, roomID)
	if err != nil {
		slog.Debug("wander: room lookup failed", "room", roomID, "error", err)
		return 0, false
	}
	cache[roomID] = room.ZoneID
	return room.ZoneID, true
}

// broadcast notifies sessions in the source room of the mob's
// departure and sessions in the destination of its arrival. Mirrors
// the registry-snapshot-and-filter pattern used by `say` and the
// door verbs.
func (h *WanderHandler) broadcast(m creature.MobInstance, fromRoomID int64, exit repo.Exit) {
	if h.sessions == nil {
		return
	}
	name := safeMobName(m.Core.Name)
	leave := "{{" + name + " leaves " + repo.DirLong(exit.Direction) + ".}}::white\r\n"
	rev := repo.DirLong(reverseDir(exit.Direction))
	var arrive string
	if rev != "" {
		arrive = "{{" + name + " arrives from the " + rev + ".}}::white\r\n"
	} else {
		arrive = "{{" + name + " arrives.}}::white\r\n"
	}
	for _, peer := range h.sessions.Snapshot() {
		switch peer.CurrentRoomID {
		case fromRoomID:
			if err := peer.WriteString(leave); err != nil {
				slog.Debug("wander: peer write failed", "to", peer.CharacterName, "error", err)
			}
		case exit.ToRoomID:
			if err := peer.WriteString(arrive); err != nil {
				slog.Debug("wander: peer write failed", "to", peer.CharacterName, "error", err)
			}
		}
	}
}

// safeMobName defangs cfmt syntax and strips control bytes from a
// mob's display name. Mob names come from authored YAML (trusted),
// but defense-in-depth so a builder typo can never inject styling
// or terminal escapes into a peer's session.
func safeMobName(name string) string {
	if name == "" {
		return "Something"
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" {
		return "Something"
	}
	rep := strings.NewReplacer("{{", "{ {", "}}", "} }", "::", ": :")
	return rep.Replace(out)
}

func reverseDir(d string) string {
	switch d {
	case repo.DirNorth:
		return repo.DirSouth
	case repo.DirSouth:
		return repo.DirNorth
	case repo.DirEast:
		return repo.DirWest
	case repo.DirWest:
		return repo.DirEast
	case repo.DirUp:
		return repo.DirDown
	case repo.DirDown:
		return repo.DirUp
	case repo.DirNortheast:
		return repo.DirSouthwest
	case repo.DirSouthwest:
		return repo.DirNortheast
	case repo.DirNorthwest:
		return repo.DirSoutheast
	case repo.DirSoutheast:
		return repo.DirNorthwest
	}
	return ""
}
