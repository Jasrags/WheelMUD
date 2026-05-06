package repo

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryAccountLoginRepo is a slice-backed AccountLoginRepo for tests
// and non-persistent runs. Concurrent-safe.
type MemoryAccountLoginRepo struct {
	mu      sync.RWMutex
	nextID  int64
	entries []AccountLoginEntry
}

func NewMemoryAccountLoginRepo() *MemoryAccountLoginRepo {
	return &MemoryAccountLoginRepo{}
}

func (r *MemoryAccountLoginRepo) Record(_ context.Context, e AccountLoginEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	e.ID = r.nextID
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	r.entries = append(r.entries, e)
	return nil
}

func (r *MemoryAccountLoginRepo) ListRecentByAccount(_ context.Context, accountID int64, limit int) ([]AccountLoginEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]AccountLoginEntry, 0, len(r.entries))
	for _, e := range r.entries {
		if e.AccountID != accountID {
			continue
		}
		out = append(out, e)
	}
	// Newest first; ties broken by ID (later inserts win).
	sort.Slice(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return out[i].ID > out[j].ID
	})
	if limit <= 0 {
		limit = DefaultAccountLoginListLimit
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Len exposes the entry count for tests asserting "no row written" on
// failure paths without paying for a full List.
func (r *MemoryAccountLoginRepo) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}
