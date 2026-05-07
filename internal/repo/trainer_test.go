package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runTrainerRepoTests(t *testing.T, name string, newRepo func(t *testing.T) TrainerRepo) {
	t.Helper()

	t.Run(name+"/create_and_get_round_trip", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		seed := Trainer{MobTemplateID: 42, ClassID: "armsman"}
		got, err := r.Create(ctx, seed)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if got.ID == 0 {
			t.Fatal("ID not assigned")
		}
		fetched, err := r.GetByMobTemplateID(ctx, 42)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if fetched.ClassID != "armsman" {
			t.Fatalf("round-trip mismatch: %+v", fetched)
		}
	})

	t.Run(name+"/create_rejects_zero_template", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Trainer{ClassID: "armsman"}); err == nil {
			t.Fatal("expected error on zero MobTemplateID")
		}
	})

	t.Run(name+"/create_rejects_empty_class", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Trainer{MobTemplateID: 1}); err == nil {
			t.Fatal("expected error on empty ClassID")
		}
	})

	t.Run(name+"/create_rejects_duplicate", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Trainer{MobTemplateID: 5, ClassID: "wilder"}); err != nil {
			t.Fatalf("first create: %v", err)
		}
		if _, err := r.Create(ctx, Trainer{MobTemplateID: 5, ClassID: "armsman"}); !errors.Is(err, ErrDuplicateExternalID) {
			t.Fatalf("dup err = %v, want ErrDuplicateExternalID", err)
		}
	})

	t.Run(name+"/get_missing_returns_not_found", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.GetByMobTemplateID(ctx, 999); !errors.Is(err, ErrTrainerNotFound) {
			t.Fatalf("err = %v, want ErrTrainerNotFound", err)
		}
	})

	t.Run(name+"/list_trainers_sorted_by_id", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Trainer{MobTemplateID: 10, ClassID: "armsman"}); err != nil {
			t.Fatalf("create A: %v", err)
		}
		if _, err := r.Create(ctx, Trainer{MobTemplateID: 11, ClassID: "wilder"}); err != nil {
			t.Fatalf("create B: %v", err)
		}
		got, err := r.ListTrainers(ctx)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 2 || got[0].ID >= got[1].ID {
			t.Fatalf("list order wrong: %+v", got)
		}
	})
}

func TestMemoryTrainerRepo(t *testing.T) {
	runTrainerRepoTests(t, "memory", func(t *testing.T) TrainerRepo {
		return NewMemoryTrainerRepo()
	})
}

func TestSQLiteTrainerRepo(t *testing.T) {
	runTrainerRepoTests(t, "sqlite", func(t *testing.T) TrainerRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteTrainerRepo(conn)
	})
}
