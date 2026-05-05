package world

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
)

// transitionStyles maps each Transition to a cfmt color tag. Color is
// chosen to mirror the natural light at that boundary: cool first light
// at dawn, warm gold at sunrise, orange at dusk, cold blue at nightfall.
var transitionStyles = [4]string{
	TransitionDawn:      "cyan",
	TransitionSunrise:   "yellow",
	TransitionDusk:      "lightred",
	TransitionNightfall: "blue",
}

// Transition names a phase boundary crossing. The watcher emits exactly
// one ambient line per Transition per crossing.
type Transition int

const (
	// TransitionDawn fires on night → dawn (the first hint of light).
	TransitionDawn Transition = iota
	// TransitionSunrise fires on dawn → day (full daylight).
	TransitionSunrise
	// TransitionDusk fires on day → dusk (light starts to fail).
	TransitionDusk
	// TransitionNightfall fires on dusk → night (darkness settles in).
	TransitionNightfall
)

// transitionFor returns the Transition crossed when the phase moves
// from prev to curr. Only the four forward-adjacent edges qualify; any
// other change (no-op, reverse, multi-step jump from a clock rebase)
// returns false so we don't emit a stale or duplicated line.
func transitionFor(prev, curr Phase) (Transition, bool) {
	switch {
	case prev == PhaseNight && curr == PhaseDawn:
		return TransitionDawn, true
	case prev == PhaseDawn && curr == PhaseDay:
		return TransitionSunrise, true
	case prev == PhaseDay && curr == PhaseDusk:
		return TransitionDusk, true
	case prev == PhaseDusk && curr == PhaseNight:
		return TransitionNightfall, true
	default:
		return 0, false
	}
}

// sectorAmbients maps each outdoor Sector to a four-line table indexed
// by Transition. Underground/Underwater rooms are filtered before
// lookup (see broadcast); they intentionally have no entry. An empty
// Sector falls back to SectorCity to mirror the policy in look /
// whereami / EffectiveLight.
var sectorAmbients = map[repo.Sector][4]string{
	repo.SectorCity: {
		TransitionDawn:      "Pale light creeps along the rooftops, and the first shutters bang open down the lane.",
		TransitionSunrise:   "The sun clears the eaves; smoke from breakfast fires curls into a brightening sky.",
		TransitionDusk:      "Long shadows stripe the cobbles as lamplighters start their rounds.",
		TransitionNightfall: "The last shutters clatter closed and the street settles into lamplight and quiet.",
	},
	repo.SectorForest: {
		TransitionDawn:      "Birdsong climbs through the canopy as grey light filters between the trunks.",
		TransitionSunrise:   "Shafts of sunlight pierce the leaves and steam lifts from the forest floor.",
		TransitionDusk:      "The wood goes still; somewhere deeper in the trees, an owl tries its first call.",
		TransitionNightfall: "Darkness settles thick between the trunks, and the forest takes on its night voices.",
	},
	repo.SectorField: {
		TransitionDawn:      "Dew silvers the grass as the eastern horizon pales.",
		TransitionSunrise:   "The sun lifts clear of the grasses, and a meadowlark answers from somewhere out in the green.",
		TransitionDusk:      "The grass goes copper, then grey, as the sun slips toward the rim of the world.",
		TransitionNightfall: "Stars prick through the deepening sky; the field hushes.",
	},
	repo.SectorHills: {
		TransitionDawn:      "Light spills across the ridgelines and the valleys below stay pooled in shadow.",
		TransitionSunrise:   "The sun crests the hills and warmth begins to chase the chill from the slopes.",
		TransitionDusk:      "The hills throw long shadows eastward; wind carries the smell of cooling stone.",
		TransitionNightfall: "The ridges fade to silhouettes against the last of the light, then to nothing.",
	},
	repo.SectorMountain: {
		TransitionDawn:      "First light strikes the peaks far above, leaving the passes still in cold blue shadow.",
		TransitionSunrise:   "Sunlight finally reaches the path; the rocks begin to give back their stored chill.",
		TransitionDusk:      "Shadow rises out of the valleys and climbs the slopes; the wind sharpens.",
		TransitionNightfall: "The cold settles in hard, and the stars overhead burn unnaturally bright.",
	},
	repo.SectorDesert: {
		TransitionDawn:      "The eastern sky bleeds orange, and the sand begins to give up the night's chill.",
		TransitionSunrise:   "The sun heaves itself over the dunes, already merciless.",
		TransitionDusk:      "The heat lets go all at once; long shadows knife across the sand.",
		TransitionNightfall: "The desert turns black and bitter cold, the sky a wash of stars.",
	},
	repo.SectorWater: {
		TransitionDawn:      "The water turns from black to pewter as the horizon brightens.",
		TransitionSunrise:   "Sunlight scatters across the surface in broken, restless light.",
		TransitionDusk:      "The water goes molten, then dim, as the sun touches the horizon.",
		TransitionNightfall: "The surface fades to dark glass, broken only by the reflections of stars.",
	},
	repo.SectorAir: {
		TransitionDawn:      "Far below, the world is still in shadow, but here aloft the light has already found you.",
		TransitionSunrise:   "The sun clears the cloud-line and the wind takes on its day-warmth.",
		TransitionDusk:      "The sky around you flares red and gold; below, the land is already dimming.",
		TransitionNightfall: "Stars come out wherever you look, and the wind goes cold and clean.",
	},
	repo.SectorSwamp: {
		TransitionDawn:      "Mist rolls thick between the cypress knees as the sky pales toward grey.",
		TransitionSunrise:   "Sunlight breaks the mist into ragged shreds; somewhere a bullfrog answers another.",
		TransitionDusk:      "The water blackens and the insects rise; fireflies begin their slow drift.",
		TransitionNightfall: "The swamp settles into its night chorus, low and wet and unbroken.",
	},
	repo.SectorWaste: {
		TransitionDawn:      "First light strikes the red rock; the air, still cold, smells of stone and dry sage.",
		TransitionSunrise:   "The sun heaves over the ridges and the heat begins its slow climb up the canyon walls.",
		TransitionDusk:      "The cliffs go copper, then violet; a hawk slips into the shadow of the stone.",
		TransitionNightfall: "The waste empties of warmth; the stars overhead are sharper than knives.",
	},
	repo.SectorStedding: {
		TransitionDawn:      "Dawn comes gentle here; the great trees stir and the air feels older than the morning.",
		TransitionSunrise:   "Sunlight settles between the boughs without hurry, and a deep stillness holds.",
		TransitionDusk:      "The grove dims slowly, as if reluctant to give up the day.",
		TransitionNightfall: "Night falls hushed; the trees hold the silence the way a chord holds a note.",
	},
	repo.SectorBlight: {
		TransitionDawn:      "The pallid sky lightens but the air does not; a sweetish rot drifts on the wind.",
		TransitionSunrise:   "What passes for sunlight reaches the ground sallow and wrong; the twisted growths shift toward it.",
		TransitionDusk:      "The light fails early here, and the things that whisper between the stems grow bolder.",
		TransitionNightfall: "Full dark — and the Blight is louder at night, all wet sounds and faint, far cries.",
	},
}

