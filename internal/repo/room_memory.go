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
// Create.
func (r *MemoryRoomRepo) Insert(room Room) Room {
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
	if len(room.ExtraDescs) > 0 {
		normalized := make(map[string]string, len(room.ExtraDescs))
		for k, v := range room.ExtraDescs {
			normalized[strings.ToLower(strings.TrimSpace(k))] = v
		}
		room.ExtraDescs = normalized
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byExt[room.ExternalID]; exists {
		return Room{}, ErrDuplicateExternalID
	}
	return r.insertLocked(room), nil
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
