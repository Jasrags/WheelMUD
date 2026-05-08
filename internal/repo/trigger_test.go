package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runTriggerRepoTests(t *testing.T, name string, newRepo func(t *testing.T) TriggerRepo) {
	t.Helper()

	t.Run(name+"/create_and_round_trip", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		seed := Trigger{
			OwnerKind: TriggerOwnerMobTemplate,
			OwnerID:   42,
			Event:     TriggerEventOnEnter,
			Action:    "say",
			Payload:   `{"text":"hello"}`,
			Priority:  1,
		}
		got, err := r.Create(ctx, seed)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if got.ID == 0 {
			t.Fatal("ID not assigned")
		}
		list, err := r.ListByOwner(ctx, TriggerOwnerMobTemplate, 42)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("len = %d, want 1", len(list))
		}
		if list[0].Action != "say" || list[0].Payload != `{"text":"hello"}` {
			t.Fatalf("round-trip mismatch: %+v", list[0])
		}
	})

	t.Run(name+"/payload_default", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		got, err := r.Create(ctx, Trigger{
			OwnerKind: TriggerOwnerRoom, OwnerID: 1,
			Event: TriggerEventOnTick, Action: "noop",
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if got.Payload != "{}" {
			t.Fatalf("default payload = %q, want %q", got.Payload, "{}")
		}
	})

	t.Run(name+"/reject_bad_owner_kind", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		_, err := r.Create(ctx, Trigger{
			OwnerKind: "item", OwnerID: 1,
			Event: TriggerEventOnEnter, Action: "noop",
		})
		if !errors.Is(err, ErrInvalidTrigger) {
			t.Fatalf("err = %v, want ErrInvalidTrigger", err)
		}
	})

	t.Run(name+"/reject_bad_event", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		_, err := r.Create(ctx, Trigger{
			OwnerKind: TriggerOwnerRoom, OwnerID: 1,
			Event: "on_lol", Action: "noop",
		})
		if !errors.Is(err, ErrInvalidTrigger) {
			t.Fatalf("err = %v, want ErrInvalidTrigger", err)
		}
	})

	t.Run(name+"/reject_zero_owner", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		_, err := r.Create(ctx, Trigger{
			OwnerKind: TriggerOwnerRoom, OwnerID: 0,
			Event: TriggerEventOnEnter, Action: "noop",
		})
		if !errors.Is(err, ErrInvalidTrigger) {
			t.Fatalf("err = %v, want ErrInvalidTrigger", err)
		}
	})

	t.Run(name+"/reject_empty_action", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		_, err := r.Create(ctx, Trigger{
			OwnerKind: TriggerOwnerRoom, OwnerID: 1,
			Event: TriggerEventOnEnter, Action: "  ",
		})
		if !errors.Is(err, ErrInvalidTrigger) {
			t.Fatalf("err = %v, want ErrInvalidTrigger", err)
		}
	})

	t.Run(name+"/list_by_owner_priority_desc", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		mk := func(prio int) {
			if _, err := r.Create(ctx, Trigger{
				OwnerKind: TriggerOwnerRoom, OwnerID: 7,
				Event: TriggerEventOnEnter, Action: "noop", Priority: prio,
			}); err != nil {
				t.Fatalf("create: %v", err)
			}
		}
		mk(1)
		mk(5)
		mk(3)
		got, err := r.ListByOwner(ctx, TriggerOwnerRoom, 7)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		if got[0].Priority != 5 || got[1].Priority != 3 || got[2].Priority != 1 {
			t.Fatalf("priority order: %+v", got)
		}
	})

	t.Run(name+"/list_all_sorted_by_id", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Trigger{
			OwnerKind: TriggerOwnerRoom, OwnerID: 1,
			Event: TriggerEventOnEnter, Action: "noop",
		}); err != nil {
			t.Fatalf("create A: %v", err)
		}
		if _, err := r.Create(ctx, Trigger{
			OwnerKind: TriggerOwnerMobTemplate, OwnerID: 2,
			Event: TriggerEventOnDeath, Action: "noop",
		}); err != nil {
			t.Fatalf("create B: %v", err)
		}
		all, err := r.ListAll(ctx)
		if err != nil {
			t.Fatalf("list all: %v", err)
		}
		if len(all) != 2 || all[0].ID >= all[1].ID {
			t.Fatalf("ListAll order: %+v", all)
		}
	})

	t.Run(name+"/delete_by_owner", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Trigger{
			OwnerKind: TriggerOwnerRoom, OwnerID: 11,
			Event: TriggerEventOnEnter, Action: "noop",
		}); err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := r.Create(ctx, Trigger{
			OwnerKind: TriggerOwnerRoom, OwnerID: 22,
			Event: TriggerEventOnEnter, Action: "noop",
		}); err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := r.DeleteByOwner(ctx, TriggerOwnerRoom, 11); err != nil {
			t.Fatalf("delete: %v", err)
		}
		left, err := r.ListByOwner(ctx, TriggerOwnerRoom, 11)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(left) != 0 {
			t.Fatalf("expected empty after delete, got %+v", left)
		}
		other, err := r.ListByOwner(ctx, TriggerOwnerRoom, 22)
		if err != nil {
			t.Fatalf("list other: %v", err)
		}
		if len(other) != 1 {
			t.Fatalf("delete leaked: %+v", other)
		}
	})
}

func TestMemoryTriggerRepo(t *testing.T) {
	runTriggerRepoTests(t, "memory", func(t *testing.T) TriggerRepo {
		return NewMemoryTriggerRepo()
	})
}

func TestSQLiteTriggerRepo(t *testing.T) {
	runTriggerRepoTests(t, "sqlite", func(t *testing.T) TriggerRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteTriggerRepo(conn)
	})
}
