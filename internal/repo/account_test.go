package repo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/db"
)

// runAccountRepoTests is a contract test that both implementations must satisfy.
// Each implementation passes a factory; the test exercises the same behaviors
// against both, so the in-memory fake stays a faithful stand-in for SQLite.
func runAccountRepoTests(t *testing.T, name string, newRepo func(t *testing.T) AccountRepo) {
	t.Helper()
	t.Run(name+"/create+find", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		got, err := r.Create(ctx, Account{Username: "Frodo", PasswordHash: "h"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if got.ID == 0 {
			t.Fatal("ID not set after Create")
		}
		if got.UsernameLower != "frodo" {
			t.Fatalf("UsernameLower = %q, want %q", got.UsernameLower, "frodo")
		}
		// Case-insensitive lookup.
		found, err := r.FindByUsername(ctx, "FRODO")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if found.ID != got.ID || found.PasswordHash != "h" {
			t.Fatalf("find returned %+v", found)
		}
	})

	t.Run(name+"/duplicate", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Account{Username: "sam", PasswordHash: "h"}); err != nil {
			t.Fatalf("first create: %v", err)
		}
		_, err := r.Create(ctx, Account{Username: "SAM", PasswordHash: "h2"})
		if !errors.Is(err, ErrDuplicateUsername) {
			t.Fatalf("err = %v, want ErrDuplicateUsername", err)
		}
	})

	t.Run(name+"/missing", func(t *testing.T) {
		r := newRepo(t)
		_, err := r.FindByUsername(context.Background(), "ghost")
		if !errors.Is(err, ErrAccountNotFound) {
			t.Fatalf("err = %v, want ErrAccountNotFound", err)
		}
	})

	t.Run(name+"/login_success_resets_counters", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		a, _ := r.Create(ctx, Account{Username: "pippin", PasswordHash: "h"})
		when := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
		future := when.Add(time.Hour)
		if err := r.RecordLoginFailure(ctx, a.ID, future); err != nil {
			t.Fatalf("failure: %v", err)
		}
		if err := r.RecordLoginSuccess(ctx, a.ID, when); err != nil {
			t.Fatalf("success: %v", err)
		}
		got, _ := r.FindByUsername(ctx, "pippin")
		if got.FailedLoginCount != 0 {
			t.Fatalf("failed count not reset: %d", got.FailedLoginCount)
		}
		if got.LockedUntil != nil {
			t.Fatalf("locked_until not cleared: %v", got.LockedUntil)
		}
		if got.LastLoginAt == nil || !got.LastLoginAt.Equal(when) {
			t.Fatalf("last_login = %v, want %v", got.LastLoginAt, when)
		}
	})

	t.Run(name+"/login_failure_increments_and_locks", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		a, _ := r.Create(ctx, Account{Username: "merry", PasswordHash: "h"})

		// Three failures with no lockout, then one with lockout.
		for i := 0; i < 3; i++ {
			if err := r.RecordLoginFailure(ctx, a.ID, time.Time{}); err != nil {
				t.Fatalf("failure %d: %v", i, err)
			}
		}
		lock := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
		if err := r.RecordLoginFailure(ctx, a.ID, lock); err != nil {
			t.Fatalf("locking failure: %v", err)
		}

		got, _ := r.FindByUsername(ctx, "merry")
		if got.FailedLoginCount != 4 {
			t.Fatalf("failed count = %d, want 4", got.FailedLoginCount)
		}
		if got.LockedUntil == nil || !got.LockedUntil.Equal(lock) {
			t.Fatalf("locked_until = %v, want %v", got.LockedUntil, lock)
		}
	})

	t.Run(name+"/update_password_hash", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		a, _ := r.Create(ctx, Account{Username: "rosie", PasswordHash: "old"})
		// Fail a login first so we can verify other columns survive.
		if err := r.RecordLoginFailure(ctx, a.ID, time.Time{}); err != nil {
			t.Fatalf("failure: %v", err)
		}
		if err := r.UpdatePasswordHash(ctx, a.ID, "new"); err != nil {
			t.Fatalf("update: %v", err)
		}
		got, _ := r.FindByUsername(ctx, "rosie")
		if got.PasswordHash != "new" {
			t.Fatalf("hash = %q, want %q", got.PasswordHash, "new")
		}
		if got.FailedLoginCount != 1 {
			t.Fatalf("failed count mutated: %d, want 1", got.FailedLoginCount)
		}
		if got.Username != "rosie" || got.UsernameLower != "rosie" {
			t.Fatalf("username changed: %+v", got)
		}
	})

	t.Run(name+"/update_password_hash_missing", func(t *testing.T) {
		r := newRepo(t)
		err := r.UpdatePasswordHash(context.Background(), 9999, "h")
		if !errors.Is(err, ErrAccountNotFound) {
			t.Fatalf("err = %v, want ErrAccountNotFound", err)
		}
	})

	t.Run(name+"/settings_default_zero_value", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		a, err := r.Create(ctx, Account{Username: "newcomer", PasswordHash: "h"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		got, err := r.FindByUsername(ctx, "newcomer")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if got.Settings != (AccountSettings{}) {
			t.Fatalf("default settings = %+v, want zero value", got.Settings)
		}
		_ = a
	})

	t.Run(name+"/update_settings_round_trip", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		a, _ := r.Create(ctx, Account{Username: "tweaker", PasswordHash: "h"})
		want := AccountSettings{
			ColorOverride: "256",
			PromptDefault: "<%h/%H hp> ",
			WidthOverride: 100,
			Locale:        "America/New_York",
			MOTDAlways:    true,
		}
		if err := r.UpdateSettings(ctx, a.ID, want); err != nil {
			t.Fatalf("update settings: %v", err)
		}
		got, err := r.FindByUsername(ctx, "tweaker")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if got.Settings != want {
			t.Fatalf("settings = %+v, want %+v", got.Settings, want)
		}
		// Verify other columns weren't trashed.
		if got.PasswordHash != "h" {
			t.Fatalf("hash mutated: %q", got.PasswordHash)
		}
	})

	t.Run(name+"/update_settings_missing", func(t *testing.T) {
		err := newRepo(t).UpdateSettings(context.Background(), 9999, AccountSettings{})
		if !errors.Is(err, ErrAccountNotFound) {
			t.Fatalf("err = %v, want ErrAccountNotFound", err)
		}
	})

	t.Run(name+"/find_by_id", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		a, _ := r.Create(ctx, Account{Username: "byid", PasswordHash: "h"})
		got, err := r.FindByID(ctx, a.ID)
		if err != nil {
			t.Fatalf("find by id: %v", err)
		}
		if got.ID != a.ID || got.Username != "byid" {
			t.Fatalf("find by id = %+v", got)
		}
		if _, err := r.FindByID(ctx, 9999); !errors.Is(err, ErrAccountNotFound) {
			t.Fatalf("missing find by id = %v, want ErrAccountNotFound", err)
		}
	})
}

func TestMemoryAccountRepo(t *testing.T) {
	runAccountRepoTests(t, "memory", func(t *testing.T) AccountRepo {
		return NewMemoryAccountRepo()
	})
}

func TestSQLiteAccountRepo(t *testing.T) {
	runAccountRepoTests(t, "sqlite", func(t *testing.T) AccountRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteAccountRepo(conn)
	})
}
