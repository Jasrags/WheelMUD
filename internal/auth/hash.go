// Package auth owns password hashing and verification for the project.
//
// We use bcrypt at the library default cost (10) — this is the
// recommended setting for x/crypto/bcrypt and matches what the rest of
// the Go ecosystem ships. Re-tune via SetCost if a future audit shows
// it's worth it; do not lower below the bcrypt minimum.
package auth

import (
	"errors"
	"fmt"
	"sync/atomic"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// MinPasswordLen is the lower bound enforced by Hash. Tune up here when
// password policy lands; do not duplicate the check in callers.
const MinPasswordLen = 8

// MaxPasswordLen guards against bcrypt's own 72-byte input limit. Inputs
// past 72 bytes are silently truncated by bcrypt itself, which would
// mask differences in long passwords; reject early with a clear error
// instead.
const MaxPasswordLen = 72

var (
	ErrPasswordTooShort = errors.New("auth: password too short")
	ErrPasswordTooLong  = errors.New("auth: password exceeds 72 bytes")
)

// cost is the bcrypt cost parameter. Stored in an atomic so tests can
// drop it to bcrypt.MinCost for speed without racing the production
// default. Real callers should not touch this.
var cost atomic.Int32

func init() { cost.Store(int32(bcrypt.DefaultCost)) }

// SetCost overrides the bcrypt cost. Intended for tests only.
// Returns the previous value so tests can restore it via t.Cleanup.
func SetCost(c int) int {
	prev := cost.Load()
	cost.Store(int32(c))
	return int(prev)
}

// Hash returns a bcrypt hash of password. Enforces MinPasswordLen and
// MaxPasswordLen. The returned string is what the caller stores in
// `accounts.password_hash`.
func Hash(password string) (string, error) {
	if utf8.RuneCountInString(password) < MinPasswordLen {
		return "", ErrPasswordTooShort
	}
	if len(password) > MaxPasswordLen {
		return "", ErrPasswordTooLong
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), int(cost.Load()))
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(h), nil
}

// Verify reports whether password matches hash. It returns false for
// any mismatch (including malformed hashes); never returns an error to
// the caller, so the timing of the failure path is the same regardless
// of why verification failed.
//
// The bcrypt comparison itself is constant-time over its input; the
// length-check shortcut below is intentional — bcrypt rejects oversized
// inputs anyway, so refusing them here just saves a hash computation
// without changing the security boundary.
func Verify(hash, password string) bool {
	if len(password) == 0 || len(password) > MaxPasswordLen {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
