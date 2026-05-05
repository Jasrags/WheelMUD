package repo

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryAdminAuditRepo is a slice-backed AdminAuditRepo for tests and
// non-persistent runs. Concurrent-safe.
type MemoryAdminAuditRepo struct {
	mu      sync.RWMutex
	nextID  int64
	entries []AdminAuditEntry
}

func NewMemoryAdminAuditRepo() *MemoryAdminAuditRepo {
	return &MemoryAdminAuditRepo{}
}

func (r *MemoryAdminAuditRepo) Record(_ context.Context, e AdminAuditEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	e.ID = r.nextID
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	r.entries = append(r.entries, e)
	return nil
}

func (r *MemoryAdminAuditRepo) List(_ context.Context, f AdminAuditFilter) ([]AdminAuditEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	verbSet := map[string]struct{}{}
	for _, v := range f.Verbs {
		verbSet[v] = struct{}{}
	}

	out := make([]AdminAuditEntry, 0, len(r.entries))
	for _, e := range r.entries {
		if !f.Since.IsZero() && e.TS.Before(f.Since) {
			continue
		}
		if f.Actor != 0 && e.ActorCharacterID != f.Actor {
			continue
		}
		if len(verbSet) > 0 {
			if _, ok := verbSet[e.Verb]; !ok {
				continue
			}
		}
		out = append(out, e)
	}

	// Newest first.
	sort.Slice(out, func(i, j int) bool {
		if !out[i].TS.Equal(out[j].TS) {
			return out[i].TS.After(out[j].TS)
		}
		return out[i].ID > out[j].ID
	})

	limit := f.Limit
	if limit <= 0 {
		limit = DefaultAdminAuditListLimit
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Len exposes the entry count for tests asserting "no row written" on
// failure paths without paying for a full List.
func (r *MemoryAdminAuditRepo) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}
