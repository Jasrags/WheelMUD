package repo

import (
	"context"
	"sort"
	"sync"
)

// MemoryZoneRepo is an in-memory ZoneRepo for tests. Concurrent-safe.
// RWMutex (rather than Mutex) so the upcoming §9 areaReset bucket —
// which will read every zone's reset_interval_s + reset_mode on
// every tick — doesn't contend with concurrent admin `zones list` /
// `zones show` queries. Reads vastly outnumber writes (one Create
// per zone per boot vs. continuous List/GetByID).
type MemoryZoneRepo struct {
	mu        sync.RWMutex
	zones     []Zone
	lastReset map[int64]int64
	maxID     int64
}

func NewMemoryZoneRepo() *MemoryZoneRepo {
	return &MemoryZoneRepo{lastReset: make(map[int64]int64)}
}

// Insert adds a zone directly without uniqueness checks. Test fixtures
// use this; production code (the YAML loader) goes through Create.
func (r *MemoryZoneRepo) Insert(z Zone) Zone {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.insertLocked(z)
}

func (r *MemoryZoneRepo) Create(_ context.Context, z Zone) (Zone, error) {
	if z.ResetMode == "" {
		z.ResetMode = ZoneResetEmpty
	}
	if !z.ResetMode.IsValid() {
		return Zone{}, ErrInvalidResetMode
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.zones {
		if existing.ExternalID == z.ExternalID {
			return Zone{}, ErrDuplicateZone
		}
	}
	return r.insertLocked(z), nil
}

func (r *MemoryZoneRepo) insertLocked(z Zone) Zone {
	if z.ID == 0 {
		r.maxID++
		z.ID = r.maxID
	} else if z.ID > r.maxID {
		r.maxID = z.ID
	}
	r.zones = append(r.zones, z)
	return z
}

func (r *MemoryZoneRepo) GetByID(_ context.Context, id int64) (Zone, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, z := range r.zones {
		if z.ID == id {
			return z, nil
		}
	}
	return Zone{}, ErrZoneNotFound
}

func (r *MemoryZoneRepo) GetByExternalID(_ context.Context, externalID string) (Zone, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, z := range r.zones {
		if z.ExternalID == externalID {
			return z, nil
		}
	}
	return Zone{}, ErrZoneNotFound
}

func (r *MemoryZoneRepo) List(_ context.Context) ([]Zone, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Zone, len(r.zones))
	copy(out, r.zones)
	sort.Slice(out, func(i, j int) bool { return out[i].ExternalID < out[j].ExternalID })
	return out, nil
}

func (r *MemoryZoneRepo) LastResetTs(_ context.Context, zoneID int64) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, z := range r.zones {
		if z.ID == zoneID {
			return r.lastReset[zoneID], nil
		}
	}
	return 0, ErrZoneNotFound
}

func (r *MemoryZoneRepo) RecordLastResetTs(_ context.Context, zoneID int64, ts int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, z := range r.zones {
		if z.ID == zoneID {
			r.lastReset[zoneID] = ts
			return nil
		}
	}
	return ErrZoneNotFound
}
