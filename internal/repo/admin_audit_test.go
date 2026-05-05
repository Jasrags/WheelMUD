package repo

import (
	"context"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runAdminAuditRepoTests(t *testing.T, name string, newRepo func(t *testing.T) AdminAuditRepo) {
	t.Helper()

	t.Run(name+"/record_assigns_id_and_ts", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if err := r.Record(ctx, AdminAuditEntry{
			ActorCharacterID: 7,
			ActorName:        "Moiraine",
			Verb:             "spawn",
			Target:           "tr.inn_lantern",
			Args:             "item tr.inn_lantern 2",
		}); err != nil {
			t.Fatalf("record: %v", err)
		}
		got, err := r.List(ctx, AdminAuditFilter{})
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
		if e.TS.IsZero() {
			t.Fatal("TS not stamped")
		}
		if e.Verb != "spawn" || e.Target != "tr.inn_lantern" || e.ActorName != "Moiraine" {
			t.Fatalf("unexpected entry: %+v", e)
		}
	})

	t.Run(name+"/list_orders_newest_first", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		base := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
		for i, v := range []string{"goto", "spawn", "shutdown"} {
			if err := r.Record(ctx, AdminAuditEntry{
				TS:               base.Add(time.Duration(i) * time.Second),
				ActorCharacterID: 1,
				Verb:             v,
			}); err != nil {
				t.Fatalf("record %s: %v", v, err)
			}
		}
		got, err := r.List(ctx, AdminAuditFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("got %d, want 3", len(got))
		}
		want := []string{"shutdown", "spawn", "goto"}
		for i, e := range got {
			if e.Verb != want[i] {
				t.Fatalf("position %d verb = %q, want %q", i, e.Verb, want[i])
			}
		}
	})

	t.Run(name+"/list_filters", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		base := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
		seed := []AdminAuditEntry{
			{TS: base, ActorCharacterID: 1, Verb: "spawn"},
			{TS: base.Add(1 * time.Second), ActorCharacterID: 2, Verb: "goto"},
			{TS: base.Add(2 * time.Second), ActorCharacterID: 1, Verb: "shutdown"},
			{TS: base.Add(3 * time.Second), ActorCharacterID: 2, Verb: "spawn"},
		}
		for _, e := range seed {
			if err := r.Record(ctx, e); err != nil {
				t.Fatalf("record: %v", err)
			}
		}

		// Verb filter.
		got, err := r.List(ctx, AdminAuditFilter{Verbs: []string{"spawn"}})
		if err != nil || len(got) != 2 {
			t.Fatalf("verbs filter: got %d (err=%v), want 2", len(got), err)
		}
		// Actor filter.
		got, err = r.List(ctx, AdminAuditFilter{Actor: 1})
		if err != nil || len(got) != 2 {
			t.Fatalf("actor filter: got %d (err=%v), want 2", len(got), err)
		}
		// Since filter (>= base+2s should yield 2 newest).
		got, err = r.List(ctx, AdminAuditFilter{Since: base.Add(2 * time.Second)})
		if err != nil || len(got) != 2 {
			t.Fatalf("since filter: got %d (err=%v), want 2", len(got), err)
		}
		// Combined.
		got, err = r.List(ctx, AdminAuditFilter{Actor: 2, Verbs: []string{"spawn"}})
		if err != nil || len(got) != 1 || got[0].Verb != "spawn" || got[0].ActorCharacterID != 2 {
			t.Fatalf("combined filter: %+v (err=%v)", got, err)
		}
	})

	t.Run(name+"/list_limit", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		for i := 0; i < 5; i++ {
			if err := r.Record(ctx, AdminAuditEntry{
				ActorCharacterID: 1,
				Verb:             "spawn",
				TS:               time.Date(2026, 5, 4, 0, 0, i, 0, time.UTC),
			}); err != nil {
				t.Fatalf("record %d: %v", i, err)
			}
		}
		got, err := r.List(ctx, AdminAuditFilter{Limit: 3})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("limit not honored: got %d", len(got))
		}
	})
}

func TestMemoryAdminAuditRepo(t *testing.T) {
	runAdminAuditRepoTests(t, "memory", func(t *testing.T) AdminAuditRepo {
		return NewMemoryAdminAuditRepo()
	})
}

func TestSQLiteAdminAuditRepo(t *testing.T) {
	runAdminAuditRepoTests(t, "sqlite", func(t *testing.T) AdminAuditRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteAdminAuditRepo(conn)
	})
}
