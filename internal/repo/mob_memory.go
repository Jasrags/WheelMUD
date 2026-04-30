package repo

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryMobRepo is an in-memory MobRepo for tests. Concurrent-safe.
type MemoryMobRepo struct {
	mu    sync.Mutex
	mobs  []Mob
	maxID int64
}

func NewMemoryMobRepo() *MemoryMobRepo { return &MemoryMobRepo{} }

// Insert adds a mob directly. Test fixtures use this to populate rooms.
// If m.ID is zero an id is auto-assigned.
func (r *MemoryMobRepo) Insert(m Mob) Mob {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.ID == 0 {
		r.maxID++
		m.ID = r.maxID
	} else if m.ID > r.maxID {
		r.maxID = m.ID
	}
	if m.NameLower == "" {
		m.NameLower = strings.ToLower(m.Name)
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	r.mobs = append(r.mobs, m)
	return m
}

func (r *MemoryMobRepo) ListInRoom(_ context.Context, roomID int64) ([]Mob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Mob
	for _, m := range r.mobs {
		if m.RoomID == roomID {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NameLower < out[j].NameLower })
	return out, nil
}
