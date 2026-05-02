package repo

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// MemoryCharacterRepo is an in-memory CharacterRepo for tests.
// Concurrent-safe.
type MemoryCharacterRepo struct {
	mu      sync.Mutex
	nextID  int64
	byLower map[string]*Character // name_lower -> character (single source of truth)
}

func NewMemoryCharacterRepo() *MemoryCharacterRepo {
	return &MemoryCharacterRepo{byLower: make(map[string]*Character)}
}

func (r *MemoryCharacterRepo) Create(_ context.Context, c Character) (Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c.NameLower = strings.ToLower(c.Name)
	if _, exists := r.byLower[c.NameLower]; exists {
		return Character{}, ErrDuplicateCharacterName
	}
	r.nextID++
	c.ID = r.nextID
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	if c.CurrentRoomID == 0 {
		c.CurrentRoomID = StarterRoomID
	}
	if c.BoundRoomID == 0 {
		c.BoundRoomID = StarterRoomID
	}
	stored := c
	r.byLower[c.NameLower] = &stored
	return stored, nil
}

func (r *MemoryCharacterRepo) FindByName(_ context.Context, name string) (Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byLower[strings.ToLower(name)]
	if !ok {
		return Character{}, ErrCharacterNotFound
	}
	return *c, nil
}

func (r *MemoryCharacterRepo) ListByAccount(_ context.Context, accountID int64) ([]Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Character
	for _, c := range r.byLower {
		if c.AccountID == accountID {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		// last_played_at DESC nulls last, then name_lower asc
		ai, aj := out[i].LastPlayedAt, out[j].LastPlayedAt
		switch {
		case ai != nil && aj == nil:
			return true
		case ai == nil && aj != nil:
			return false
		case ai != nil && aj != nil && !ai.Equal(*aj):
			return ai.After(*aj)
		}
		return out[i].NameLower < out[j].NameLower
	})
	return out, nil
}

func (r *MemoryCharacterRepo) RecordPlay(_ context.Context, id int64, when time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			t := when
			c.LastPlayedAt = &t
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordRoom(_ context.Context, id, roomID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			c.CurrentRoomID = roomID
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordChannelSettings(_ context.Context, id int64, settings map[string]bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			if len(settings) == 0 {
				c.ChannelSettings = nil
				return nil
			}
			cp := make(map[string]bool, len(settings))
			for k, v := range settings {
				cp[k] = v
			}
			c.ChannelSettings = cp
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordCore(_ context.Context, id int64, hp, subdual int32, cond creature.Condition, pos creature.PositionFlags) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			c.Core.HPCurrent = hp
			c.Core.Subdual = subdual
			c.Core.Conditions = cond
			c.Core.Position = pos
			return nil
		}
	}
	return ErrCharacterNotFound
}