// ambientLine returns the line for (sector, transition), defaulting an
// empty sector to SectorCity. Returns "" if the sector has no entry
// (Underground/Underwater) so the caller can skip without writing.
func ambientLine(sector repo.Sector, t Transition) string {
	if sector == "" {
		sector = repo.SectorCity
	}
	lines, ok := sectorAmbients[sector]
	if !ok {
		return ""
	}
	return lines[t]
}

// PhaseAmbientWatcher polls the day/night clock and emits a per-sector
// ambient line to outdoor non-Silent rooms whenever the phase
// advances. Construction seeds lastPhase to the current phase so a
// fresh boot doesn't broadcast a spurious transition on the first
// poll.
type PhaseAmbientWatcher struct {
	clock    *Clock
	rooms    repo.RoomRepo
	sessions *session.Registry

	mu        sync.Mutex
	lastPhase Phase
}

// NewPhaseAmbientWatcher constructs a watcher seeded with the current
// phase. clock, rooms, and sessions must be non-nil.
func NewPhaseAmbientWatcher(clock *Clock, rooms repo.RoomRepo, sessions *session.Registry) *PhaseAmbientWatcher {
	return &PhaseAmbientWatcher{
		clock:     clock,
		rooms:     rooms,
		sessions:  sessions,
		lastPhase: clock.Phase(),
	}
}

// Tick is the tick.HandlerFunc-shaped entry point. Reads the current
// phase, exits early on no-op, and broadcasts the matched transition
// to every qualifying session. Safe to call from a tick goroutine.
func (w *PhaseAmbientWatcher) Tick(ctx context.Context) {
	w.mu.Lock()
	prev := w.lastPhase
	curr := w.clock.Phase()
	if prev == curr {
		w.mu.Unlock()
		return
	}
	w.lastPhase = curr
	w.mu.Unlock()

	t, ok := transitionFor(prev, curr)
	if !ok {
		// Non-adjacent jump (e.g. future `time set` rebase). Don't
		// pretend a single boundary was crossed; advancing lastPhase
		// resyncs us for the next clean transition.
		return
	}
	w.broadcast(ctx, t)
}

// broadcast walks the session registry and writes the appropriate
// sector line to each player whose room qualifies. Errors are logged
// at debug and never propagated, matching the broadcast helpers in
// internal/cmd (a single misbehaving session can't suppress everyone
// else's ambient).
func (w *PhaseAmbientWatcher) broadcast(ctx context.Context, t Transition) {
	for _, s := range w.sessions.Snapshot() {
		if s == nil {
			continue
		}
		charID, charName, roomID := s.InWorld()
		if charID == 0 || roomID == 0 {
			continue
		}
		room, err := w.rooms.FindByID(ctx, roomID)
		if err != nil {
			if !errors.Is(err, repo.ErrRoomNotFound) {
				slog.Debug("phase ambient: room lookup failed",
					"session", charName, "roomID", roomID, "error", err)
			}
			continue
		}
		if !roomReceivesAmbient(room) {
			continue
		}
		line := ambientLine(room.Sector, t)
		if line == "" {
			continue
		}
		line = fmt.Sprintf("{{%s}}::%s", line, transitionStyles[t])
		if err := s.WriteAsync(line); err != nil {
			slog.Debug("phase ambient: write failed",
				"session", charName, "error", err)
		}
	}
}

// roomReceivesAmbient applies the same outdoor gate that
// Clock.EffectiveLight uses to decide whether the cycle affects light,
// plus a Silent-room mute. Indoor / underground / underwater / Dark
// rooms see no light ramp, so they shouldn't receive narration about
// it either; Silent rooms (sacred spaces, dream rooms) suppress
// environmental chatter as well.
func roomReceivesAmbient(room repo.Room) bool {
	if room.Flags.Silent || room.Flags.Dark || room.Flags.Indoors {
		return false
	}
	if room.Sector == repo.SectorUnderground || room.Sector == repo.SectorUnderwater {
		return false
	}
	// Avoid silently swallowing a future sector with no ambient table:
	// require the sector to resolve to a non-empty line set. Empty
	// sector is allowed (defaults to city in ambientLine).
	if room.Sector == "" {
		return true
	}
	_, ok := sectorAmbients[room.Sector]
	return ok
}
