package repo

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryCharacterAuditRepo is a slice-backed CharacterAuditRepo for
// tests and non-persistent runs. Concurrent-safe.
type MemoryCharacterAuditRepo struct {
	mu      sync.RWMutex
	nextID  int64
	entries []CharacterAuditEntry
}

func NewMemoryCharacterAuditRepo() *MemoryCharacterAuditRepo {
	return &MemoryCharacterAuditRepo{}
}

func (r *MemoryCharacterAuditRepo) Record(_ context.Context, e CharacterAuditEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	e.ID = r.nextID
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	if len(e.Raw) > CharacterAuditRawCap {
		e.Raw = e.Raw[:CharacterAuditRawCap]
	}
	r.entries = append(r.entries, e)
	return nil
}

func (r *MemoryCharacterAuditRepo) List(_ context.Context, f CharacterAuditFilter) ([]CharacterAuditEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	verbSet := map[string]struct{}{}
	for _, v := range f.Verbs {
		verbSet[v] = struct{}{}
	}

	out := make([]CharacterAuditEntry, 0, len(r.entries))
	for _, e := range r.entries {
		if !f.Since.IsZero() && e.TS.Before(f.Since) {
			continue
		}
		if f.Character != 0 && e.CharacterID != f.Character {
			continue
		}
		if len(verbSet) > 0 {
			if _, ok := verbSet[e.Verb]; !ok {
				continue
			}
		}
		out = append(out, e)
	}

	sort.Slice(out, func(i, j int) bool {
		if !out[i].TS.Equal(out[j].TS) {
			return out[i].TS.After(out[j].TS)
		}
		return out[i].ID > out[j].ID
	})

	limit := f.Limit
	if limit <= 0 {
		limit = DefaultCharacterAuditListLimit
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Len exposes the entry count for tests asserting "no row written" on
// failure paths without paying for a full List.
func (r *MemoryCharacterAuditRepo) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}
