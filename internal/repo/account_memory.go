package repo

import (
	"context"
	"strings"
	"sync"
	"time"
)

// MemoryAccountRepo is an in-memory AccountRepo for tests. Concurrent-safe.
type MemoryAccountRepo struct {
	mu      sync.Mutex
	nextID  int64
	byLower map[string]*Account // username_lower -> account (single source of truth)
}

func NewMemoryAccountRepo() *MemoryAccountRepo {
	return &MemoryAccountRepo{
		byLower: make(map[string]*Account),
	}
}

func (r *MemoryAccountRepo) Create(_ context.Context, a Account) (Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a.UsernameLower = strings.ToLower(a.Username)
	if _, exists := r.byLower[a.UsernameLower]; exists {
		return Account{}, ErrDuplicateUsername
	}
	r.nextID++
	a.ID = r.nextID
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	stored := a
	r.byLower[a.UsernameLower] = &stored
	return stored, nil
}

func (r *MemoryAccountRepo) FindByUsername(_ context.Context, username string) (Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byLower[strings.ToLower(username)]
	if !ok {
		return Account{}, ErrAccountNotFound
	}
	return *a, nil
}

func (r *MemoryAccountRepo) RecordLoginSuccess(_ context.Context, id int64, when time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.findByIDLocked(id)
	if a == nil {
		return ErrAccountNotFound
	}
	t := when
	a.LastLoginAt = &t
	a.FailedLoginCount = 0
	a.LockedUntil = nil
	return nil
}

func (r *MemoryAccountRepo) RecordLoginFailure(_ context.Context, id int64, lockedUntil time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.findByIDLocked(id)
	if a == nil {
		return ErrAccountNotFound
	}
	a.FailedLoginCount++
	if !lockedUntil.IsZero() {
		t := lockedUntil
		a.LockedUntil = &t
	}
	return nil
}

func (r *MemoryAccountRepo) UpdatePasswordHash(_ context.Context, id int64, newHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.findByIDLocked(id)
	if a == nil {
		return ErrAccountNotFound
	}
	a.PasswordHash = newHash
	return nil
}

func (r *MemoryAccountRepo) findByIDLocked(id int64) *Account {
	for _, a := range r.byLower {
		if a.ID == id {
			return a
		}
	}
	return nil
}
