package repo

import (
	"context"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runAccountLoginRepoTests(t *testing.T, name string, newRepo func(t *testing.T) AccountLoginRepo) {
	t.Helper()

	t.Run(name+"/record_assigns_id_and_at", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if err := r.Record(ctx, AccountLoginEntry{
			AccountID:     7,
			RemoteAddress: "203.0.113.4",
			Outcome:       LoginOutcomeSuccess,
		}); err != nil {
			t.Fatalf("record: %v", err)
		}
		got, err := r.ListRecentByAccount(ctx, 7, 0)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d entries, want 1", len(got))
		}
		e := got[0]
		if e.ID == 0 {
			t.Fatal("ID not assigned")
		}
		if e.At.IsZero() {
			t.Fatal("At not stamped")
		}
		if e.Outcome != LoginOutcomeSuccess || e.RemoteAddress != "203.0.113.4" {
			t.Fatalf("unexpected entry: %+v", e)
		}
	})

	t.Run(name+"/orders_newest_first", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		base := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
		for i, outcome := range []string{LoginOutcomeFailure, LoginOutcomeFailure, LoginOutcomeSuccess} {
			if err := r.Record(ctx, AccountLoginEntry{
				AccountID: 1,
				At:        base.Add(time.Duration(i) * time.Second),
				Outcome:   outcome,
			}); err != nil {
				t.Fatalf("record %d: %v", i, err)
			}
		}
		got, err := r.ListRecentByAccount(ctx, 1, 0)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("got %d, want 3", len(got))
		}
		want := []string{LoginOutcomeSuccess, LoginOutcomeFailure, LoginOutcomeFailure}
		for i, e := range got {
			if e.Outcome != want[i] {
				t.Fatalf("position %d outcome = %q, want %q", i, e.Outcome, want[i])
			}
		}
	})

	t.Run(name+"/per_account_isolation", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		seed := []AccountLoginEntry{
			{AccountID: 1, Outcome: LoginOutcomeSuccess},
			{AccountID: 2, Outcome: LoginOutcomeFailure},
			{AccountID: 1, Outcome: LoginOutcomeFailure},
		}
		for _, e := range seed {
			if err := r.Record(ctx, e); err != nil {
				t.Fatalf("record: %v", err)
			}
		}
		got1, err := r.ListRecentByAccount(ctx, 1, 0)
		if err != nil || len(got1) != 2 {
			t.Fatalf("account 1: got %d (err=%v), want 2", len(got1), err)
		}
		got2, err := r.ListRecentByAccount(ctx, 2, 0)
		if err != nil || len(got2) != 1 {
			t.Fatalf("account 2: got %d (err=%v), want 1", len(got2), err)
		}
		got3, err := r.ListRecentByAccount(ctx, 999, 0)
		if err != nil || len(got3) != 0 {
			t.Fatalf("missing account: got %d (err=%v), want 0", len(got3), err)
		}
	})

	t.Run(name+"/limit_honored", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		base := time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC)
		for i := 0; i < 5; i++ {
			if err := r.Record(ctx, AccountLoginEntry{
				AccountID: 1,
				At:        base.Add(time.Duration(i) * time.Second),
				Outcome:   LoginOutcomeSuccess,
			}); err != nil {
				t.Fatalf("record %d: %v", i, err)
			}
		}
		got, err := r.ListRecentByAccount(ctx, 1, 3)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("limit not honored: got %d", len(got))
		}
	})
}

func TestMemoryAccountLoginRepo(t *testing.T) {
	runAccountLoginRepoTests(t, "memory", func(t *testing.T) AccountLoginRepo {
		return NewMemoryAccountLoginRepo()
	})
}

func TestSQLiteAccountLoginRepo(t *testing.T) {
	runAccountLoginRepoTests(t, "sqlite", func(t *testing.T) AccountLoginRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteAccountLoginRepo(conn)
	})
}
