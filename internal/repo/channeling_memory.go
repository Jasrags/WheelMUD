package repo

import (
	"context"
	"sync"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

type channelingKey struct {
	Kind OwnerKind
	ID   int64
}

// MemoryChannelingRepo is an in-memory ChannelingRepo for tests.
type MemoryChannelingRepo struct {
	mu   sync.Mutex
	rows map[channelingKey]creature.Channeling
}

func NewMemoryChannelingRepo() *MemoryChannelingRepo {
	return &MemoryChannelingRepo{rows: make(map[channelingKey]creature.Channeling)}
}

func (r *MemoryChannelingRepo) Upsert(_ context.Context, kind OwnerKind, ownerID int64, c creature.Channeling) error {
	if !kind.Valid() {
		return ErrInvalidOwnerKind
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[channelingKey{Kind: kind, ID: ownerID}] = c
	return nil
}

func (r *MemoryChannelingRepo) GetByOwner(_ context.Context, kind OwnerKind, ownerID int64) (creature.Channeling, error) {
	if !kind.Valid() {
		return creature.Channeling{}, ErrInvalidOwnerKind
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.rows[channelingKey{Kind: kind, ID: ownerID}]
	if !ok {
		return creature.Channeling{}, ErrChannelingNotFound
	}
	return c, nil
}

func (r *MemoryChannelingRepo) DeleteByOwner(_ context.Context, kind OwnerKind, ownerID int64) error {
	if !kind.Valid() {
		return ErrInvalidOwnerKind
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	k := channelingKey{Kind: kind, ID: ownerID}
	if _, ok := r.rows[k]; !ok {
		return ErrChannelingNotFound
	}
	delete(r.rows, k)
	return nil
}
