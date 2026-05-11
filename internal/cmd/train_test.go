package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// trainFixture stages a trainer mob in room 1 plus the Alice player
// session. Standalone (does not use invFixture) so we can seed
// Alice's ClassLevels and Core abilities at character-create time.
type trainFixture struct {
	characters *repo.MemoryCharacterRepo
	mobs       *repo.MemoryMobInstanceRepo
	templates  *repo.MemoryMobTemplateRepo
	trainers   *repo.MemoryTrainerRepo
	audits     *repo.MemoryAdminAuditRepo
	cat        *chargen.Catalog
	sessions   *session.Registry
	alice      *telnet.Session
	aOut       *bufConn
	keeperTpl  int64
}

// newTrainFixture seeds Alice with the given XP and Armsman class
// level (single-class baseline) and a trainer NPC keyed to
// trainerClassID. Alice's abilities are 14 Con / 12 Dex / 10 Wis so
// HP/save deltas have non-zero modifiers in commit assertions.
func newTrainFixture(t *testing.T, trainerClassID string, aliceXP int64, aliceLvl int8) *trainFixture {
	t.Helper()

	chars := repo.NewMemoryCharacterRepo()
	if _, err := chars.Create(context.Background(), repo.Character{
		AccountID:     100,
		Name:          "Alice",
		CurrentRoomID: 1,
		ClassLevels:   map[creature.Class]int8{creature.ClassArmsman: aliceLvl},
		XP:            aliceXP,
		Core: creature.Core{
			HPCurrent: 12, HPMax: 12,
			BAB:   1, // chargen-seeded armsman L1
			Saves: creature.Saves{Fort: 4, Ref: 1, Will: 0},
			Abilities: creature.Abilities{
				Con: creature.AbilityScore{Current: 14, Max: 14},
				Dex: creature.AbilityScore{Current: 12, Max: 12},
				Wis: creature.AbilityScore{Current: 10, Max: 10},
			},
		},
	}); err != nil {
		t.Fatalf("seed Alice: %v", err)
	}

	templates := repo.NewMemoryMobTemplateRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	trainers := repo.NewMemoryTrainerRepo()
	audits := repo.NewMemoryAdminAuditRepo()

	tpl, err := templates.Create(context.Background(), creature.MobTemplate{
		ExternalID: "city.weaponmaster",
		Core:       creature.Core{Name: "Lan"},
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	if _, err := mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: tpl.ID,
		Core:       creature.Core{CurrentRoomID: 1},
	}); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if _, err := trainers.Create(context.Background(), repo.Trainer{
		MobTemplateID: tpl.ID,
		ClassID:       trainerClassID,
	}); err != nil {
		t.Fatalf("create trainer: %v", err)
	}

	fsys, err := chargen.SourceFS()
	if err != nil {
		t.Fatalf("chargen source: %v", err)
	}
	cat, err := chargen.Load(fsys)
	if err != nil {
		t.Fatalf("load chargen catalog: %v", err)
	}

	sessions := session.NewRegistry()
	alice, aConn := bufSession(t)
	alice.AccountID = 100
	alice.AuthLevel = telnet.AuthPlayer
	alice.CharacterID = 1
	alice.CharacterName = "Alice"
	alice.CurrentRoomID = 1
	sessions.Bind(alice.AccountID, alice)

	return &trainFixture{
		characters: chars,
		mobs:       mobs,
		templates:  templates,
		trainers:   trainers,
		audits:     audits,
		cat:        cat,
		sessions:   sessions,
		alice:      alice,
		aOut:       aConn,
		keeperTpl:  tpl.ID,
	}
}

func (f *trainFixture) trainCmd() *telnet.Command {
	return NewTrain(f.characters, f.mobs, f.templates, f.trainers, f.cat, f.audits)
}

func TestTrain_NoTrainerInRoom(t *testing.T) {
	f := newTrainFixture(t, "armsman", 1500, 1)
	f.alice.CurrentRoomID = 2 // empty room

	runCmd(t, f.trainCmd(), f.alice, "")
	if !strings.Contains(f.aOut.String(), "no trainer here") {
		t.Fatalf("expected refusal:\n%s", f.aOut.String())
	}
	if rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10}); len(rows) != 0 {
		t.Fatalf("refusal must not audit: %d rows", len(rows))
	}
}

func TestTrain_NotReadyToAdvance(t *testing.T) {
	// XP 0 → level 1; classTotal 1 → 0 pending.
	f := newTrainFixture(t, "armsman", 0, 1)

	runCmd(t, f.trainCmd(), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "not ready to advance") {
		t.Fatalf("expected 'not ready to advance':\n%s", out)
	}
	if rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10}); len(rows) != 0 {
		t.Fatalf("refusal must not audit: %d rows", len(rows))
	}
}

func TestTrain_CommitsLevelAndPersists(t *testing.T) {
	// XP 1500 → level 2; classTotal 1 → 1 pending.
	f := newTrainFixture(t, "armsman", 1500, 1)

	runCmd(t, f.trainCmd(), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "Armsman") || !strings.Contains(out, "L2") {
		t.Fatalf("expected commit line with class+level:\n%s", out)
	}

	got, err := f.characters.FindByName(context.Background(), "Alice")
	if err != nil {
		t.Fatalf("find alice: %v", err)
	}
	if got.ClassLevels[creature.ClassArmsman] != 2 {
		t.Errorf("class level = %d, want 2", got.ClassLevels[creature.ClassArmsman])
	}
	// Armsman d10 + Con+2 = 8 HP delta. Pre: 12/12. Post: 20/20.
	if got.Core.HPMax != 20 || got.Core.HPCurrent != 20 {
		t.Errorf("HP after train = %d/%d, want 20/20", got.Core.HPCurrent, got.Core.HPMax)
	}
	// Armsman L2 BAB high = 2.
	if got.Core.BAB != 2 {
		t.Errorf("BAB = %d, want 2", got.Core.BAB)
	}
	// Armsman L2 Fort high = 2 + 2/2 = 3, +Con(+2) = 5.
	if got.Core.Saves.Fort != 5 {
		t.Errorf("Fort = %d, want 5", got.Core.Saves.Fort)
	}
}

