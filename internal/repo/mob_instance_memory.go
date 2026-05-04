package repo

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// MemoryMobInstanceRepo is an in-memory MobInstanceRepo for tests.
type MemoryMobInstanceRepo struct {
	mu     sync.Mutex
	byID   map[int64]creature.MobInstance
	trails map[int64][]MobTrail
	maxID  int64
}

func NewMemoryMobInstanceRepo() *MemoryMobInstanceRepo {
	return &MemoryMobInstanceRepo{
		byID:   make(map[int64]creature.MobInstance),
		trails: make(map[int64][]MobTrail),
	}
}

func (r *MemoryMobInstanceRepo) Create(_ context.Context, m creature.MobInstance) (creature.MobInstance, error) {
	if m.TemplateID == 0 {
		return creature.MobInstance{}, fmt.Errorf("mob_instance.Create: TemplateID required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.maxID++
	m.ID = r.maxID
	m.Core.ID = r.maxID
	if m.SpawnedAt.IsZero() {
		m.SpawnedAt = time.Now().UTC()
	}
	r.byID[m.ID] = m
	return m, nil
}

func (r *MemoryMobInstanceRepo) GetByID(_ context.Context, id int64) (creature.MobInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.byID[id]
	if !ok {
		return creature.MobInstance{}, ErrInstanceNotFound
	}
	return m, nil
}

func (r *MemoryMobInstanceRepo) ListInRoom(_ context.Context, roomID int64) ([]creature.MobInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []creature.MobInstance
	for _, m := range r.byID {
		if m.Core.CurrentRoomID == roomID {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *MemoryMobInstanceRepo) ListSpawned(_ context.Context, limit int) ([]creature.MobInstance, error) {
	if limit <= 0 {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []creature.MobInstance
	for _, m := range r.byID {
		if m.Core.CurrentRoomID == 0 {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *MemoryMobInstanceRepo) UpdateLive(_ context.Context, id int64, hp, subdual int32, cond creature.Condition, pos creature.PositionFlags) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.byID[id]
	if !ok {
		return ErrInstanceNotFound
	}
	m.Core.HPCurrent = hp
	m.Core.Subdual = subdual
	m.Core.Conditions = cond
	m.Core.Position = pos
	r.byID[id] = m
	return nil
}

func (r *MemoryMobInstanceRepo) UpdateRoom(_ context.Context, id, roomID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.byID[id]
	if !ok {
		return ErrInstanceNotFound
	}
	m.Core.CurrentRoomID = roomID
	r.byID[id] = m
	if roomID == 0 {
		// Removed from world but not despawned — mirror the sqlite
		// branch and skip the trail entry.
		return nil
	}
	// Always rebuild into a fresh backing array; any slice handed out
	// by RecentTrails must remain stable even if the caller holds it
	// across a subsequent UpdateRoom on the same mob.
	old := r.trails[id]
	keep := len(old) + 1
	if keep > MobTrailCap {
		keep = MobTrailCap
	}
	tail := make([]MobTrail, keep)
	if len(old) >= keep-1 {
		copy(tail, old[len(old)-(keep-1):])
	} else {
		copy(tail, old)
	}
	tail[keep-1] = MobTrail{MobID: id, RoomID: roomID, At: time.Now().UTC()}
	r.trails[id] = tail
	return nil
}

func (r *MemoryMobInstanceRepo) RecentTrails(_ context.Context, mobID int64, limit int) ([]MobTrail, error) {
	if limit <= 0 {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	src := r.trails[mobID]
	if len(src) == 0 {
		return nil, nil
	}
	// Newest first — append in reverse, capped at limit.
	n := len(src)
	if n > limit {
		n = limit
	}
	out := make([]MobTrail, 0, n)
	for i := len(src) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, src[i])
	}
	return out, nil
}

func (r *MemoryMobInstanceRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return ErrInstanceNotFound
	}
	delete(r.byID, id)
	delete(r.trails, id)
	return nil
}
