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

// Insert adds an exit directly without uniqueness checks. Test fixtures
// use this; production code (the YAML loader) goes through Create.
func (r *MemoryExitRepo) Insert(e Exit) Exit {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.insertLocked(e)
}

func (r *MemoryExitRepo) Create(_ context.Context, e Exit) (Exit, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.exits {
		if existing.FromRoomID == e.FromRoomID && existing.Direction == e.Direction {
			return Exit{}, ErrDuplicateExit
		}
	}
	return r.insertLocked(e), nil
}

func (r *MemoryExitRepo) insertLocked(e Exit) Exit {
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

func (r *MemoryExitRepo) UpdateFlags(_ context.Context, exitID int64, closed, locked bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.exits {
		if r.exits[i].ID == exitID {
			r.exits[i].Flags.Closed = closed
			r.exits[i].Flags.Locked = locked
			return nil
		}
	}
	return ErrExitNotFound
}

// Update overwrites the authoring subset of an exit row in place,
// preserving identity (id, from_room_id, direction), runtime door
// state (Closed, Locked), and authored_* snapshots. Phase G #34
// redit slice 2.
func (r *MemoryExitRepo) Update(_ context.Context, in Exit) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.exits {
		e := &r.exits[i]
		if e.ID != in.ID {
			continue
		}
		e.ToRoomID = in.ToRoomID
		e.Description = in.Description
		e.KeyExternalID = in.KeyExternalID
		e.LockDifficulty = in.LockDifficulty
		e.Flags.Pickable = in.Flags.Pickable
		e.Flags.Hidden = in.Flags.Hidden
		e.Flags.NoPass = in.Flags.NoPass
		return nil
	}
	return ErrExitNotFound
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

// RestoreAuthored mirrors the SQLite implementation: snap each
// matching exit's runtime closed/locked back to its authored value
// when divergent. Returns the count of rows it actually changed.
func (r *MemoryExitRepo) RestoreAuthored(_ context.Context, fromRoomIDs []int64) (int, error) {
	if len(fromRoomIDs) == 0 {
		return 0, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	wanted := make(map[int64]struct{}, len(fromRoomIDs))
	for _, id := range fromRoomIDs {
		wanted[id] = struct{}{}
	}
	n := 0
	for i := range r.exits {
		e := &r.exits[i]
		if _, ok := wanted[e.FromRoomID]; !ok {
			continue
		}
		if e.Flags.Closed == e.Flags.AuthoredClosed && e.Flags.Locked == e.Flags.AuthoredLocked {
			continue
		}
		e.Flags.Closed = e.Flags.AuthoredClosed
		e.Flags.Locked = e.Flags.AuthoredLocked
		n++
	}
	return n, nil
}
