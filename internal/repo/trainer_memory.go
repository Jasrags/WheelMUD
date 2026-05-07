package repo

import (
	"context"
	"sort"
	"sync"
)

// MemoryTrainerRepo is a map-backed TrainerRepo for tests and
// non-persistent runs. Concurrent-safe.
type MemoryTrainerRepo struct {
	mu       sync.RWMutex
	nextID   int64
	trainers map[int64]Trainer
	byMobTpl map[int64]int64 // mob_template_id -> trainer_id
}

func NewMemoryTrainerRepo() *MemoryTrainerRepo {
	return &MemoryTrainerRepo{
		trainers: make(map[int64]Trainer),
		byMobTpl: make(map[int64]int64),
	}
}

func (r *MemoryTrainerRepo) Create(_ context.Context, t Trainer) (Trainer, error) {
	if t.MobTemplateID == 0 {
		return Trainer{}, ErrInvalidExternalID
	}
	if t.ClassID == "" {
		return Trainer{}, ErrInvalidExternalID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byMobTpl[t.MobTemplateID]; dup {
		return Trainer{}, ErrDuplicateExternalID
	}
	r.nextID++
	t.ID = r.nextID
	r.trainers[t.ID] = t
	r.byMobTpl[t.MobTemplateID] = t.ID
	return t, nil
}

func (r *MemoryTrainerRepo) GetByMobTemplateID(_ context.Context, mobTemplateID int64) (Trainer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byMobTpl[mobTemplateID]
	if !ok {
		return Trainer{}, ErrTrainerNotFound
	}
	return r.trainers[id], nil
}

func (r *MemoryTrainerRepo) ListTrainers(_ context.Context) ([]Trainer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Trainer, 0, len(r.trainers))
	for _, t := range r.trainers {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
