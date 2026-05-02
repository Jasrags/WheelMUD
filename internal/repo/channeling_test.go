package repo

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/db"
)

func runChannelingRepoTests(t *testing.T, name string, newRepo func(t *testing.T) ChannelingRepo) {
	t.Helper()

	sample := func() creature.Channeling {
		return creature.Channeling{
			GenderSource:     creature.SourceSaidar,
			ChannelerType:    creature.ChannelerInitiate,
			Affinities:       creature.PowerSet(1<<creature.PowerFire | 1<<creature.PowerSpirit),
			Talents:          []creature.TalentID{1, 4, 7},
			WeavesKnown:      []creature.WeaveRef{{ID: 100, Rarity: creature.RarityCommon}, {ID: 250, Rarity: creature.RarityRare}},
			Slots:            [10]creature.SlotPool{{Cur: 4, Max: 4}, {Cur: 3, Max: 3}, {Cur: 2, Max: 2}},
			Embraced:         true,
			EmbracedSince:    time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
			Madness:          0, // female channeler
			Stilled:          false,
			BondedWarderID:   42,
			HeldAngrealID:    7,
			AesSedaiOaths:    creature.OathTruth | creature.OathNoWeapon,
			Ageless:          true,
		}
	}

	t.Run(name+"/upsert_get_roundtrip", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		want := sample()
		if err := repo.Upsert(ctx, OwnerKindCharacter, 100, want); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
		got, err := repo.GetByOwner(ctx, OwnerKindCharacter, 100)
		if err != nil {
			t.Fatalf("GetByOwner: %v", err)
		}
		if got.GenderSource != want.GenderSource ||
			got.ChannelerType != want.ChannelerType ||
			got.Affinities != want.Affinities {
			t.Fatalf("scalars mismatch: got %+v", got)
		}
		if !reflect.DeepEqual(got.Talents, want.Talents) {
			t.Fatalf("Talents mismatch: got %+v", got.Talents)
		}
		if !reflect.DeepEqual(got.WeavesKnown, want.WeavesKnown) {
			t.Fatalf("WeavesKnown mismatch: got %+v", got.WeavesKnown)
		}
		if got.Slots != want.Slots {
			t.Fatalf("Slots mismatch: got %+v", got.Slots)
		}
		if got.Embraced != want.Embraced || got.Ageless != want.Ageless ||
			got.AesSedaiOaths != want.AesSedaiOaths {
			t.Fatalf("flags mismatch: got embraced=%v ageless=%v oaths=%x",
				got.Embraced, got.Ageless, got.AesSedaiOaths)
		}
		if !got.EmbracedSince.Equal(want.EmbracedSince) {
			t.Fatalf("EmbracedSince = %v, want %v", got.EmbracedSince, want.EmbracedSince)
		}
		if got.BondedWarderID != want.BondedWarderID || got.HeldAngrealID != want.HeldAngrealID {
			t.Fatalf("bond/angreal mismatch: %+v", got)
		}
	})

	t.Run(name+"/upsert_replaces_existing", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		first := sample()
		if err := repo.Upsert(ctx, OwnerKindCharacter, 1, first); err != nil {
			t.Fatalf("first Upsert: %v", err)
		}
		second := first
		second.Madness = 5
		second.Embraced = false
		if err := repo.Upsert(ctx, OwnerKindCharacter, 1, second); err != nil {
			t.Fatalf("second Upsert: %v", err)
		}
		got, err := repo.GetByOwner(ctx, OwnerKindCharacter, 1)
		if err != nil {
			t.Fatalf("GetByOwner: %v", err)
		}
		if got.Madness != 5 || got.Embraced {
			t.Fatalf("upsert did not replace: %+v", got)
		}
	})

	t.Run(name+"/different_owner_kinds_isolated", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		c1 := sample()
		c1.GenderSource = creature.SourceSaidin
		c2 := sample()
		c2.GenderSource = creature.SourceSaidar
		if err := repo.Upsert(ctx, OwnerKindCharacter, 1, c1); err != nil {
			t.Fatalf("upsert char: %v", err)
		}
		if err := repo.Upsert(ctx, OwnerKindMobInstance, 1, c2); err != nil {
			t.Fatalf("upsert mob: %v", err)
		}
		got1, _ := repo.GetByOwner(ctx, OwnerKindCharacter, 1)
		got2, _ := repo.GetByOwner(ctx, OwnerKindMobInstance, 1)
		if got1.GenderSource != creature.SourceSaidin || got2.GenderSource != creature.SourceSaidar {
			t.Fatalf("kinds collided: char=%v mob=%v", got1.GenderSource, got2.GenderSource)
		}
	})

	t.Run(name+"/get_unknown_returns_not_found", func(t *testing.T) {
		repo := newRepo(t)
		_, err := repo.GetByOwner(context.Background(), OwnerKindCharacter, 9999)
		if !errors.Is(err, ErrChannelingNotFound) {
			t.Fatalf("err = %v, want ErrChannelingNotFound", err)
		}
	})

	t.Run(name+"/delete", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		if err := repo.Upsert(ctx, OwnerKindCharacter, 5, sample()); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
		if err := repo.DeleteByOwner(ctx, OwnerKindCharacter, 5); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		_, err := repo.GetByOwner(ctx, OwnerKindCharacter, 5)
		if !errors.Is(err, ErrChannelingNotFound) {
			t.Fatalf("err = %v, want ErrChannelingNotFound", err)
		}
		if err := repo.DeleteByOwner(ctx, OwnerKindCharacter, 5); !errors.Is(err, ErrChannelingNotFound) {
			t.Fatalf("Delete twice err = %v, want ErrChannelingNotFound", err)
		}
	})

	t.Run(name+"/invalid_owner_kind", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		err := repo.Upsert(ctx, OwnerKind(99), 1, sample())
		if !errors.Is(err, ErrInvalidOwnerKind) {
			t.Fatalf("err = %v, want ErrInvalidOwnerKind", err)
		}
	})
}

func TestMemoryChannelingRepo(t *testing.T) {
	runChannelingRepoTests(t, "memory", func(t *testing.T) ChannelingRepo {
		return NewMemoryChannelingRepo()
	})
}

func TestSQLiteChannelingRepo(t *testing.T) {
	runChannelingRepoTests(t, "sqlite", func(t *testing.T) ChannelingRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteChannelingRepo(conn)
	})
}
