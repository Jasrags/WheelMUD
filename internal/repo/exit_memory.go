package repo

import (
	"context"
	"sort"
	"sync"
)

// MemoryExitRepo is an in-memory ExitRepo for tests. Concurrent-safe.
type MemoryExitRepo struct {
	mu    sync.Mutex
	exits []Exit
	maxID int64
}

func NewMemoryExitRepo() *MemoryExitRepo { return &MemoryExitRepo{} }

// Insert adds an exit directly. Test fixtures use this to seed the
// connectivity graph. If e.ID is zero an id is auto-assigned.
func (r *MemoryExitRepo) Insert(e Exit) Exit {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.ID == 0 {
		r.maxID++
		e.ID = r.maxID
	} else if e.ID > r.maxID {
		r.maxID = e.ID
	}
	r.exits = append(r.exits, e)
	return e
}

func (r *MemoryExitRepo) ListFrom(_ context.Context, fromRoomID int64) ([]Exit, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Exit
	for _, e := range r.exits {
		if e.FromRoomID == fromRoomID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Direction < out[j].Direction })
	return out, nil
}

func (r *MemoryExitRepo) FindByDirection(_ context.Context, fromRoomID int64, direction string) (Exit, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.exits {
		if e.FromRoomID == fromRoomID && e.Direction == direction {
			return e, nil
		}
	}
	return Exit{}, ErrExitNotFound
}
