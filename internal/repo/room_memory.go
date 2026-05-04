package repo

import (
	"context"
	"strings"
	"sync"
	"time"
)

// MemoryRoomRepo is an in-memory RoomRepo for tests. Concurrent-safe.
type MemoryRoomRepo struct {
	mu    sync.Mutex
	byID  map[int64]*Room
	byExt map[string]*Room
	maxID int64
}

func NewMemoryRoomRepo() *MemoryRoomRepo {
	return &MemoryRoomRepo{
		byID:  make(map[int64]*Room),
		byExt: make(map[string]*Room),
	}
}

// Insert adds a room directly without ExternalID validation. Test
// fixtures use this; production code (the YAML loader) goes through
// Create. ExtraDescs keys are normalized so the case-insensitive
// look <noun> contract matches whichever entry path produced the row;
// LightLevel defaults to DefaultLightLevel when zero (mirroring the
// YAML loader) so a fixture that doesn't set it doesn't accidentally
// render pitch-black under the §9 day/night cycle. Pass `Flags.Dark`
// or set `LightLevel: -1` (clamped to 0 by the renderer) to opt into
// a deliberately unlit room.
func (r *MemoryRoomRepo) Insert(room Room) Room {
	room.ExtraDescs = normalizeExtraDescs(room.ExtraDescs)
	if room.LightLevel == 0 && !room.Flags.Dark {
		room.LightLevel = DefaultLightLevel
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.insertLocked(room)
}

func (r *MemoryRoomRepo) Create(_ context.Context, room Room) (Room, error) {
	if room.ExternalID == "" {
		return Room{}, ErrInvalidExternalID
	}
	if room.Sector == "" {
		room.Sector = SectorCity
	}
	// Note: LightLevel defaulting deliberately lives only in Insert
	// (test-fixture path). Create preserves caller intent — the YAML
	// loader applies the default upstream, so a Create caller passing
	// 0 means 0, and a contract test pins that.
	room.ExtraDescs = normalizeExtraDescs(room.ExtraDescs)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byExt[room.ExternalID]; exists {
		return Room{}, ErrDuplicateExternalID
	}
	return r.insertLocked(room), nil
}

// normalizeExtraDescs lowercases + trims keys so look-keyword matches
// regardless of how the row was authored. Returns nil for an empty
// input so storage stays sparse.
func normalizeExtraDescs(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[strings.ToLower(strings.TrimSpace(k))] = v
	}
	return out
}

func (r *MemoryRoomRepo) insertLocked(room Room) Room {
	if room.ID == 0 {
		r.maxID++
		room.ID = r.maxID
	} else if room.ID > r.maxID {
		r.maxID = room.ID
	}
	if room.CreatedAt.IsZero() {
		room.CreatedAt = time.Now().UTC()
	}
	stored := room
	r.byID[room.ID] = &stored
	if room.ExternalID != "" {
		r.byExt[room.ExternalID] = &stored
	}
	return stored
}

func (r *MemoryRoomRepo) FindByID(_ context.Context, id int64) (Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.byID[id]
	if !ok {
		return Room{}, ErrRoomNotFound
	}
	return *room, nil
}

func (r *MemoryRoomRepo) FindByExternalID(_ context.Context, externalID string) (Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.byExt[externalID]
	if !ok {
		return Room{}, ErrRoomNotFound
	}
	return *room, nil
}

func (r *MemoryRoomRepo) CountByZone(_ context.Context, zoneID int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int
	for _, room := range r.byID {
		if room.ZoneID == zoneID {
			n++
		}
	}
	return n, nil
}
