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

func runMobTemplateRepoTests(t *testing.T, name string, newRepo func(t *testing.T) MobTemplateRepo) {
	t.Helper()

	// sample exercises every column class — Core stat block,
	// JSON slices, the Shadowspawn fields, the optional
	// shopkeeper subtype — so a missing column in either the
	// migration or the SQL builder shows up as a roundtrip diff.
	sample := func() creature.MobTemplate {
		return creature.MobTemplate{
			ExternalID:    "trolloc.warrior.basic",
			ChallengeCode: 'D',
			Organization:  "pack (3-6)",
			Climate:       []string{"temperate", "cold"},
			Terrain:       []string{"forest", "mountain"},
			BehaviorFlags: creature.BehavAggressive | creature.BehavAssistSameRace,
			NaturalAttacks: []creature.Attack{
				{Name: "claws", HitBonus: 4, DamageDice: "1d6+3", DamageType: creature.DamageSlash, ReachFt: 5},
			},
			SpecialAttacks: []creature.SpecialAttack{
				{Name: "rake", ScriptRef: "trolloc.rake"},
			},
			Traits:           []int32{101, 102},
			Advancement:      []creature.AdvanceRule{{HDRange: "5-8", SizeChange: creature.SizeLarge}},
			LootTableID:      42,
			GoldDice:         "2d10",
			XPValue:          1500,
			DialogueTreeID:   0,
			ShopkeeperConfig: nil,
			TriggerScripts:   []string{"trolloc.on_enter"},

			CorpseDecayTicks:      600,
			RespawnZoneResetID:    7,
			ShadowLinkMyrddraalID: 99,
			TaintImmune:           true,
			FadeOnLinkMasterTimer: 5 * time.Second,
			ShortDesc:             "a trolloc warrior",
			LongDesc:              "A bestial Shadowspawn covered in matted fur.",

			Core: creature.Core{
				Name:      "a trolloc warrior",
				Size:      creature.SizeLarge,
				Type:      creature.TypeShadowspawn,
				Gender:    creature.GenderMale,
				Alignment: creature.PostureEvil,
				Abilities: creature.Abilities{
					Str: creature.AbilityScore{Current: 16, Max: 16, Inherent: 16},
					Dex: creature.AbilityScore{Current: 12, Max: 12, Inherent: 12},
					Con: creature.AbilityScore{Current: 14, Max: 14, Inherent: 14},
					Int: creature.AbilityScore{Current: 6, Max: 6, Inherent: 6},
					Wis: creature.AbilityScore{Current: 8, Max: 8, Inherent: 8},
					Cha: creature.AbilityScore{Current: 6, Max: 6, Inherent: 6},
				},
				HPMax:   32,
				HitDice: "4d10+8",
				Defense: 14,
				Saves:   creature.Saves{Fort: 4, Ref: 2, Will: 1},
				InitMod: 1,
				BAB:     4,
				Speed:   creature.Speed{BaseFt: 30},
				ReachFt: 5, FaceFt: 5, ThreatFt: 5,
				Specials: creature.QualLowLightVision | creature.QualScent,
				DR:       []creature.DamageReduction{{Amount: 2, Bypass: ""}},
				Resists:  []creature.Resist{{Type: creature.DamageCold, Pct: 25}},
			},
		}
	}

	t.Run(name+"/create_get_roundtrip", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		want := sample()
		got, err := repo.Create(ctx, want)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if got.ID == 0 {
			t.Fatal("Create returned zero ID")
		}
		if got.Core.ID != got.ID {
			t.Fatalf("Core.ID %d != row ID %d", got.Core.ID, got.ID)
		}

		fetched, err := repo.GetByID(ctx, got.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		assertTemplateEqual(t, fetched, got)

		fetched2, err := repo.GetByExternalID(ctx, want.ExternalID)
		if err != nil {
			t.Fatalf("GetByExternalID: %v", err)
		}
		if fetched2.ID != got.ID {
			t.Fatalf("GetByExternalID returned id %d, want %d", fetched2.ID, got.ID)
		}
	})

	t.Run(name+"/create_rejects_empty_external_id", func(t *testing.T) {
		repo := newRepo(t)
		_, err := repo.Create(context.Background(), creature.MobTemplate{Core: creature.Core{Name: "x"}})
		if !errors.Is(err, ErrInvalidExternalID) {
			t.Fatalf("err = %v, want ErrInvalidExternalID", err)
		}
	})

	t.Run(name+"/create_duplicate_external_id", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		base := sample()
		if _, err := repo.Create(ctx, base); err != nil {
			t.Fatalf("first Create: %v", err)
		}
		_, err := repo.Create(ctx, base)
		if !errors.Is(err, ErrDuplicateExternalID) {
			t.Fatalf("err = %v, want ErrDuplicateExternalID", err)
		}
	})

	t.Run(name+"/get_unknown_returns_not_found", func(t *testing.T) {
		repo := newRepo(t)
		_, err := repo.GetByID(context.Background(), 999999)
		if !errors.Is(err, ErrTemplateNotFound) {
			t.Fatalf("err = %v, want ErrTemplateNotFound", err)
		}
		_, err = repo.GetByExternalID(context.Background(), "nope")
		if !errors.Is(err, ErrTemplateNotFound) {
			t.Fatalf("err = %v, want ErrTemplateNotFound", err)
		}
	})

	t.Run(name+"/shopkeeper_config_roundtrip", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		t1 := sample()
		t1.ExternalID = "shopkeeper.basic"
		t1.ShopkeeperConfig = &creature.ShopConfig{
			SellMarkup: 1.2, BuyMarkdown: 0.5, OpenHour: 8, CloseHour: 18,
			InventoryIDs: []int64{1, 2, 3},
		}
		created, err := repo.Create(ctx, t1)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := repo.GetByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ShopkeeperConfig == nil {
			t.Fatal("ShopkeeperConfig nil after roundtrip")
		}
		if got.ShopkeeperConfig.SellMarkup != 1.2 {
			t.Fatalf("SellMarkup = %v, want 1.2", got.ShopkeeperConfig.SellMarkup)
		}
		if !reflect.DeepEqual(got.ShopkeeperConfig.InventoryIDs, []int64{1, 2, 3}) {
			t.Fatalf("InventoryIDs = %v", got.ShopkeeperConfig.InventoryIDs)
		}
	})

	t.Run(name+"/dialogue_json_roundtrip", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		t.Run("nil_when_unset", func(t *testing.T) {
			t1 := sample()
			t1.ExternalID = "dialogue.absent"
			created, err := repo.Create(ctx, t1)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			got, err := repo.GetByID(ctx, created.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if len(got.DialogueJSON) != 0 {
				t.Fatalf("DialogueJSON = %q, want empty", got.DialogueJSON)
			}
		})

		t.Run("populated_roundtrip", func(t *testing.T) {
			t2 := sample()
			t2.ExternalID = "dialogue.present"
			t2.DialogueJSON = []byte(`{"root":"root","nodes":{"root":{"id":"root","prompt":"hi"}}}`)
			created, err := repo.Create(ctx, t2)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			got, err := repo.GetByID(ctx, created.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if string(got.DialogueJSON) != string(t2.DialogueJSON) {
				t.Fatalf("DialogueJSON mismatch:\n got = %s\nwant = %s", got.DialogueJSON, t2.DialogueJSON)
			}
		})
	})

	// Phase F #32a slice 1: the path column round-trips the authored
	// external_id sequence. PathRoomIDs is a non-persisted runtime
	// cache so it must NOT survive the round-trip.
	t.Run(name+"/path_roundtrip", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		t.Run("nil_when_unset", func(t *testing.T) {
			t1 := sample()
			t1.ExternalID = "path.absent"
			created, err := repo.Create(ctx, t1)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			got, err := repo.GetByID(ctx, created.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if len(got.Path) != 0 {
				t.Fatalf("Path = %v, want empty", got.Path)
			}
			if len(got.PathRoomIDs) != 0 {
				t.Fatalf("PathRoomIDs = %v, want empty (non-persisted cache)", got.PathRoomIDs)
			}
		})

		t.Run("populated_roundtrip", func(t *testing.T) {
			t2 := sample()
			t2.ExternalID = "path.present"
			t2.Path = []string{"qa.hub", "qa.shop", "qa.bank"}
			// Set a bogus PathRoomIDs that must NOT survive the round-trip.
			t2.PathRoomIDs = []int64{99, 98, 97}
			created, err := repo.Create(ctx, t2)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			got, err := repo.GetByID(ctx, created.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if !reflect.DeepEqual(got.Path, t2.Path) {
				t.Fatalf("Path mismatch:\n got = %v\nwant = %v", got.Path, t2.Path)
			}
			if len(got.PathRoomIDs) != 0 {
				t.Fatalf("PathRoomIDs leaked through persistence: %v", got.PathRoomIDs)
			}
		})
	})

	// Phase F #32a slice 2: wander_radius round-trips as INT NOT NULL
	// DEFAULT 0. Out-of-range values get clamped at the storage layer.
	t.Run(name+"/wander_radius_roundtrip", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		t.Run("zero_when_unset", func(t *testing.T) {
			t1 := sample()
			t1.ExternalID = "radius.unset"
			created, err := repo.Create(ctx, t1)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			got, err := repo.GetByID(ctx, created.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if got.WanderRadius != 0 {
				t.Fatalf("WanderRadius = %d, want 0", got.WanderRadius)
			}
		})

		t.Run("positive_roundtrip", func(t *testing.T) {
			t2 := sample()
			t2.ExternalID = "radius.three"
			t2.WanderRadius = 3
			created, err := repo.Create(ctx, t2)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			got, err := repo.GetByID(ctx, created.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if got.WanderRadius != 3 {
				t.Fatalf("WanderRadius = %d, want 3", got.WanderRadius)
			}
		})
	})
}

func assertTemplateEqual(t *testing.T, got, want creature.MobTemplate) {
	t.Helper()
	if got.ID != want.ID || got.ExternalID != want.ExternalID {
		t.Fatalf("identity mismatch: got id=%d ext=%q, want id=%d ext=%q",
			got.ID, got.ExternalID, want.ID, want.ExternalID)
	}
	if got.Core.Name != want.Core.Name ||
		got.Core.Size != want.Core.Size ||
		got.Core.Type != want.Core.Type ||
		got.Core.HPMax != want.Core.HPMax ||
		got.Core.Defense != want.Core.Defense ||
		got.Core.BAB != want.Core.BAB {
		t.Fatalf("Core mismatch: got %+v, want %+v", got.Core, want.Core)
	}
	if got.Core.Abilities != want.Core.Abilities {
		t.Fatalf("Abilities mismatch: got %+v, want %+v", got.Core.Abilities, want.Core.Abilities)
	}
	if got.Core.Saves != want.Core.Saves {
		t.Fatalf("Saves mismatch: got %+v, want %+v", got.Core.Saves, want.Core.Saves)
	}
	if !reflect.DeepEqual(got.Core.DR, want.Core.DR) {
		t.Fatalf("DR mismatch: got %+v, want %+v", got.Core.DR, want.Core.DR)
	}
	if !reflect.DeepEqual(got.Core.Resists, want.Core.Resists) {
		t.Fatalf("Resists mismatch: got %+v, want %+v", got.Core.Resists, want.Core.Resists)
	}
	if got.ChallengeCode != want.ChallengeCode {
		t.Fatalf("ChallengeCode = %q, want %q", got.ChallengeCode, want.ChallengeCode)
	}
	if got.XPValue != want.XPValue {
		t.Fatalf("XPValue = %d, want %d", got.XPValue, want.XPValue)
	}
	if got.BehaviorFlags != want.BehaviorFlags {
		t.Fatalf("BehaviorFlags = %x, want %x", got.BehaviorFlags, want.BehaviorFlags)
	}
	if got.TaintImmune != want.TaintImmune {
		t.Fatalf("TaintImmune = %v, want %v", got.TaintImmune, want.TaintImmune)
	}
	if got.ShadowLinkMyrddraalID != want.ShadowLinkMyrddraalID {
		t.Fatalf("ShadowLinkMyrddraalID = %d, want %d", got.ShadowLinkMyrddraalID, want.ShadowLinkMyrddraalID)
	}
	if got.FadeOnLinkMasterTimer != want.FadeOnLinkMasterTimer {
		t.Fatalf("FadeOnLinkMasterTimer = %v, want %v", got.FadeOnLinkMasterTimer, want.FadeOnLinkMasterTimer)
	}
	if !reflect.DeepEqual(got.NaturalAttacks, want.NaturalAttacks) {
		t.Fatalf("NaturalAttacks mismatch: got %+v, want %+v", got.NaturalAttacks, want.NaturalAttacks)
	}
	if !reflect.DeepEqual(got.SpecialAttacks, want.SpecialAttacks) {
		t.Fatalf("SpecialAttacks mismatch: got %+v, want %+v", got.SpecialAttacks, want.SpecialAttacks)
	}
	if !reflect.DeepEqual(got.Traits, want.Traits) {
		t.Fatalf("Traits mismatch: got %+v, want %+v", got.Traits, want.Traits)
	}
	if !reflect.DeepEqual(got.Climate, want.Climate) || !reflect.DeepEqual(got.Terrain, want.Terrain) {
		t.Fatalf("Climate/Terrain mismatch")
	}
	if got.ShortDesc != want.ShortDesc || got.LongDesc != want.LongDesc {
		t.Fatalf("desc mismatch: got short=%q long=%q", got.ShortDesc, got.LongDesc)
	}
}

func testListExternalIDs(t *testing.T, name string, newRepo func(t *testing.T) MobTemplateRepo) {
	t.Run(name+"/list_external_ids_sorted_and_empty", func(t *testing.T) {
		r := newRepo(t)
		ctx := context.Background()
		got, err := r.ListExternalIDs(ctx)
		if err != nil {
			t.Fatalf("ListExternalIDs empty: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected empty, got %+v", got)
		}
		for _, ext := range []string{"tr.zeta", "tr.alpha", "tr.beta"} {
			if _, err := r.Create(ctx, creature.MobTemplate{
				ExternalID:    ext,
				ChallengeCode: 'A',
				Core:          creature.Core{Name: ext, HPMax: 1, Defense: 10, Speed: creature.Speed{BaseFt: 30}, ReachFt: 5, FaceFt: 5, ThreatFt: 5, Size: creature.SizeMedium, Type: creature.TypeHumanoid},
			}); err != nil {
				t.Fatalf("Create %s: %v", ext, err)
			}
		}
		got, err = r.ListExternalIDs(ctx)
		if err != nil {
			t.Fatalf("ListExternalIDs: %v", err)
		}
		if len(got) != 3 || got[0] != "tr.alpha" || got[1] != "tr.beta" || got[2] != "tr.zeta" {
			t.Fatalf("unsorted: %+v", got)
		}
	})

	t.Run(name+"/spawn_anchor_round_trip", func(t *testing.T) {
		r := newRepo(t)
		ctx := context.Background()
		stub := func(id string) creature.MobTemplate {
			return creature.MobTemplate{
				ExternalID:    id,
				ChallengeCode: 'A',
				Core: creature.Core{
					Name: id, Size: creature.SizeMedium, Type: creature.TypeHumanoid,
					HPMax: 1, Defense: 10, Speed: creature.Speed{BaseFt: 30},
					ReachFt: 5, FaceFt: 5, ThreatFt: 5,
				},
			}
		}
		a, err := r.Create(ctx, stub("anchor.a"))
		if err != nil {
			t.Fatalf("create a: %v", err)
		}
		b, err := r.Create(ctx, stub("anchor.b"))
		if err != nil {
			t.Fatalf("create b: %v", err)
		}
		// Anchor a → zone 7 / room 100; b → zone 9 / room 200.
		if err := r.SetSpawnAnchor(ctx, a.ID, 7, 100); err != nil {
			t.Fatalf("SetSpawnAnchor a: %v", err)
		}
		if err := r.SetSpawnAnchor(ctx, b.ID, 9, 200); err != nil {
			t.Fatalf("SetSpawnAnchor b: %v", err)
		}

		got, err := r.GetByID(ctx, a.ID)
		if err != nil {
			t.Fatalf("GetByID a: %v", err)
		}
		if got.RespawnZoneResetID != 7 || got.HomeRoomID != 100 {
			t.Fatalf("a anchor = (%d,%d), want (7,100)", got.RespawnZoneResetID, got.HomeRoomID)
		}

		zone7, err := r.ListByRespawnZone(ctx, 7)
		if err != nil {
			t.Fatalf("ListByRespawnZone 7: %v", err)
		}
		if len(zone7) != 1 || zone7[0].ID != a.ID {
			t.Fatalf("zone 7 list = %+v, want [a]", zone7)
		}

		empty, err := r.ListByRespawnZone(ctx, 12345)
		if err != nil {
			t.Fatalf("ListByRespawnZone empty: %v", err)
		}
		if len(empty) != 0 {
			t.Fatalf("empty zone list = %+v", empty)
		}

		if err := r.SetSpawnAnchor(ctx, a.ID+999, 1, 1); !errors.Is(err, ErrTemplateNotFound) {
			t.Fatalf("unknown id err = %v, want ErrTemplateNotFound", err)
		}
	})
}

func TestMemoryMobTemplateRepo(t *testing.T) {
	mk := func(t *testing.T) MobTemplateRepo { return NewMemoryMobTemplateRepo() }
	runMobTemplateRepoTests(t, "memory", mk)
	testListExternalIDs(t, "memory", mk)
}

func TestSQLiteMobTemplateRepo(t *testing.T) {
	mk := func(t *testing.T) MobTemplateRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteMobTemplateRepo(conn)
	}
	runMobTemplateRepoTests(t, "sqlite", mk)
	testListExternalIDs(t, "sqlite", mk)
}
