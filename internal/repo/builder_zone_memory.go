package repo

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryBuilderZoneRepo is a map-backed BuilderZoneRepo for tests and
// :memory: runs. Concurrent-safe.
type MemoryBuilderZoneRepo struct {
	mu     sync.RWMutex
	grants map[builderZoneKey]BuilderZone
}

type builderZoneKey struct {
	characterID int64
	zoneID      int64
}

func NewMemoryBuilderZoneRepo() *MemoryBuilderZoneRepo {
	return &MemoryBuilderZoneRepo{grants: map[builderZoneKey]BuilderZone{}}
}

func (r *MemoryBuilderZoneRepo) Grant(_ context.Context, characterID, zoneID, grantedBy int64, grantedAt time.Time) error {
	if grantedAt.IsZero() {
		grantedAt = time.Now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grants[builderZoneKey{characterID, zoneID}] = BuilderZone{
		CharacterID: characterID,
		ZoneID:      zoneID,
		GrantedBy:   grantedBy,
		GrantedAt:   grantedAt,
	}
	return nil
}

func (r *MemoryBuilderZoneRepo) Revoke(_ context.Context, characterID, zoneID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := builderZoneKey{characterID, zoneID}
	if _, ok := r.grants[k]; !ok {
		return ErrBuilderZoneNotFound
	}
	delete(r.grants, k)
	return nil
}

func (r *MemoryBuilderZoneRepo) Has(_ context.Context, characterID, zoneID int64) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.grants[builderZoneKey{characterID, zoneID}]
	return ok, nil
}

func (r *MemoryBuilderZoneRepo) ListForCharacter(_ context.Context, characterID int64) ([]BuilderZone, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []BuilderZone
	for k, v := range r.grants {
		if k.characterID == characterID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ZoneID < out[j].ZoneID })
	return out, nil
}

func (r *MemoryBuilderZoneRepo) ListForZone(_ context.Context, zoneID int64) ([]BuilderZone, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []BuilderZone
	for k, v := range r.grants {
		if k.zoneID == zoneID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CharacterID < out[j].CharacterID })
	return out, nil
}
