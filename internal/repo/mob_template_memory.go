package repo

import (
	"context"
	"sort"
	"sync"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// MemoryMobTemplateRepo is an in-memory MobTemplateRepo for tests.
// Concurrent-safe. Templates are stored as value types, so callers
// reading through GetByID / GetByExternalID get a shallow copy of
// the row — mutating scalar fields on the returned value is safe,
// but mutating slice/map fields will alias the stored row. Treat
// returned values as read-only.
type MemoryMobTemplateRepo struct {
	mu    sync.Mutex
	byID  map[int64]creature.MobTemplate
	byExt map[string]int64
	maxID int64
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

func (r *MemoryMobTemplateRepo) ListExternalIDs(_ context.Context) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.byExt))
	for ext := range r.byExt {
		out = append(out, ext)
	}
	sort.Strings(out)
	return out, nil
}
