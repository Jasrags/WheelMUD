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

	t.Run(name+"/record_xp", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, err := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Aviendha"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := cr.RecordXP(ctx, c.ID, 1500); err != nil {
			t.Fatalf("record xp: %v", err)
		}
		got, _ := cr.GetByID(ctx, c.ID)
		if got.XP != 1500 {
			t.Fatalf("xp = %d, want 1500", got.XP)
		}
		if err := cr.RecordXP(ctx, c.ID+999, 100); !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("missing id: err = %v, want ErrCharacterNotFound", err)
		}
	})

	t.Run(name+"/get_by_id", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		created, err := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Nynaeve"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		got, err := cr.GetByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("get by id: %v", err)
		}
		if got.ID != created.ID || got.Name != "Nynaeve" {
			t.Fatalf("got %+v", got)
		}
		if _, err := cr.GetByID(ctx, created.ID+999); !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("missing id: err = %v, want ErrCharacterNotFound", err)
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
				Speed:   creature.Speed{BaseFt: 30},
				ReachFt: 5, FaceFt: 5, ThreatFt: 5,
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
			Coin:          currency.Amount(123),
			BankBalance:   currency.Amount(456),
			Position:      creature.StanceFighting,
			Encumbrance:   creature.LoadLight,
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

	t.Run(name+"/channeling_roundtrip", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})

		// Channeler — non-nil pointer with affinity bitmask + weave ids.
		ch := Character{
			AccountID:   acc.ID,
			Name:        "Egwene",
			Race:        creature.RaceHuman,
			Background:  creature.BackgroundMidlander,
			ClassLevels: map[creature.Class]int8{creature.ClassInitiate: 1},
			Channeling: &creature.Channeling{
				GenderSource:   creature.SourceSaidar,
				ChannelerType:  creature.ChannelerInitiate,
				Affinities:     1<<uint(creature.PowerFire) | 1<<uint(creature.PowerSpirit),
				WeavesKnownIDs: []string{"spark", "warmth", "steady_hand"},
			},
		}
		if _, err := cr.Create(ctx, ch); err != nil {
			t.Fatalf("create channeler: %v", err)
		}
		got, err := cr.FindByName(ctx, "Egwene")
		if err != nil {
			t.Fatalf("find channeler: %v", err)
		}
		if got.Channeling == nil {
			t.Fatalf("expected non-nil Channeling on channeler row")
		}
		if got.Channeling.GenderSource != creature.SourceSaidar {
			t.Fatalf("source mismatch: %v", got.Channeling.GenderSource)
		}
		if got.Channeling.ChannelerType != creature.ChannelerInitiate {
			t.Fatalf("type mismatch: %v", got.Channeling.ChannelerType)
		}
		if got.Channeling.Affinities != ch.Channeling.Affinities {
			t.Fatalf("affinities mismatch: got %b want %b",
				got.Channeling.Affinities, ch.Channeling.Affinities)
		}
		if !reflect.DeepEqual(got.Channeling.WeavesKnownIDs, ch.Channeling.WeavesKnownIDs) {
			t.Fatalf("weave ids mismatch: %+v", got.Channeling.WeavesKnownIDs)
		}

		// Non-channeler — nil pointer. Default 'null' on the column
		// must round-trip back as nil, not a zero-value struct.
		nonCh := Character{
			AccountID:   acc.ID,
			Name:        "Lan",
			Race:        creature.RaceHuman,
			Background:  creature.BackgroundBorderlander,
			ClassLevels: map[creature.Class]int8{creature.ClassArmsman: 1},
			// Channeling left nil
		}
		if _, err := cr.Create(ctx, nonCh); err != nil {
			t.Fatalf("create non-channeler: %v", err)
		}
		got2, err := cr.FindByName(ctx, "Lan")
		if err != nil {
			t.Fatalf("find non-channeler: %v", err)
		}
		if got2.Channeling != nil {
			t.Fatalf("expected nil Channeling on non-channeler; got %+v", got2.Channeling)
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

	t.Run(name+"/record_level_up_persists", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{
			AccountID: acc.ID, Name: "Egwene",
			ClassLevels: map[creature.Class]int8{creature.ClassArmsman: 1},
			Core:        creature.Core{HPCurrent: 12, HPMax: 12, BAB: 1},
		})
		newLevels := map[creature.Class]int8{
			creature.ClassArmsman: 2,
			creature.ClassWilder:  1,
		}
		newSaves := creature.Saves{Fort: 5, Ref: 1, Will: 3}
		if err := cr.RecordLevelUp(ctx, c.ID, LevelUpFields{
			ClassLevels: newLevels, HPCurrent: 20, HPMax: 20, BAB: 3, Saves: newSaves,
			PendingFeatsDelta: 1, PendingSkillPointsDelta: 5,
			PendingAbilityBumpsDelta: 0, PendingWeavesDelta: 1,
		}); err != nil {
			t.Fatalf("RecordLevelUp: %v", err)
		}
		got, err := cr.FindByName(ctx, "Egwene")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if got.Core.HPCurrent != 20 || got.Core.HPMax != 20 {
			t.Fatalf("HP not persisted: %+v", got.Core)
		}
		if got.Core.BAB != 3 {
			t.Fatalf("BAB = %d, want 3", got.Core.BAB)
		}
		if got.Core.Saves != newSaves {
			t.Fatalf("Saves = %+v, want %+v", got.Core.Saves, newSaves)
		}
		if got.ClassLevels[creature.ClassArmsman] != 2 ||
			got.ClassLevels[creature.ClassWilder] != 1 {
			t.Fatalf("ClassLevels not persisted: %+v", got.ClassLevels)
		}
		if got.PendingFeats != 1 || got.PendingSkillPoints != 5 ||
			got.PendingAbilityBumps != 0 || got.PendingWeaves != 1 {
			t.Fatalf("pending pools not persisted: feats=%d skill=%d abil=%d weave=%d",
				got.PendingFeats, got.PendingSkillPoints,
				got.PendingAbilityBumps, got.PendingWeaves)
		}
		// A second level-up accumulates onto the existing pools.
		if err := cr.RecordLevelUp(ctx, c.ID, LevelUpFields{
			ClassLevels: newLevels, HPCurrent: 20, HPMax: 20, BAB: 3, Saves: newSaves,
			PendingFeatsDelta: 0, PendingSkillPointsDelta: 4,
			PendingAbilityBumpsDelta: 1, PendingWeavesDelta: 0,
		}); err != nil {
			t.Fatalf("RecordLevelUp 2: %v", err)
		}
		got, _ = cr.FindByName(ctx, "Egwene")
		if got.PendingFeats != 1 || got.PendingSkillPoints != 9 ||
			got.PendingAbilityBumps != 1 || got.PendingWeaves != 1 {
			t.Fatalf("pending pools didn't accumulate: feats=%d skill=%d abil=%d weave=%d",
				got.PendingFeats, got.PendingSkillPoints,
				got.PendingAbilityBumps, got.PendingWeaves)
		}
	})

	t.Run(name+"/record_level_up_unknown_returns_not_found", func(t *testing.T) {
		cr, _ := newRepo(t)
		err := cr.RecordLevelUp(context.Background(), 9999, LevelUpFields{
			ClassLevels: map[creature.Class]int8{creature.ClassArmsman: 1},
			HPCurrent:   1, HPMax: 1,
		})
		if !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("err = %v, want ErrCharacterNotFound", err)
		}
	})

	t.Run(name+"/record_level_up_isolates_caller_map", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Mat",
			ClassLevels: map[creature.Class]int8{creature.ClassArmsman: 1}})
		levels := map[creature.Class]int8{creature.ClassArmsman: 2}
		if err := cr.RecordLevelUp(ctx, c.ID, LevelUpFields{
			ClassLevels: levels, HPCurrent: 12, HPMax: 12, BAB: 2,
		}); err != nil {
			t.Fatalf("RecordLevelUp: %v", err)
		}
		// Mutate the caller's map; stored state must not move.
		levels[creature.ClassArmsman] = 99
		got, _ := cr.FindByName(ctx, "Mat")
		if got.ClassLevels[creature.ClassArmsman] != 2 {
			t.Fatalf("caller mutation bled through: %+v", got.ClassLevels)
		}
	})

	t.Run(name+"/record_skill_rank_upserts_and_decrements", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{
			AccountID: acc.ID, Name: "Nynaeve",
			ClassLevels:        map[creature.Class]int8{creature.ClassWilder: 1},
			PendingSkillPoints: 5,
			Skills:             map[int32]creature.SkillRanks{42: {Ranks: 2, IsClassSkill: true}},
		})
		// Bump existing skill 42 from 2 → 3 (cost 1).
		if err := cr.RecordSkillRank(ctx, c.ID, 42, 3, true, 4); err != nil {
			t.Fatalf("RecordSkillRank existing: %v", err)
		}
		got, _ := cr.FindByName(ctx, "Nynaeve")
		if got.Skills[42].Ranks != 3 {
			t.Errorf("skill 42 ranks = %d, want 3", got.Skills[42].Ranks)
		}
		if got.PendingSkillPoints != 4 {
			t.Errorf("PendingSkillPoints = %d, want 4", got.PendingSkillPoints)
		}
		// Add a brand-new skill 99 with 2 ranks (cost 2).
		if err := cr.RecordSkillRank(ctx, c.ID, 99, 2, true, 2); err != nil {
			t.Fatalf("RecordSkillRank new: %v", err)
		}
		got, _ = cr.FindByName(ctx, "Nynaeve")
		if got.Skills[99].Ranks != 2 || !got.Skills[99].IsClassSkill {
			t.Errorf("skill 99 = %+v, want {Ranks:2, IsClassSkill:true}", got.Skills[99])
		}
		if got.Skills[42].Ranks != 3 {
			t.Errorf("skill 42 clobbered: %+v", got.Skills[42])
		}
		if got.PendingSkillPoints != 2 {
			t.Errorf("PendingSkillPoints = %d, want 2", got.PendingSkillPoints)
		}
	})

	t.Run(name+"/record_skill_rank_unknown_returns_not_found", func(t *testing.T) {
		cr, _ := newRepo(t)
		err := cr.RecordSkillRank(context.Background(), 9999, 1, 1, true, 0)
		if !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("err = %v, want ErrCharacterNotFound", err)
		}
	})

	t.Run(name+"/record_xp_debt_roundtrips", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Rand"})
		if c.XPDebt != 0 {
			t.Fatalf("fresh XPDebt = %d, want 0", c.XPDebt)
		}
		if err := cr.RecordXPDebt(ctx, c.ID, 1234); err != nil {
			t.Fatalf("RecordXPDebt: %v", err)
		}
		got, _ := cr.FindByName(ctx, "Rand")
		if got.XPDebt != 1234 {
			t.Errorf("XPDebt after set = %d, want 1234", got.XPDebt)
		}
		if err := cr.RecordXPDebt(ctx, c.ID, 0); err != nil {
			t.Fatalf("RecordXPDebt clear: %v", err)
		}
		got, _ = cr.FindByName(ctx, "Rand")
		if got.XPDebt != 0 {
			t.Errorf("XPDebt after clear = %d, want 0", got.XPDebt)
		}
	})

	t.Run(name+"/record_xp_debt_unknown_returns_not_found", func(t *testing.T) {
		cr, _ := newRepo(t)
		err := cr.RecordXPDebt(context.Background(), 9999, 100)
		if !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("err = %v, want ErrCharacterNotFound", err)
		}
	})

	t.Run(name+"/record_feat_pick_appends_and_decrements", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{
			AccountID:    acc.ID, Name: "Egwene",
			PendingFeats: 2,
			Feats:        []int32{111},
		})
		if err := cr.RecordFeatPick(ctx, c.ID, 222, 1); err != nil {
			t.Fatalf("RecordFeatPick first: %v", err)
		}
		got, _ := cr.FindByName(ctx, "Egwene")
		if !reflect.DeepEqual(got.Feats, []int32{111, 222}) {
			t.Fatalf("Feats = %v, want [111 222]", got.Feats)
		}
		if got.PendingFeats != 1 {
			t.Errorf("PendingFeats = %d, want 1", got.PendingFeats)
		}
		if err := cr.RecordFeatPick(ctx, c.ID, 333, 0); err != nil {
			t.Fatalf("RecordFeatPick second: %v", err)
		}
		got, _ = cr.FindByName(ctx, "Egwene")
		if !reflect.DeepEqual(got.Feats, []int32{111, 222, 333}) {
			t.Fatalf("Feats = %v, want [111 222 333]", got.Feats)
		}
		if got.PendingFeats != 0 {
			t.Errorf("PendingFeats = %d, want 0", got.PendingFeats)
		}
	})

	t.Run(name+"/record_feat_pick_unknown_returns_not_found", func(t *testing.T) {
		cr, _ := newRepo(t)
		err := cr.RecordFeatPick(context.Background(), 9999, 42, 0)
		if !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("err = %v, want ErrCharacterNotFound", err)
		}
	})

	t.Run(name+"/record_ability_bump_writes_one_column", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		seed := Character{AccountID: acc.ID, Name: "Lan", PendingAbilityBumps: 3}
		seed.Core.Abilities.Str.Current = 14
		seed.Core.Abilities.Dex.Current = 12
		c, _ := cr.Create(ctx, seed)

		if err := cr.RecordAbilityBump(ctx, c.ID, AbilityStr, 15, 2); err != nil {
			t.Fatalf("bump str: %v", err)
		}
		got, _ := cr.FindByName(ctx, "Lan")
		if got.Core.Abilities.Str.Current != 15 {
			t.Errorf("Str = %d, want 15", got.Core.Abilities.Str.Current)
		}
		if got.Core.Abilities.Dex.Current != 12 {
			t.Errorf("Dex clobbered: %d, want 12", got.Core.Abilities.Dex.Current)
		}
		if got.PendingAbilityBumps != 2 {
			t.Errorf("PendingAbilityBumps = %d, want 2", got.PendingAbilityBumps)
		}

		if err := cr.RecordAbilityBump(ctx, c.ID, AbilityDex, 13, 1); err != nil {
			t.Fatalf("bump dex: %v", err)
		}
		got, _ = cr.FindByName(ctx, "Lan")
		if got.Core.Abilities.Dex.Current != 13 {
			t.Errorf("Dex after bump = %d, want 13", got.Core.Abilities.Dex.Current)
		}
		if got.Core.Abilities.Str.Current != 15 {
			t.Errorf("Str regressed: %d, want 15", got.Core.Abilities.Str.Current)
		}
	})

	t.Run(name+"/record_ability_bump_unknown_returns_not_found", func(t *testing.T) {
		cr, _ := newRepo(t)
		err := cr.RecordAbilityBump(context.Background(), 9999, AbilityStr, 10, 0)
		if !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("err = %v, want ErrCharacterNotFound", err)
		}
	})

	t.Run(name+"/record_weave_pick_appends_and_decrements", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{
			AccountID:     acc.ID, Name: "Moiraine",
			PendingWeaves: 2,
			Channeling: &creature.Channeling{
				ChannelerType:  creature.ChannelerInitiate,
				Affinities:     creature.PowerSet(1<<creature.PowerFire) | creature.PowerSet(1<<creature.PowerSpirit),
				WeavesKnownIDs: []string{"spark"},
			},
		})
		if err := cr.RecordWeavePick(ctx, c.ID, "ember", 1); err != nil {
			t.Fatalf("RecordWeavePick: %v", err)
		}
		got, _ := cr.FindByName(ctx, "Moiraine")
		if got.Channeling == nil {
			t.Fatalf("Channeling cleared")
		}
		if !reflect.DeepEqual(got.Channeling.WeavesKnownIDs, []string{"spark", "ember"}) {
			t.Fatalf("WeavesKnownIDs = %v", got.Channeling.WeavesKnownIDs)
		}
		if got.PendingWeaves != 1 {
			t.Errorf("PendingWeaves = %d, want 1", got.PendingWeaves)
		}
	})

	t.Run(name+"/record_weave_pick_non_channeler_returns_err", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Perrin"})
		err := cr.RecordWeavePick(ctx, c.ID, "spark", 0)
		if !errors.Is(err, ErrNotChanneler) {
			t.Fatalf("err = %v, want ErrNotChanneler", err)
		}
	})

	t.Run(name+"/record_weave_pick_unknown_returns_not_found", func(t *testing.T) {
		cr, _ := newRepo(t)
		err := cr.RecordWeavePick(context.Background(), 9999, "spark", 0)
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

	t.Run(name+"/record_equipment_roundtrips", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "ow", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Perrin"})
		eq := creature.Equipment{
			Armor:        11,
			PrimaryWield: 22,
			OffHand:      33,
			BeltPouches:  []int64{44, 55},
		}
		if err := cr.RecordEquipment(ctx, c.ID, eq); err != nil {
			t.Fatalf("RecordEquipment: %v", err)
		}
		got, err := cr.FindByName(ctx, "Perrin")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if got.Equipment.Armor != 11 || got.Equipment.PrimaryWield != 22 || got.Equipment.OffHand != 33 {
			t.Fatalf("equipment slot mismatch: %+v", got.Equipment)
		}
		if !reflect.DeepEqual(got.Equipment.BeltPouches, []int64{44, 55}) {
			t.Fatalf("BeltPouches mismatch: %+v", got.Equipment.BeltPouches)
		}
		// Overwrite to empty.
		if err := cr.RecordEquipment(ctx, c.ID, creature.Equipment{}); err != nil {
			t.Fatalf("RecordEquipment empty: %v", err)
		}
		got, _ = cr.FindByName(ctx, "Perrin")
		if got.Equipment.PrimaryWield != 0 || len(got.Equipment.BeltPouches) != 0 {
			t.Fatalf("equipment not cleared: %+v", got.Equipment)
		}
	})

	t.Run(name+"/record_equipment_unknown_returns_not_found", func(t *testing.T) {
		cr, _ := newRepo(t)
		err := cr.RecordEquipment(context.Background(), 9999, creature.Equipment{})
		if !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("err = %v, want ErrCharacterNotFound", err)
		}
	})

	t.Run(name+"/delete_removes_row", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, err := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Doomed"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := cr.Delete(ctx, c.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := cr.FindByName(ctx, "Doomed"); !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("post-delete FindByName err = %v, want ErrCharacterNotFound", err)
		}
		got, _ := cr.ListByAccount(ctx, acc.ID)
		for _, lc := range got {
			if lc.ID == c.ID {
				t.Fatalf("deleted character still appears in ListByAccount: %+v", got)
			}
		}
	})

	t.Run(name+"/delete_missing_returns_not_found", func(t *testing.T) {
		cr, _ := newRepo(t)
		if err := cr.Delete(context.Background(), 9999); !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("err = %v, want ErrCharacterNotFound", err)
		}
	})

	t.Run(name+"/delete_frees_name", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepo(t)
		acc, _ := ar.Create(ctx, Account{Username: "owner", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Reborn"})
		if err := cr.Delete(ctx, c.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		// Same name must be reusable post-delete.
		if _, err := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Reborn"}); err != nil {
			t.Fatalf("recreate after delete: %v", err)
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

// runRecordCoinVersionTests exercises the optimistic-concurrency
// contract on RecordCoin: success bumps the version, mismatched
// versions return ErrCoinConflict without mutating the row, and a
// missing id still returns ErrCharacterNotFound.
func runRecordCoinVersionTests(t *testing.T, name string, newRepos func(t *testing.T) (CharacterRepo, AccountRepo)) {
	t.Helper()

	t.Run(name+"/record_coin_bumps_version", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepos(t)
		acc, _ := ar.Create(ctx, Account{Username: "u1", PasswordHash: "h"})
		c, err := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Alpha"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if c.CoinVersion != 0 {
			t.Fatalf("fresh character coin_version = %d, want 0", c.CoinVersion)
		}
		if err := cr.RecordCoin(ctx, c.ID, 100, 0, c.CoinVersion); err != nil {
			t.Fatalf("first RecordCoin: %v", err)
		}
		got, _ := cr.FindByName(ctx, "Alpha")
		if got.CoinVersion != 1 {
			t.Fatalf("after first write coin_version = %d, want 1", got.CoinVersion)
		}
		if int64(got.Coin) != 100 {
			t.Fatalf("coin = %d, want 100", int64(got.Coin))
		}
	})

	t.Run(name+"/record_coin_stale_version_refuses", func(t *testing.T) {
		ctx := context.Background()
		cr, ar := newRepos(t)
		acc, _ := ar.Create(ctx, Account{Username: "u2", PasswordHash: "h"})
		c, _ := cr.Create(ctx, Character{AccountID: acc.ID, Name: "Beta"})
		// First write succeeds with version 0.
		if err := cr.RecordCoin(ctx, c.ID, 50, 0, 0); err != nil {
			t.Fatalf("first write: %v", err)
		}
		// Second write with stale version 0 must fail; row is now at 1.
		err := cr.RecordCoin(ctx, c.ID, 999, 0, 0)
		if !errors.Is(err, ErrCoinConflict) {
			t.Fatalf("err = %v, want ErrCoinConflict", err)
		}
		// Verify the row didn't budge.
		got, _ := cr.FindByName(ctx, "Beta")
		if int64(got.Coin) != 50 {
			t.Fatalf("conflict mutated coin: %d", int64(got.Coin))
		}
		if got.CoinVersion != 1 {
			t.Fatalf("conflict bumped version: %d", got.CoinVersion)
		}
	})

	t.Run(name+"/record_coin_missing_id_returns_not_found", func(t *testing.T) {
		ctx := context.Background()
		cr, _ := newRepos(t)
		err := cr.RecordCoin(ctx, 9999, 1, 0, 0)
		if !errors.Is(err, ErrCharacterNotFound) {
			t.Fatalf("err = %v, want ErrCharacterNotFound", err)
		}
	})
}

func TestMemoryCharacterRepo(t *testing.T) {
	mk := func(t *testing.T) (CharacterRepo, AccountRepo) {
		return NewMemoryCharacterRepo(), NewMemoryAccountRepo()
	}
	runCharacterRepoTests(t, "memory", mk)
	runRecordCoinVersionTests(t, "memory", mk)
}

func TestSQLiteCharacterRepo(t *testing.T) {
	mk := func(t *testing.T) (CharacterRepo, AccountRepo) {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteCharacterRepo(conn), NewSQLiteAccountRepo(conn)
	}
	runCharacterRepoTests(t, "sqlite", mk)
	runRecordCoinVersionTests(t, "sqlite", mk)
}
