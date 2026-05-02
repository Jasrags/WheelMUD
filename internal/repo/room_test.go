package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runRoomRepoTests(t *testing.T, name string, newRepo func(t *testing.T) RoomRepo) {
	t.Helper()
	t.Run(name+"/create_and_find", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		got, err := r.Create(ctx, Room{ExternalID: "plaza.fountain", Name: "Town Plaza", LongDesc: "Cobblestones."})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if got.ID == 0 {
			t.Fatal("ID not assigned")
		}

		byID, err := r.FindByID(ctx, got.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if byID.ExternalID != "plaza.fountain" || byID.Name != "Town Plaza" {
			t.Fatalf("FindByID round-trip: %+v", byID)
		}

		byExt, err := r.FindByExternalID(ctx, "plaza.fountain")
		if err != nil {
			t.Fatalf("FindByExternalID: %v", err)
		}
		if byExt.ID != got.ID {
			t.Fatalf("FindByExternalID = %d, want %d", byExt.ID, got.ID)
		}
	})

	t.Run(name+"/create_with_pinned_id", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		got, err := r.Create(ctx, Room{ID: StarterRoomID, ExternalID: "plaza.fountain", Name: "Plaza"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if got.ID != StarterRoomID {
			t.Fatalf("got ID %d, want %d", got.ID, StarterRoomID)
		}
	})

	t.Run(name+"/create_rejects_empty_external_id", func(t *testing.T) {
		r := newRepo(t)
		_, err := r.Create(context.Background(), Room{Name: "Anywhere"})
		if !errors.Is(err, ErrInvalidExternalID) {
			t.Fatalf("err = %v, want ErrInvalidExternalID", err)
		}
	})

	t.Run(name+"/create_duplicate_external_id", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Room{ExternalID: "x", Name: "first"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		_, err := r.Create(ctx, Room{ExternalID: "x", Name: "second"})
		if !errors.Is(err, ErrDuplicateExternalID) {
			t.Fatalf("err = %v, want ErrDuplicateExternalID", err)
		}
	})

	t.Run(name+"/flags_sector_light_coords_roundtrip", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		input := Room{
			ExternalID: "deep.cavern",
			Name:       "Deep Cavern",
			LongDesc:   "Damp stone walls.",
			Flags: RoomFlags{
				Indoors: true, NoTeleport: true, Dark: true, Silent: true, Peaceful: true,
			},
			Sector:     SectorUnderground,
			LightLevel: 0,
			CoordX:     -3, CoordY: 7, CoordZ: -1,
		}
		created, err := r.Create(ctx, input)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := r.FindByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if got.Sector != SectorUnderground {
			t.Errorf("Sector = %q, want underground", got.Sector)
		}
		if got.LightLevel != 0 {
			t.Errorf("LightLevel = %d, want 0", got.LightLevel)
		}
		if !got.Flags.Dark || !got.Flags.Silent || !got.Flags.Indoors ||
			!got.Flags.Peaceful || !got.Flags.NoTeleport || got.Flags.NoPVP {
			t.Errorf("Flags = %+v, want indoors+noteleport+dark+silent+peaceful only", got.Flags)
		}
		if got.CoordX != -3 || got.CoordY != 7 || got.CoordZ != -1 {
			t.Errorf("Coords = (%d,%d,%d), want (-3,7,-1)", got.CoordX, got.CoordY, got.CoordZ)
		}
	})

	t.Run(name+"/sector_default_when_unspecified", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		created, err := r.Create(ctx, Room{ExternalID: "default.room", Name: "Default", LightLevel: DefaultLightLevel})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := r.FindByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if got.Sector != SectorCity {
			t.Errorf("default Sector = %q, want city", got.Sector)
		}
	})

	t.Run(name+"/light_zero_preserved_without_dark", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		// A "dim" room: light_level=0 but Dark=false. Caller's choice;
		// the repo must not silently promote 0 to DefaultLightLevel.
		created, err := r.Create(ctx, Room{
			ExternalID: "dim.room", Name: "Dim", LightLevel: 0,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := r.FindByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if got.LightLevel != 0 {
			t.Errorf("LightLevel = %d, want 0 (no auto-promote)", got.LightLevel)
		}
		if got.Flags.Dark {
			t.Errorf("Flags.Dark = true, want false")
		}
	})

	t.Run(name+"/extra_descs_roundtrip_lowercased", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		created, err := r.Create(ctx, Room{
			ExternalID: "fountain.plaza", Name: "Plaza",
			ExtraDescs: map[string]string{
				"Fountain": "A marble fountain spills clear water.",
				"  Statue ": "A weathered hero salutes the sky.",
			},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := r.FindByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if got.ExtraDescs["fountain"] == "" {
			t.Errorf("missing 'fountain' key after lowercase normalization: %+v", got.ExtraDescs)
		}
		if got.ExtraDescs["statue"] == "" {
			t.Errorf("missing 'statue' key after trim+lowercase: %+v", got.ExtraDescs)
		}
		if _, mixedCase := got.ExtraDescs["Fountain"]; mixedCase {
			t.Errorf("uppercase key leaked through normalization")
		}
	})

	t.Run(name+"/extra_descs_empty_is_nil", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		created, err := r.Create(ctx, Room{ExternalID: "blank", Name: "Blank"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := r.FindByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if got.ExtraDescs != nil {
			t.Errorf("ExtraDescs = %+v, want nil for empty map", got.ExtraDescs)
		}
	})

	t.Run(name+"/find_missing", func(t *testing.T) {
		r := newRepo(t)
		_, err := r.FindByID(context.Background(), 99999)
		if !errors.Is(err, ErrRoomNotFound) {
			t.Fatalf("FindByID err = %v", err)
		}
		_, err = r.FindByExternalID(context.Background(), "nope")
		if !errors.Is(err, ErrRoomNotFound) {
			t.Fatalf("FindByExternalID err = %v", err)
		}
	})
}

func TestMemoryRoomRepo(t *testing.T) {
	runRoomRepoTests(t, "memory", func(t *testing.T) RoomRepo {
		return NewMemoryRoomRepo()
	})
}

func TestSQLiteRoomRepo(t *testing.T) {
	runRoomRepoTests(t, "sqlite", func(t *testing.T) RoomRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteRoomRepo(conn)
	})
}
