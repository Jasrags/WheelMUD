// Package repo holds the persistence interfaces and concrete
// implementations for game-state aggregates.
//
// Login mode and tests depend on the AccountRepo interface; concrete
// SQLite-backed implementations live in this package as well, with an
// in-memory fake exposed for tests via Memory*.
package repo

import (
	"context"
	"errors"
	"time"
)

// Account is the persisted user record. Username is stored case-preserving
// for display; lookups go through UsernameLower (the unique index column).
// PasswordHash is opaque to this package — the auth layer owns hashing
// and verification.
type Account struct {
	ID               int64
	Username         string
	UsernameLower    string
	PasswordHash     string
	CreatedAt        time.Time
	LastLoginAt      *time.Time
	FailedLoginCount int
	LockedUntil      *time.Time
}

// IsLockedAt reports whether the account is in a lockout window at t.
// Login mode calls this *before* invoking auth.Verify so we don't burn
// bcrypt CPU on a known-locked account.
func (a Account) IsLockedAt(t time.Time) bool {
	return a.LockedUntil != nil && t.Before(*a.LockedUntil)
}

// AccountRepo is the persistence boundary login mode talks to. The
// in-memory fake (NewMemoryAccountRepo) and the SQLite implementation
// (NewSQLiteAccountRepo) both satisfy it.
type AccountRepo interface {
	// Create inserts a new account. Returns ErrDuplicateUsername if
	// UsernameLower already exists.
	Create(ctx context.Context, a Account) (Account, error)
	// FindByUsername resolves an account by case-insensitive username.
	// Returns ErrAccountNotFound when missing.
	FindByUsername(ctx context.Context, username string) (Account, error)
	// RecordLoginSuccess updates last_login_at and resets the failed counter.
	RecordLoginSuccess(ctx context.Context, id int64, when time.Time) error
	// RecordLoginFailure bumps failed_login_count and optionally sets a
	// lockout window. Pass a zero `lockedUntil` to leave the existing
	// lockout untouched.
	RecordLoginFailure(ctx context.Context, id int64, lockedUntil time.Time) error
}

var (
	ErrAccountNotFound   = errors.New("repo: account not found")
	ErrDuplicateUsername = errors.New("repo: username already taken")
)
