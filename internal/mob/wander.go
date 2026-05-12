// Package mob holds the gameplay-driven mob runners that sit between
// the persistence layer (repo) and the game loop (tick). Today this
// is just the wander tick; combat AI, aggro, and corpse decay will
// land alongside it as §11 / §12 progress.
package mob

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/display"
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
	if h == nil || h.cap <= 0 {
		return
	}
	// Note: a zero multiplier (h.chance <= 0) is NOT a short-circuit
	// here — strict-path templates (Phase F #32a slice 1) still need
	// to advance. The per-mob chance gate inside consider() handles
	// the random-wander branch.
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

	// pathRoomIDs is the resolved-at-cache-populate set of internal
	// room IDs for an authored mob path (Phase F #32a slice 1).
	// Empty means "no path; use chance for random wandering". When
	// non-empty, consider() switches to the strict-path branch and
	// ignores chance.
	pathRoomIDs []int64
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
		// Resolve authored path external_ids → internal room IDs the
		// first time we see this template in the pulse. Resolution
		// failures (room renamed / deleted after deploy) drop the path
		// silently so the mob still wanders randomly via the chance
		// branch — preferable to wedging mid-patrol.
		if len(tpl.Path) > 0 {
			ids := make([]int64, 0, len(tpl.Path))
			for _, ext := range tpl.Path {
				room, rerr := h.rooms.FindByExternalID(ctx, ext)
				if rerr != nil {
					slog.Warn("wander: authored path room not found",
						"tpl", tpl.ExternalID, "missing_room", ext, "error", rerr)
					ids = nil
					break
				}
				ids = append(ids, room.ID)
			}
			prof.pathRoomIDs = ids
		}
		profiles[m.TemplateID] = prof
	}
	if prof.flags&creature.BehavSentinel != 0 {
		return
	}
	if len(prof.pathRoomIDs) > 0 {
		h.considerStrictPath(ctx, m, prof, zone)
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

// considerStrictPath handles the Phase F #32a slice 1 authored path
// branch. The mob's current room must appear in prof.pathRoomIDs;
// when it does, we step to the next room in the closed loop and
// move via the existing walkable-exit gate. An off-path mob (admin
// teleport, mid-deploy path edit) silently no-ops until they land
// back on a path entry — preserves authoring intent without forcing
// the mob into a route they didn't author.
func (h *WanderHandler) considerStrictPath(
	ctx context.Context,
	m creature.MobInstance,
	prof wanderProfile,
	zone map[int64]int64,
) {
	// Find the mob's current position on the authored path. Loader
	// validation guarantees no duplicates so the first match is
	// unambiguous.
	idx := -1
	for i, rid := range prof.pathRoomIDs {
		if rid == m.Core.CurrentRoomID {
			idx = i
			break
		}
	}
	if idx == -1 {
		// Off-path. Loader-validated adjacency means we can't recover
		// by guessing; admin needs to teleport the mob back. Log at
		// debug because this is expected during reset / spawn-anchor
		// transitions where the spawn room may not be on the path.
		slog.Debug("wander: mob off authored path",
			"mob", m.ID, "tpl", m.TemplateID, "room", m.Core.CurrentRoomID)
		return
	}
	target := prof.pathRoomIDs[(idx+1)%len(prof.pathRoomIDs)]

	// Adjacency was validated at boot, but a runtime door close
	// (NPC slammed a door, scripted lock) can invalidate the step.
	// Look up the exit fresh each pulse so the mob waits politely
	// for a closed door to open rather than tunneling through it.
	exits, err := h.exits.ListFrom(ctx, m.Core.CurrentRoomID)
	if err != nil {
		slog.Debug("wander: list exits failed (path)", "room", m.Core.CurrentRoomID, "error", err)
		return
	}
	var chosen repo.Exit
	found := false
	for _, e := range exits {
		if !exitWalkable(e) {
			continue
		}
		if e.ToRoomID != target {
			continue
		}
		chosen = e
		found = true
		break
	}
	if !found {
		// Door closed mid-deploy, or builder-time adjacency drifted.
		// Stay put this pulse; retry next tick.
		slog.Debug("wander: path step blocked",
			"mob", m.ID, "from", m.Core.CurrentRoomID, "to", target)
		return
	}
	// Zone gate: same as the random-wander branch — refuse to leave
	// the home zone, even when the authored path crosses zone lines.
	// Builders should keep paths zone-local; if a future need lands
	// cross-zone patrols, lift this gate behind a per-template flag.
	srcZone, ok := h.zoneOf(ctx, m.Core.CurrentRoomID, zone)
	if !ok {
		return
	}
	dstZone, ok := h.zoneOf(ctx, target, zone)
	if !ok || dstZone != srcZone {
		slog.Debug("wander: path step crosses zone, refusing",
			"mob", m.ID, "from_zone", srcZone, "to_zone", dstZone)
		return
	}

	from := m.Core.CurrentRoomID
	if err := h.mobs.UpdateRoom(ctx, m.ID, target); err != nil {
		slog.Warn("wander: UpdateRoom failed (path)", "mob", m.ID, "to", target, "error", err)
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
	leave := "{{" + name + " leaves " + repo.DirLong(exit.Direction) + ".}}::white"
	rev := repo.DirLong(reverseDir(exit.Direction))
	var arrive string
	if rev != "" {
		arrive = "{{" + name + " arrives from the " + rev + ".}}::white"
	} else {
		arrive = "{{" + name + " arrives.}}::white"
	}
	for _, peer := range h.sessions.Snapshot() {
		_, peerName, peerRoom := peer.InWorld()
		switch peerRoom {
		case fromRoomID:
			if err := peer.WriteAsync(leave); err != nil {
				slog.Debug("wander: peer write failed", "to", peerName, "error", err)
			}
		case exit.ToRoomID:
			if err := peer.WriteAsync(arrive); err != nil {
				slog.Debug("wander: peer write failed", "to", peerName, "error", err)
			}
		}
	}
}

// safeMobName defangs cfmt syntax and strips control bytes from a
// mob's display name. Mob names come from authored YAML (trusted),
// but defense-in-depth so a builder typo can never inject styling
// or terminal escapes into a peer's session.
func safeMobName(name string) string { return display.Defang(name, "Something") }

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
