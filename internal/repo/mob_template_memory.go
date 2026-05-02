package repo

import (
	"context"
	"sync"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// MemoryMobTemplateRepo is an in-memory MobTemplateRepo for tests.
// Concurrent-safe. Stored values are deep-copied through the
// existing JSON helpers' clone behavior on read so callers can
// mutate returned slices without corrupting the store.
type MemoryMobTemplateRepo struct {
	mu        sync.Mutex
	byID      map[int64]creature.MobTemplate
	byExt     map[string]int64
	maxID     int64
}

func NewMemoryMobTemplateRepo() *MemoryMobTemplateRepo {
	return &MemoryMobTemplateRepo{
		byID:  make(map[int64]creature.MobTemplate),
		byExt: make(map[string]int64),
	}
}

func (r *MemoryMobTemplateRepo) Create(_ context.Context, t creature.MobTemplate) (creature.MobTemplate, error) {
	if t.ExternalID == "" {
		return creature.MobTemplate{}, ErrInvalidExternalID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byExt[t.ExternalID]; dup {
		return creature.MobTemplate{}, ErrDuplicateExternalID
	}
	r.maxID++
	t.ID = r.maxID
	t.Core.ID = r.maxID
	r.byID[t.ID] = t
	r.byExt[t.ExternalID] = t.ID
	return t, nil
}

func (r *MemoryMobTemplateRepo) GetByID(_ context.Context, id int64) (creature.MobTemplate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[id]
	if !ok {
		return creature.MobTemplate{}, ErrTemplateNotFound
	}
	return t, nil
}

func (r *MemoryMobTemplateRepo) GetByExternalID(_ context.Context, externalID string) (creature.MobTemplate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byExt[externalID]
	if !ok {
		return creature.MobTemplate{}, ErrTemplateNotFound
	}
	return r.byID[id], nil
}
