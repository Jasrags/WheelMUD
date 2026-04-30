package repo

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryItemRepo is an in-memory ItemRepo for tests. Concurrent-safe.
type MemoryItemRepo struct {
	mu    sync.Mutex
	items []Item
	byExt map[string]struct{}
	maxID int64
}

func NewMemoryItemRepo() *MemoryItemRepo {
	return &MemoryItemRepo{byExt: make(map[string]struct{})}
}

// Insert adds an item directly without ExternalID validation. Test
// fixtures use this; production code (the YAML loader) goes through
// Create.
func (r *MemoryItemRepo) Insert(i Item) Item {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.insertLocked(i)
}

func (r *MemoryItemRepo) Create(_ context.Context, i Item) (Item, error) {
	if i.ExternalID == "" {
		return Item{}, ErrInvalidExternalID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byExt[i.ExternalID]; dup {
		return Item{}, ErrDuplicateExternalID
	}
	return r.insertLocked(i), nil
}

func (r *MemoryItemRepo) insertLocked(i Item) Item {
	if i.ID == 0 {
		r.maxID++
		i.ID = r.maxID
	} else if i.ID > r.maxID {
		r.maxID = i.ID
	}
	if i.NameLower == "" {
		i.NameLower = strings.ToLower(i.Name)
	}
	if i.CreatedAt.IsZero() {
		i.CreatedAt = time.Now().UTC()
	}
	r.items = append(r.items, i)
	if i.ExternalID != "" {
		r.byExt[i.ExternalID] = struct{}{}
	}
	return i
}

func (r *MemoryItemRepo) ListInRoom(_ context.Context, roomID int64) ([]Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Item
	for _, i := range r.items {
		if i.RoomID == roomID {
			out = append(out, i)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NameLower < out[j].NameLower })
	return out, nil
}
