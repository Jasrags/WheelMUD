package repo

import (
	"context"
	"sync"
)

// WorldStateRepo is the persistence boundary for the singleton world
// clock. The dayclock (internal/world/dayclock.go) reads GetTicks once
// at boot and writes the live tick count back through SetTicks via the
// persist.Manager Save bucket — periodic + shutdown — so a restart
// resumes near where it left off.
type WorldStateRepo interface {
	// GetTicks returns the persisted tick count. A fresh database
	// (migration 0024 just ran) returns 675 (noon on the curve).
	GetTicks(ctx context.Context) (int64, error)
	// SetTicks overwrites the persisted tick count.
	SetTicks(ctx context.Context, ticks int64) error
}

// MemoryWorldStateRepo is the in-memory impl for tests. Mirrors the
// 675 default of migration 0024 so tests don't have to special-case
// the "first boot" path.
type MemoryWorldStateRepo struct {
	mu    sync.Mutex
	ticks int64
}

func NewMemoryWorldStateRepo() *MemoryWorldStateRepo {
	return &MemoryWorldStateRepo{ticks: 675}
}

func (r *MemoryWorldStateRepo) GetTicks(_ context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ticks, nil
}

func (r *MemoryWorldStateRepo) SetTicks(_ context.Context, ticks int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ticks = ticks
	return nil
}
