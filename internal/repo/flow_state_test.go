package repo

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runFlowStateRepoTests(t *testing.T, name string, newRepo func(t *testing.T) FlowStateRepo) {
	t.Helper()

	t.Run(name+"/save_load_roundtrip", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		now := time.Date(2026, 5, 18, 14, 0, 0, 0, time.UTC)
		fs := FlowState{
			AccountID:   42,
			FlowID:      "wizdemo",
			CurrentStep: "ask_color",
			Values:      map[string]string{"name": "Moiraine", "color": "blue"},
			StartedAt:   now,
			UpdatedAt:   now.Add(30 * time.Second),
		}
		if err := r.Save(ctx, fs); err != nil {
			t.Fatalf("save: %v", err)
		}
		got, err := r.Load(ctx, 42, "wizdemo")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if got.CurrentStep != "ask_color" {
			t.Errorf("current_step = %q, want ask_color", got.CurrentStep)
		}
		if got.Values["name"] != "Moiraine" || got.Values["color"] != "blue" {
			t.Errorf("values lost in roundtrip: %+v", got.Values)
		}
		if !got.StartedAt.Equal(now) {
			t.Errorf("started_at = %v, want %v", got.StartedAt, now)
		}
		if !got.UpdatedAt.Equal(now.Add(30 * time.Second)) {
			t.Errorf("updated_at = %v, want %v", got.UpdatedAt, now.Add(30*time.Second))
		}
	})

	t.Run(name+"/load_missing_returns_sentinel", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		_, err := r.Load(ctx, 1, "nope")
		if !errors.Is(err, ErrFlowStateNotFound) {
			t.Fatalf("Load missing err = %v, want ErrFlowStateNotFound", err)
		}
	})

	t.Run(name+"/save_overwrites_existing", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		base := time.Date(2026, 5, 18, 14, 0, 0, 0, time.UTC)
		if err := r.Save(ctx, FlowState{AccountID: 1, FlowID: "f", CurrentStep: "a", StartedAt: base, UpdatedAt: base}); err != nil {
			t.Fatalf("first save: %v", err)
		}
		if err := r.Save(ctx, FlowState{AccountID: 1, FlowID: "f", CurrentStep: "b", StartedAt: base, UpdatedAt: base.Add(time.Minute)}); err != nil {
			t.Fatalf("second save: %v", err)
		}
		got, err := r.Load(ctx, 1, "f")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if got.CurrentStep != "b" {
			t.Errorf("second save not applied: current_step=%q", got.CurrentStep)
		}
	})

	t.Run(name+"/delete_is_idempotent", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if err := r.Delete(ctx, 1, "never_existed"); err != nil {
			t.Fatalf("delete missing: %v", err)
		}
		base := time.Date(2026, 5, 18, 14, 0, 0, 0, time.UTC)
		if err := r.Save(ctx, FlowState{AccountID: 1, FlowID: "f", CurrentStep: "a", StartedAt: base, UpdatedAt: base}); err != nil {
			t.Fatalf("save: %v", err)
		}
		if err := r.Delete(ctx, 1, "f"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Load(ctx, 1, "f"); !errors.Is(err, ErrFlowStateNotFound) {
			t.Fatalf("after delete Load err=%v, want ErrFlowStateNotFound", err)
		}
		if err := r.Delete(ctx, 1, "f"); err != nil {
			t.Fatalf("re-delete: %v", err)
		}
	})

	t.Run(name+"/list_by_account_orders_newest_first", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		base := time.Date(2026, 5, 18, 14, 0, 0, 0, time.UTC)
		for i, id := range []string{"alpha", "beta", "gamma"} {
			if err := r.Save(ctx, FlowState{
				AccountID:   7,
				FlowID:      id,
				CurrentStep: "s",
				StartedAt:   base,
				UpdatedAt:   base.Add(time.Duration(i) * time.Minute),
			}); err != nil {
				t.Fatalf("save %s: %v", id, err)
			}
		}
		got, err := r.ListByAccount(ctx, 7)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("got %d rows, want 3", len(got))
		}
		want := []string{"gamma", "beta", "alpha"}
		for i, fs := range got {
			if fs.FlowID != want[i] {
				t.Fatalf("pos %d flow_id = %q, want %q", i, fs.FlowID, want[i])
			}
		}
	})

	t.Run(name+"/list_by_account_isolates_accounts", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		base := time.Date(2026, 5, 18, 14, 0, 0, 0, time.UTC)
		if err := r.Save(ctx, FlowState{AccountID: 1, FlowID: "f", CurrentStep: "s", StartedAt: base, UpdatedAt: base}); err != nil {
			t.Fatal(err)
		}
		if err := r.Save(ctx, FlowState{AccountID: 2, FlowID: "f", CurrentStep: "s", StartedAt: base, UpdatedAt: base}); err != nil {
			t.Fatal(err)
		}
		got1, _ := r.ListByAccount(ctx, 1)
		got2, _ := r.ListByAccount(ctx, 2)
		if len(got1) != 1 || got1[0].AccountID != 1 {
			t.Fatalf("account 1 list = %+v", got1)
		}
		if len(got2) != 1 || got2[0].AccountID != 2 {
			t.Fatalf("account 2 list = %+v", got2)
		}
	})

	t.Run(name+"/cap_evicts_oldest_on_insert", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		base := time.Date(2026, 5, 18, 14, 0, 0, 0, time.UTC)
		// Fill to cap with strictly-ascending UpdatedAt so the eviction
		// order is deterministic across impls.
		for i := 0; i < MaxFlowStatesPerAccount; i++ {
			if err := r.Save(ctx, FlowState{
				AccountID:   99,
				FlowID:      fmt.Sprintf("flow_%d", i),
				CurrentStep: "s",
				StartedAt:   base,
				UpdatedAt:   base.Add(time.Duration(i) * time.Minute),
			}); err != nil {
				t.Fatalf("seed %d: %v", i, err)
			}
		}
		// One past cap → should evict flow_0 (oldest UpdatedAt).
		if err := r.Save(ctx, FlowState{
			AccountID:   99,
			FlowID:      "overflow",
			CurrentStep: "s",
			StartedAt:   base,
			UpdatedAt:   base.Add(time.Duration(MaxFlowStatesPerAccount) * time.Minute),
		}); err != nil {
			t.Fatalf("overflow save: %v", err)
		}
		got, err := r.ListByAccount(ctx, 99)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != MaxFlowStatesPerAccount {
			t.Fatalf("post-eviction count = %d, want %d", len(got), MaxFlowStatesPerAccount)
		}
		for _, fs := range got {
			if fs.FlowID == "flow_0" {
				t.Fatal("flow_0 (oldest) should have been evicted")
			}
		}
		// Verify the new row is present and the others survived.
		if _, err := r.Load(ctx, 99, "overflow"); err != nil {
			t.Fatalf("overflow row missing: %v", err)
		}
		if _, err := r.Load(ctx, 99, "flow_0"); !errors.Is(err, ErrFlowStateNotFound) {
			t.Fatalf("flow_0 still present after eviction (err=%v)", err)
		}
	})

	t.Run(name+"/update_at_cap_does_not_evict", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		base := time.Date(2026, 5, 18, 14, 0, 0, 0, time.UTC)
		for i := 0; i < MaxFlowStatesPerAccount; i++ {
			if err := r.Save(ctx, FlowState{
				AccountID:   55,
				FlowID:      fmt.Sprintf("flow_%d", i),
				CurrentStep: "s",
				StartedAt:   base,
				UpdatedAt:   base.Add(time.Duration(i) * time.Minute),
			}); err != nil {
				t.Fatal(err)
			}
		}
		// Update an existing row at cap — must NOT evict anything.
		if err := r.Save(ctx, FlowState{
			AccountID:   55,
			FlowID:      "flow_0",
			CurrentStep: "advanced",
			StartedAt:   base,
			UpdatedAt:   base.Add(time.Hour),
		}); err != nil {
			t.Fatalf("update-at-cap save: %v", err)
		}
		got, err := r.ListByAccount(ctx, 55)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != MaxFlowStatesPerAccount {
			t.Fatalf("count after update = %d, want %d", len(got), MaxFlowStatesPerAccount)
		}
	})

	t.Run(name+"/save_defaults_zero_times", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		before := time.Now().UTC().Add(-time.Second)
		if err := r.Save(ctx, FlowState{AccountID: 1, FlowID: "f", CurrentStep: "s"}); err != nil {
			t.Fatalf("save: %v", err)
		}
		got, err := r.Load(ctx, 1, "f")
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if got.UpdatedAt.Before(before) {
			t.Fatalf("UpdatedAt not stamped: %v < %v", got.UpdatedAt, before)
		}
		if got.StartedAt.IsZero() {
			t.Fatal("StartedAt not defaulted")
		}
	})
}

func TestMemoryFlowStateRepo(t *testing.T) {
	runFlowStateRepoTests(t, "memory", func(t *testing.T) FlowStateRepo {
		return NewMemoryFlowStateRepo()
	})
}

func TestSQLiteFlowStateRepo(t *testing.T) {
	runFlowStateRepoTests(t, "sqlite", func(t *testing.T) FlowStateRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		// FK from flow_state(account_id) → accounts(id) requires the
		// row to exist. The shared suite uses synthetic account ids
		// that won't be in `accounts`; the FK is enforced via the
		// FOREIGN KEY clause but only when foreign_keys is ON, which
		// db.Open enables. Insert stub accounts up to the highest id
		// touched by the suite.
		ctx := context.Background()
		for _, id := range []int64{1, 2, 7, 42, 55, 99} {
			if _, err := conn.ExecContext(ctx,
				`INSERT INTO accounts(id, username, username_lower, password_hash, created_at)
				 VALUES (?, ?, ?, '', ?)`,
				id, fmt.Sprintf("acct%d", id), fmt.Sprintf("acct%d", id), time.Now().UTC(),
			); err != nil {
				t.Fatalf("seed account %d: %v", id, err)
			}
		}
		return NewSQLiteFlowStateRepo(conn)
	})
}
