package repo

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// MemoryChannelRepo is an in-memory ChannelRepo for tests. The catalog
// is small and read-mostly; List sorts on every call rather than
// maintaining a sorted slice.
type MemoryChannelRepo struct {
	mu     sync.Mutex
	nextID int64
	byName map[string]Channel
}

func NewMemoryChannelRepo(seed ...Channel) *MemoryChannelRepo {
	r := &MemoryChannelRepo{byName: make(map[string]Channel)}
	for _, c := range seed {
		r.nextID++
		c.ID = r.nextID
		r.byName[strings.ToLower(c.Name)] = c
	}
	return r
}

func (r *MemoryChannelRepo) List(_ context.Context) ([]Channel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Channel, 0, len(r.byName))
	for _, c := range r.byName {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
