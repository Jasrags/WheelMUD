package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runBankerRepoTests(t *testing.T, name string, newRepo func(t *testing.T) BankerRepo) {
	t.Helper()

	t.Run(name+"/create_and_get_round_trip", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		seed := Banker{MobTemplateID: 42, OpenHour: 8, CloseHour: 18}
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
		if fetched.OpenHour != 8 || fetched.CloseHour != 18 {
			t.Fatalf("round-trip mismatch: %+v", fetched)
		}
	})

	t.Run(name+"/create_rejects_zero_template", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Banker{}); err == nil {
			t.Fatal("expected error on zero MobTemplateID")
		}
	})

	t.Run(name+"/create_rejects_duplicate", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Banker{MobTemplateID: 5}); err != nil {
			t.Fatalf("first create: %v", err)
		}
		if _, err := r.Create(ctx, Banker{MobTemplateID: 5}); !errors.Is(err, ErrDuplicateExternalID) {
			t.Fatalf("dup err = %v, want ErrDuplicateExternalID", err)
		}
	})

	t.Run(name+"/get_missing_returns_not_found", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.GetByMobTemplateID(ctx, 999); !errors.Is(err, ErrBankerNotFound) {
			t.Fatalf("err = %v, want ErrBankerNotFound", err)
		}
	})

	t.Run(name+"/list_bankers_sorted_by_id", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Banker{MobTemplateID: 10}); err != nil {
			t.Fatalf("create A: %v", err)
		}
		if _, err := r.Create(ctx, Banker{MobTemplateID: 11}); err != nil {
			t.Fatalf("create B: %v", err)
		}
		got, err := r.ListBankers(ctx)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 2 || got[0].ID >= got[1].ID {
			t.Fatalf("list order wrong: %+v", got)
		}
	})
}

func TestMemoryBankerRepo(t *testing.T) {
	runBankerRepoTests(t, "memory", func(t *testing.T) BankerRepo {
		return NewMemoryBankerRepo()
	})
}

func TestSQLiteBankerRepo(t *testing.T) {
	runBankerRepoTests(t, "sqlite", func(t *testing.T) BankerRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteBankerRepo(conn)
	})
}

func TestBanker_IsOpenAt(t *testing.T) {
	always := Banker{}
	for h := 0; h < 24; h++ {
		if !always.IsOpenAt(h) {
			t.Fatalf("always-open closed at %d", h)
		}
	}
	day := Banker{OpenHour: 8, CloseHour: 18}
	cases := map[int]bool{0: false, 7: false, 8: true, 17: true, 18: false, 23: false}
	for h, want := range cases {
		if got := day.IsOpenAt(h); got != want {
			t.Fatalf("day-banker hour=%d got %v, want %v", h, got, want)
		}
	}
	wrap := Banker{OpenHour: 22, CloseHour: 4}
	wcases := map[int]bool{21: false, 22: true, 23: true, 0: true, 3: true, 4: false, 12: false}
	for h, want := range wcases {
		if got := wrap.IsOpenAt(h); got != want {
			t.Fatalf("wrap hour=%d got %v, want %v", h, got, want)
		}
	}
}
