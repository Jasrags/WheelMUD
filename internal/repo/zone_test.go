package repo

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runZoneRepoTests(t *testing.T, name string, newRepo func(t *testing.T) ZoneRepo) {
	t.Helper()

	sample := func(externalID string) Zone {
		return Zone{
			ExternalID:     externalID,
			Name:           externalID + " (display)",
			Builder:        "jrags",
			MinLevel:       1,
			MaxLevel:       5,
			ResetIntervalS: 900,
			ResetMode:      ZoneResetEmpty,
			Climate:        "temperate",
			Ambient: []string{
				"Wind sighs through the eaves.",
				"A bird calls in the distance.",
			},
		}
	}

	t.Run(name+"/create_then_get_by_id", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		z, err := repo.Create(ctx, sample("emonds_field"))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if z.ID == 0 {
			t.Fatal("Create returned zone with id=0")
		}
		got, err := repo.GetByID(ctx, z.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ExternalID != "emonds_field" {
			t.Errorf("ExternalID = %q, want emonds_field", got.ExternalID)
		}
		if !reflect.DeepEqual(got.Ambient, z.Ambient) {
			t.Errorf("Ambient = %v, want %v", got.Ambient, z.Ambient)
		}
		if got.ResetMode != ZoneResetEmpty {
			t.Errorf("ResetMode = %q, want %q", got.ResetMode, ZoneResetEmpty)
		}
		if got.MinLevel != 1 || got.MaxLevel != 5 {
			t.Errorf("level range = %d-%d, want 1-5", got.MinLevel, got.MaxLevel)
		}
	})

	t.Run(name+"/get_by_external_id", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		if _, err := repo.Create(ctx, sample("watch_hill")); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.GetByExternalID(ctx, "watch_hill")
		if err != nil {
			t.Fatalf("GetByExternalID: %v", err)
		}
		if got.Name != "watch_hill (display)" {
			t.Errorf("Name = %q", got.Name)
		}
	})

	t.Run(name+"/get_missing_returns_not_found", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		if _, err := repo.GetByID(ctx, 99999); !errors.Is(err, ErrZoneNotFound) {
			t.Errorf("GetByID err = %v, want ErrZoneNotFound", err)
		}
		if _, err := repo.GetByExternalID(ctx, "nope"); !errors.Is(err, ErrZoneNotFound) {
			t.Errorf("GetByExternalID err = %v, want ErrZoneNotFound", err)
		}
	})

	t.Run(name+"/duplicate_external_id_rejected", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		if _, err := repo.Create(ctx, sample("dup")); err != nil {
			t.Fatalf("first Create: %v", err)
		}
		_, err := repo.Create(ctx, sample("dup"))
		if !errors.Is(err, ErrDuplicateZone) {
			t.Errorf("err = %v, want ErrDuplicateZone", err)
		}
	})

	t.Run(name+"/invalid_reset_mode_rejected", func(t *testing.T) {
		repo := newRepo(t)
		z := sample("bad")
		z.ResetMode = "blah"
		_, err := repo.Create(context.Background(), z)
		if err == nil {
			t.Fatal("expected error on invalid reset mode, got nil")
		}
	})

	t.Run(name+"/empty_reset_mode_defaults_to_empty", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		z := sample("defaults")
		z.ResetMode = ""
		created, err := repo.Create(ctx, z)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.GetByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ResetMode != ZoneResetEmpty {
			t.Errorf("ResetMode = %q, want %q", got.ResetMode, ZoneResetEmpty)
		}
	})

	t.Run(name+"/list_sorted_by_external_id", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		// Insert in non-sorted order to prove List sorts.
		for _, ext := range []string{"watch_hill", "emonds_field", "deven_ride", "taren_ferry"} {
			if _, err := repo.Create(ctx, sample(ext)); err != nil {
				t.Fatalf("Create %s: %v", ext, err)
			}
		}
		got, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 4 {
			t.Fatalf("List len = %d, want 4", len(got))
		}
		want := []string{"deven_ride", "emonds_field", "taren_ferry", "watch_hill"}
		for i, z := range got {
			if z.ExternalID != want[i] {
				t.Errorf("got[%d].ExternalID = %q, want %q", i, z.ExternalID, want[i])
			}
		}
	})

	t.Run(name+"/empty_ambient_round_trip", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		z := sample("quiet")
		z.Ambient = nil
		created, err := repo.Create(ctx, z)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.GetByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if len(got.Ambient) != 0 {
			t.Errorf("Ambient = %v, want empty", got.Ambient)
		}
	})

	t.Run(name+"/reset_modes_all_accepted", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		for _, m := range []ZoneResetMode{ZoneResetAlways, ZoneResetEmpty, ZoneResetNever} {
			z := sample("mode_" + string(m))
			z.ResetMode = m
			created, err := repo.Create(ctx, z)
			if err != nil {
				t.Fatalf("Create %s: %v", m, err)
			}
			got, err := repo.GetByID(ctx, created.ID)
			if err != nil {
				t.Fatalf("GetByID %s: %v", m, err)
			}
			if got.ResetMode != m {
				t.Errorf("mode = %q, want %q", got.ResetMode, m)
			}
		}
	})
}

func TestMemoryZoneRepo(t *testing.T) {
	runZoneRepoTests(t, "memory", func(t *testing.T) ZoneRepo {
		return NewMemoryZoneRepo()
	})
}

func TestSQLiteZoneRepo(t *testing.T) {
	runZoneRepoTests(t, "sqlite", func(t *testing.T) ZoneRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteZoneRepo(conn)
	})
}
