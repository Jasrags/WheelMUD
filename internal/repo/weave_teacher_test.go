package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/db"
)

func runWeaveTeacherRepoTests(t *testing.T, name string, newRepo func(t *testing.T) WeaveTeacherRepo) {
	t.Helper()

	t.Run(name+"/create_and_get_round_trip", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		seed := WeaveTeacher{
			MobTemplateID:  42,
			MaxLevelTaught: 1,
			AffinityFilter: 1<<creature.PowerAir | 1<<creature.PowerFire,
		}
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
		if fetched.MaxLevelTaught != 1 {
			t.Fatalf("MaxLevelTaught round-trip: %+v", fetched)
		}
		if fetched.AffinityFilter != seed.AffinityFilter {
			t.Fatalf("AffinityFilter round-trip: got %d want %d",
				fetched.AffinityFilter, seed.AffinityFilter)
		}
	})

	t.Run(name+"/create_rejects_zero_template", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, WeaveTeacher{MaxLevelTaught: 0}); err == nil {
			t.Fatal("expected error on zero MobTemplateID")
		}
	})

	t.Run(name+"/create_rejects_bad_level", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, WeaveTeacher{MobTemplateID: 1, MaxLevelTaught: 10}); err == nil {
			t.Fatal("expected error on out-of-range level")
		}
		if _, err := r.Create(ctx, WeaveTeacher{MobTemplateID: 2, MaxLevelTaught: -1}); err == nil {
			t.Fatal("expected error on negative level")
		}
	})

	t.Run(name+"/create_rejects_duplicate", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, WeaveTeacher{MobTemplateID: 5}); err != nil {
			t.Fatalf("first create: %v", err)
		}
		if _, err := r.Create(ctx, WeaveTeacher{MobTemplateID: 5}); !errors.Is(err, ErrDuplicateExternalID) {
			t.Fatalf("dup err = %v, want ErrDuplicateExternalID", err)
		}
	})

	t.Run(name+"/get_missing_returns_not_found", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.GetByMobTemplateID(ctx, 999); !errors.Is(err, ErrWeaveTeacherNotFound) {
			t.Fatalf("err = %v, want ErrWeaveTeacherNotFound", err)
		}
	})

	t.Run(name+"/list_sorted_by_id", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, WeaveTeacher{MobTemplateID: 10}); err != nil {
			t.Fatalf("create A: %v", err)
		}
		if _, err := r.Create(ctx, WeaveTeacher{MobTemplateID: 11}); err != nil {
			t.Fatalf("create B: %v", err)
		}
		got, err := r.ListWeaveTeachers(ctx)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 2 || got[0].ID >= got[1].ID {
			t.Fatalf("list order wrong: %+v", got)
		}
	})

	t.Run(name+"/affinity_zero_round_trips", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, WeaveTeacher{MobTemplateID: 7, MaxLevelTaught: 0}); err != nil {
			t.Fatalf("create: %v", err)
		}
		got, err := r.GetByMobTemplateID(ctx, 7)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.AffinityFilter != 0 {
			t.Fatalf("zero AffinityFilter not preserved: %d", got.AffinityFilter)
		}
	})
}

func TestMemoryWeaveTeacherRepo(t *testing.T) {
	runWeaveTeacherRepoTests(t, "memory", func(t *testing.T) WeaveTeacherRepo {
		return NewMemoryWeaveTeacherRepo()
	})
}

func TestSQLiteWeaveTeacherRepo(t *testing.T) {
	runWeaveTeacherRepoTests(t, "sqlite", func(t *testing.T) WeaveTeacherRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteWeaveTeacherRepo(conn)
	})
}
