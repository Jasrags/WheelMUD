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
	maxID int64
}

func NewMemoryItemRepo() *MemoryItemRepo { return &MemoryItemRepo{} }

// Insert adds an item directly. Test fixtures use this to populate
// rooms. If i.ID is zero an id is auto-assigned.
func (r *MemoryItemRepo) Insert(i Item) Item {
	r.mu.Lock()
	defer r.mu.Unlock()
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
