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
	byExt map[string]struct{}
	maxID int64
}

func NewMemoryMobRepo() *MemoryMobRepo {
	return &MemoryMobRepo{byExt: make(map[string]struct{})}
}

// Insert adds a mob directly without ExternalID validation. Test
// fixtures use this; production code (the YAML loader) goes through
// Create.
func (r *MemoryMobRepo) Insert(m Mob) Mob {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.insertLocked(m)
}

func (r *MemoryMobRepo) Create(_ context.Context, m Mob) (Mob, error) {
	if m.ExternalID == "" {
		return Mob{}, ErrInvalidExternalID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byExt[m.ExternalID]; dup {
		return Mob{}, ErrDuplicateExternalID
	}
	return r.insertLocked(m), nil
}

func (r *MemoryMobRepo) insertLocked(m Mob) Mob {
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
	if m.ExternalID != "" {
		r.byExt[m.ExternalID] = struct{}{}
	}
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
