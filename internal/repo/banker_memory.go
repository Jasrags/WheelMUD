package repo

import (
	"context"
	"sort"
	"sync"
)

// MemoryBankerRepo is a map-backed BankerRepo for tests and
// non-persistent runs. Concurrent-safe.
type MemoryBankerRepo struct {
	mu       sync.RWMutex
	nextID   int64
	bankers  map[int64]Banker // by id
	byMobTpl map[int64]int64  // mob_template_id -> banker_id
}

func NewMemoryBankerRepo() *MemoryBankerRepo {
	return &MemoryBankerRepo{
		bankers:  make(map[int64]Banker),
		byMobTpl: make(map[int64]int64),
	}
}

func (r *MemoryBankerRepo) Create(_ context.Context, b Banker) (Banker, error) {
	if b.MobTemplateID == 0 {
		return Banker{}, ErrInvalidExternalID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byMobTpl[b.MobTemplateID]; dup {
		return Banker{}, ErrDuplicateExternalID
	}
	r.nextID++
	b.ID = r.nextID
	r.bankers[b.ID] = b
	r.byMobTpl[b.MobTemplateID] = b.ID
	return b, nil
}

func (r *MemoryBankerRepo) GetByMobTemplateID(_ context.Context, mobTemplateID int64) (Banker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byMobTpl[mobTemplateID]
	if !ok {
		return Banker{}, ErrBankerNotFound
	}
	return r.bankers[id], nil
}

func (r *MemoryBankerRepo) ListBankers(_ context.Context) ([]Banker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Banker, 0, len(r.bankers))
	for _, b := range r.bankers {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
