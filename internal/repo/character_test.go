package repo

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/db"
)

func runCharacterRepoTests(t *testing.T, name string, newRepo func(t *testing.T) (CharacterRepo, AccountRepo)) {
	t.Helper()
	t.Run(name+"/create+find", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		got, err := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Frodo"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if got.ID == 0 {
			t.Fatal("ID not set")
		}
		if got.NameLower != "frodo" {
			t.Fatalf("NameLower = %q", got.NameLower)
		}
		found, err := cr.FindByName(ctx, "FRODO")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if found.ID != got.ID || found.AccountID != acc.ID {
			t.Fatalf("found %+v", found)
		}
	})

	t.Run(name+"/duplicate", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		a1, _ := ar.Create(ctx, Account{Username: "a1", PasswordHash: "h"})
		a2, _ := ar.Create(ctx, Account{Username: "a2", PasswordHash: "h"})
		if _, err := cr.Create(ctx, Character{AccountID: a1.ID, Name: "Sam"}); err != nil {
			t.Fatalf("first create: %v", err)
		}
		_, err := cr.Create(ctx, Character{AccountID: a2.ID, Name: "SAM"})
		if !errors.Is(err, ErrDuplicateCharacterName) {
			t.Fatalf("err = %v, want ErrDuplicateCharacterName", err)
		}
	})

	t.Run(name+"/missing", func(t *testing.T) {
		cr, _ := newRepo(t)
		_, err := cr.FindByName(context.Background(), "ghost")
		if !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("err = %v, want ErrCharacterNotFound", err)
		}
	})

	t.Run(name+"/list_by_account_orders_recent_first", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		// Three characters; only the second has a last_played_at.
		c1, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "alpha"})
		c2, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "beta"})
		c3, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "gamma"})
		_ = cr.RecordPlay(ctx, c2.ID, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC))

		got, err := cr.ListByAccount(ctx, acc.ID)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("got %d, want 3", len(got))
		}
		// Played first; then alphabetical.
		want := []int64{c2.ID, c1.ID, c3.ID}
		for i, id := range want {
			if got[i].ID != id {
				t.Fatalf("position %d: got id %d, want %d (got=%+v)", i, got[i].ID, id, got)
			}
		}
	})

	t.Run(name+"/create_defaults_to_starter_room", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		got, err := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Pippin"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if got.CurrentRoomID != StarterRoomID {
			t.Fatalf("CurrentRoomID = %d, want %d", got.CurrentRoomID, StarterRoomID)
		}
		// Round-trip via FindByName.
		found, err := cr.FindByName(ctx, "Pippin")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if found.CurrentRoomID != StarterRoomID {
			t.Fatalf("found.CurrentRoomID = %d, want %d", found.CurrentRoomID, StarterRoomID)
		}
	})

	t.Run(name+"/record_room_persists", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Merry"})
		if err := cr.RecordRoom(ctx, c.ID, 42); err != nil {
			t.Fatalf("RecordRoom: %v", err)
		}
		found, err := cr.FindByName(ctx, "Merry")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if found.CurrentRoomID != 42 {
			t.Fatalf("CurrentRoomID = %d, want 42", found.CurrentRoomID)
		}
	})

	t.Run(name+"/full_core_and_player_roundtrip", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		want := Character{
			AccountID: acc.ID,
			Name:      "Rand",
			Core: creature.Core{
				Size: creature.SizeMedium, Type: creature.TypeHumanoid,
				Gender: creature.GenderMale, Alignment: creature.PostureGood,
				Abilities: creature.Abilities{
					Str: creature.AbilityScore{Current: 14, Max: 14, Inherent: 14},
					Dex: creature.AbilityScore{Current: 16, Max: 16, Inherent: 16},
					Con: creature.AbilityScore{Current: 13, Max: 13, Inherent: 13},
					Int: creature.AbilityScore{Current: 12, Max: 12, Inherent: 12},
					Wis: creature.AbilityScore{Current: 11, Max: 11, Inherent: 11},
					Cha: creature.AbilityScore{Current: 15, Max: 15, Inherent: 15},
				},
				HPCurrent: 8, HPMax: 10, Subdual: 0, HitDice: "1d10",
				Defense: 14,
				Saves:   creature.Saves{Fort: 1, Ref: 3, Will: 0},
				InitMod: 3, BAB: 1,
				Speed:    creature.Speed{BaseFt: 30},
				ReachFt:  5, FaceFt: 5, ThreatFt: 5,
				Conditions: creature.CondFatigued,
				Position:   creature.PosCharging,
				Specials:   creature.QualLowLightVision,
				DR:         []creature.DamageReduction{{Amount: 1}},
				Resists:    []creature.Resist{{Type: creature.DamageFire, Pct: 10}},
			},
			Race:        creature.RaceHuman,
			Background:  creature.BackgroundAiel,
			ClassLevels: map[creature.Class]int8{creature.ClassAlgaiDSiswai: 1},
			XP:          1000,
			Feats:       []int32{42},
			Skills:      map[int32]creature.SkillRanks{7: {Ranks: 4, IsClassSkill: true}},
			HeightCm:    195, WeightKg: 88, Age: 20,
			Handedness: creature.HandRight,
			Fame:       12, Infamy: 3, InfamyShare: 0.2,
			Coin:        currency.Amount(123),
			BankBalance: currency.Amount(456),
			Position:    creature.StanceFighting,
			Encumbrance: creature.LoadLight,
			BoundRoomID:   StarterRoomID,
			PlayedSeconds: 7200,
			LastLogin:     time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
			Inventory:     []int64{11, 22},
		}
		got, err := cr.Create(ctx, want)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		fetched, err := cr.FindByName(ctx, "Rand")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if fetched.ID != got.ID {
			t.Fatalf("id roundtrip: got %d want %d", fetched.ID, got.ID)
		}
		if fetched.Core.HPCurrent != 8 || fetched.Core.HPMax != 10 || fetched.Core.Defense != 14 {
			t.Fatalf("Core mismatch: %+v", fetched.Core)
		}
		if fetched.Core.Abilities != want.Core.Abilities {
			t.Fatalf("Abilities mismatch: got %+v want %+v", fetched.Core.Abilities, want.Core.Abilities)
		}
		if fetched.Core.Conditions != creature.CondFatigued || fetched.Core.Position != creature.PosCharging {
			t.Fatalf("conditions/position mismatch: %+v", fetched.Core)
		}
		if !reflect.DeepEqual(fetched.Core.DR, want.Core.DR) {
			t.Fatalf("DR mismatch: %+v", fetched.Core.DR)
		}
		if !reflect.DeepEqual(fetched.Core.Resists, want.Core.Resists) {
			t.Fatalf("Resists mismatch: %+v", fetched.Core.Resists)
		}
		if fetched.Race != want.Race || fetched.Background != want.Background {
			t.Fatalf("race/background mismatch: %v / %v", fetched.Race, fetched.Background)
		}
		if !reflect.DeepEqual(fetched.ClassLevels, want.ClassLevels) {
			t.Fatalf("ClassLevels mismatch: %+v", fetched.ClassLevels)
		}
		if fetched.XP != want.XP || fetched.HeightCm != want.HeightCm {
			t.Fatalf("xp/height mismatch")
		}
		if fetched.Coin != want.Coin || fetched.BankBalance != want.BankBalance {
			t.Fatalf("wealth mismatch: coin=%d bank=%d", fetched.Coin, fetched.BankBalance)
		}
		if fetched.Position != creature.StanceFighting {
			t.Fatalf("Stance mismatch: %v", fetched.Position)
		}
		if !fetched.LastLogin.Equal(want.LastLogin) {
			t.Fatalf("LastLogin mismatch: %v", fetched.LastLogin)
		}
		if !reflect.DeepEqual(fetched.Inventory, want.Inventory) {
			t.Fatalf("Inventory mismatch: %v", fetched.Inventory)
		}
	})

	t.Run(name+"/record_core_persists", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Mat",
			Core: creature.Core{HPCurrent: 10, HPMax: 10}})
		if err := cr.RecordCore(ctx, c.ID, 4, 1, creature.CondStunned, creature.PosFlanked); err != nil {
			t.Fatalf("RecordCore: %v", err)
		}
		got, err := cr.FindByName(ctx, "Mat")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if got.Core.HPCurrent != 4 || got.Core.Subdual != 1 ||
			got.Core.Conditions != creature.CondStunned || got.Core.Position != creature.PosFlanked {
			t.Fatalf("RecordCore did not persist: %+v", got.Core)
		}
	})

	t.Run(name+"/record_core_unknown_returns_not_found", func(t *testing.T) {
		cr, _ := newRepo(t)
		err := cr.RecordCore(context.Background(), 9999, 0, 0, 0, 0)
		if !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("err = %v, want ErrCharacterNotFound", err)
		}
	})

	t.Run(name+"/first_character_promoted_to_admin", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		// Caller asks for player; repo overrides to admin because the
		// characters table is empty.
		first, err := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Lan", AuthLevel: AuthLevelPlayer})
		if err != nil {
			t.Fatalf("first create: %v", err)
		}
		if first.AuthLevel != AuthLevelAdmin {
			t.Fatalf("first character AuthLevel = %d, want AuthLevelAdmin", first.AuthLevel)
		}
		got, _ := cr.FindByName(ctx, "Lan")
		if got.AuthLevel != AuthLevelAdmin {
			t.Fatalf("first character AuthLevel persisted = %d, want AuthLevelAdmin", got.AuthLevel)
		}

		// Second character on the same account honors the requested
		// level — admin is per-character, not per-account.
		second, err := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Nynaeve", AuthLevel: AuthLevelPlayer})
		if err != nil {
			t.Fatalf("second create: %v", err)
		}
		if second.AuthLevel != AuthLevelPlayer {
			t.Fatalf("second character AuthLevel = %d, want AuthLevelPlayer", second.AuthLevel)
		}
	})

	t.Run(name+"/record_prompt_template_roundtrip", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Mat"})
		if c.PromptTemplate != "" {
			t.Fatalf("fresh character PromptTemplate = %q, want empty", c.PromptTemplate)
		}
		want := "<{{red}}%h::red/%H> "
		if err := cr.RecordPromptTemplate(ctx, c.ID, want); err != nil {
			t.Fatalf("RecordPromptTemplate: %v", err)
		}
		got, _ := cr.FindByName(ctx, "Mat")
		if got.PromptTemplate != want {
			t.Fatalf("PromptTemplate after set = %q, want %q", got.PromptTemplate, want)
		}
		if err := cr.RecordPromptTemplate(ctx, c.ID, ""); err != nil {
			t.Fatalf("RecordPromptTemplate clear: %v", err)
		}
		got, _ = cr.FindByName(ctx, "Mat")
		if got.PromptTemplate != "" {
			t.Fatalf("PromptTemplate after clear = %q, want empty", got.PromptTemplate)
		}
	})

	t.Run(name+"/record_prompt_template_unknown_returns_not_found", func(t *testing.T) {
		cr, _ := newRepo(t)
		err := cr.RecordPromptTemplate(context.Background(), 9999, "x")
		if !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("err = %v, want ErrCharacterNotFound", err)
		}
	})

	t.Run(name+"/mark_news_seen_advances_and_clamps", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Egwene"})
		if !c.LastNewsSeen.IsZero() {
			t.Fatalf("fresh character LastNewsSeen = %v, want zero", c.LastNewsSeen)
		}
		t1 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
		if err := cr.MarkNewsSeen(ctx, c.ID, t1); err != nil {
			t.Fatalf("MarkNewsSeen t1: %v", err)
		}
		if err := cr.MarkNewsSeen(ctx, c.ID, t2); err != nil {
			t.Fatalf("MarkNewsSeen t2: %v", err)
		}
		// Stale write must not regress.
		if err := cr.MarkNewsSeen(ctx, c.ID, t1); err != nil {
			t.Fatalf("MarkNewsSeen stale: %v", err)
		}
		got, err := cr.FindByName(ctx, "Egwene")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if !got.LastNewsSeen.Equal(t2) {
			t.Fatalf("LastNewsSeen = %v, want %v", got.LastNewsSeen, t2)
		}
	})

	t.Run(name+"/mark_news_seen_unknown_returns_not_found", func(t *testing.T) {
		cr, _ := newRepo(t)
		err := cr.MarkNewsSeen(context.Background(), 9999, time.Now())
		if !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("err = %v, want ErrCharacterNotFound", err)
		}
	})

	t.Run(name+"/list_by_account_isolates", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		a1, _ := ar.Create(ctx, Account{Username: "a1", PasswordHash: "h"})
		a2, _ := ar.Create(ctx, Account{Username: "a2", PasswordHash: "h"})
		_, _ = cr.Create(ctx, Character{AccountID: a1.ID, Name: "alpha"})
		_, _ = cr.Create(ctx, Character{AccountID: a2.ID, Name: "beta"})
		got, _ := cr.ListByAccount(ctx, a1.ID)
		if len(got) != 1 || got[0].Name != "alpha" {
			t.Fatalf("a1 got %+v", got)
		}
	})
}

func TestMemoryCharacterRepo(t *testing.T) {
	runCharacterRepoTests(t, "memory", func(t *testing.T) (CharacterRepo, AccountRepo) {
		return NewMemoryCharacterRepo(), NewMemoryAccountRepo()
	})
}

func TestSQLiteCharacterRepo(t *testing.T) {
	runCharacterRepoTests(t, "sqlite", func(t *testing.T) (CharacterRepo, AccountRepo) {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteCharacterRepo(conn), NewSQLiteAccountRepo(conn)
	})
}