func TestTrain_AuditsOnSuccess(t *testing.T) {
	f := newTrainFixture(t, "armsman", 1500, 1)

	runCmd(t, f.trainCmd(), f.alice, "")
	rows, err := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(rows))
	}
	if rows[0].Verb != "train" {
		t.Errorf("verb = %q, want \"train\"", rows[0].Verb)
	}
	if rows[0].Target != "armsman" {
		t.Errorf("target = %q, want \"armsman\"", rows[0].Target)
	}
	if rows[0].Args != "L2" {
		t.Errorf("args = %q, want \"L2\"", rows[0].Args)
	}
}

func TestTrain_OpensNewClassMulticlass(t *testing.T) {
	// Alice is Armsman L1 with XP for L2; visiting a Wilder trainer
	// opens Wilder at L1 (multiclass).
	f := newTrainFixture(t, "wilder", 1500, 1)

	runCmd(t, f.trainCmd(), f.alice, "")
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.ClassLevels[creature.ClassWilder] != 1 {
		t.Errorf("wilder = %d, want 1", got.ClassLevels[creature.ClassWilder])
	}
	if got.ClassLevels[creature.ClassArmsman] != 1 {
		t.Errorf("armsman preserved = %d, want 1", got.ClassLevels[creature.ClassArmsman])
	}
}

func TestTrain_RefusesOnUnknownClassID(t *testing.T) {
	// Trainer's class id isn't in the chargen catalog.
	f := newTrainFixture(t, "ghoulkin", 1500, 1)

	before, _ := f.characters.FindByName(context.Background(), "Alice")
	runCmd(t, f.trainCmd(), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "path's broken") {
		t.Fatalf("expected catalog-miss refusal:\n%s", out)
	}

	after, _ := f.characters.FindByName(context.Background(), "Alice")
	if after.ClassLevels[creature.ClassArmsman] != before.ClassLevels[creature.ClassArmsman] {
		t.Errorf("catalog miss must not mutate ClassLevels")
	}
	if after.Core.HPMax != before.Core.HPMax {
		t.Errorf("catalog miss must not mutate HP")
	}
	if rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10}); len(rows) != 0 {
		t.Fatalf("catalog miss must not audit: %d rows", len(rows))
	}
}

func TestTrain_PendingLineRendersSkillOnly(t *testing.T) {
	// L1→L2 for Armsman has no feat (3-cycle) or ability bump
	// (4-cycle). With Int unset (mod -5) skill delta is floored at 1.
	f := newTrainFixture(t, "armsman", 1500, 1)
	runCmd(t, f.trainCmd(), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "1 skill point") {
		t.Errorf("expected '1 skill point' in pending line:\n%s", out)
	}
	if strings.Contains(out, "feat pick") {
		t.Errorf("L2 must not grant a feat:\n%s", out)
	}
	if strings.Contains(out, "ability bump") {
		t.Errorf("L2 must not grant an ability bump:\n%s", out)
	}
}

func TestTrain_PendingLineHasFeatAtMultiplesOfThree(t *testing.T) {
	// Seed Alice as Armsman L2 with XP for L3.
	f := newTrainFixture(t, "armsman", 3000, 2)
	runCmd(t, f.trainCmd(), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "1 feat pick") {
		t.Errorf("expected '1 feat pick' at L3:\n%s", out)
	}
}

func TestTrain_PendingLineHasAbilityAtMultiplesOfFour(t *testing.T) {
	// Seed Alice as Armsman L3 with XP for L4.
	f := newTrainFixture(t, "armsman", 6000, 3)
	runCmd(t, f.trainCmd(), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "1 ability bump") {
		t.Errorf("expected '1 ability bump' at L4:\n%s", out)
	}
}

func TestTrain_PendingPoolsPersistAfterTrain(t *testing.T) {
	// L2→L3: feat 1, skill 1, ability 0, weave 0.
	f := newTrainFixture(t, "armsman", 3000, 2)
	runCmd(t, f.trainCmd(), f.alice, "")
	got, err := f.characters.FindByName(context.Background(), "Alice")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.PendingFeats != 1 {
		t.Errorf("PendingFeats = %d, want 1", got.PendingFeats)
	}
	if got.PendingSkillPoints != 1 {
		t.Errorf("PendingSkillPoints = %d, want 1", got.PendingSkillPoints)
	}
	if got.PendingAbilityBumps != 0 {
		t.Errorf("PendingAbilityBumps = %d, want 0", got.PendingAbilityBumps)
	}
	if got.PendingWeaves != 0 {
		t.Errorf("PendingWeaves = %d, want 0", got.PendingWeaves)
	}
}

func TestTrain_NilCatalogRefuses(t *testing.T) {
	f := newTrainFixture(t, "armsman", 1500, 1)

	cmd := NewTrain(f.characters, f.mobs, f.templates, f.trainers, nil, f.audits)
	runCmd(t, cmd, f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "path's broken") {
		t.Fatalf("expected catalog-miss refusal on nil catalog:\n%s", out)
	}

	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.ClassLevels[creature.ClassArmsman] != 1 {
		t.Errorf("nil catalog must not mutate ClassLevels")
	}
}
