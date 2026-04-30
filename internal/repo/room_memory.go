package repo

import (
	"context"
	"sync"
	"time"
)

// MemoryRoomRepo is an in-memory RoomRepo for tests. Concurrent-safe.
// Pre-seeded via Insert; there is no public Create method until the world
// loader lands (intentionally mirrors the seed-only state of the SQLite
// repo's interface today).
type MemoryRoomRepo struct {
	mu    sync.Mutex
	byID  map[int64]*Room
	maxID int64
}

func NewMemoryRoomRepo() *MemoryRoomRepo {
	return &MemoryRoomRepo{byID: make(map[int64]*Room)}
}

// Insert adds a room directly. Test fixtures use this to seed the map.
// If r.ID is zero an id is auto-assigned starting at 1.
func (r *MemoryRoomRepo) Insert(room Room) Room {
	r.mu.Lock()
	defer r.mu.Unlock()
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
