package repo

import (
	"context"
	"sort"
	"sync"
)

// MemoryTriggerRepo is a map-backed TriggerRepo for tests and
// non-persistent runs. Concurrent-safe.
type MemoryTriggerRepo struct {
	mu       sync.RWMutex
	nextID   int64
	triggers map[int64]Trigger
}

func NewMemoryTriggerRepo() *MemoryTriggerRepo {
	return &MemoryTriggerRepo{triggers: make(map[int64]Trigger)}
}

func (r *MemoryTriggerRepo) Create(_ context.Context, t Trigger) (Trigger, error) {
	if err := validateTrigger(t); err != nil {
		return Trigger{}, err
	}
	if t.Payload == "" {
		t.Payload = "{}"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	t.ID = r.nextID
	r.triggers[t.ID] = t
	return t, nil
}

func (r *MemoryTriggerRepo) ListByOwner(_ context.Context, kind TriggerOwnerKind, ownerID int64) ([]Trigger, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Trigger, 0)
	for _, t := range r.triggers {
		if t.OwnerKind == kind && t.OwnerID == ownerID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (r *MemoryTriggerRepo) ListAll(_ context.Context) ([]Trigger, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Trigger, 0, len(r.triggers))
	for _, t := range r.triggers {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *MemoryTriggerRepo) DeleteByOwner(_ context.Context, kind TriggerOwnerKind, ownerID int64) error {
	if !ValidTriggerOwnerKind(kind) {
		return ErrInvalidTrigger
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, t := range r.triggers {
		if t.OwnerKind == kind && t.OwnerID == ownerID {
			delete(r.triggers, id)
		}
	}
	return nil
}
