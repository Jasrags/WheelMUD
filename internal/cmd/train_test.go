package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/progression"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// trainFixture stages a trainer mob in room 1 plus the Alice player
// session. Standalone (does not use invFixture) so we can seed
// Alice's ClassLevels at character-create time — there's no repo
// method to mutate ClassLevels on an existing row in slice 2.
type trainFixture struct {
	characters *repo.MemoryCharacterRepo
	mobs       *repo.MemoryMobInstanceRepo
	templates  *repo.MemoryMobTemplateRepo
	trainers   *repo.MemoryTrainerRepo
	cat        *chargen.Catalog
	sessions   *session.Registry
	alice      *telnet.Session
	aOut       *bufConn
	keeperTpl  int64
}

// newTrainFixture seeds Alice with the given XP and Armsman class
// level (single-class for V1) and a trainer NPC keyed to trainerClassID.
func newTrainFixture(t *testing.T, trainerClassID string, aliceXP int64, aliceLvl int8) *trainFixture {
	t.Helper()

	chars := repo.NewMemoryCharacterRepo()
	if _, err := chars.Create(context.Background(), repo.Character{
		AccountID:     100,
		Name:          "Alice",
		CurrentRoomID: 1,
		ClassLevels:   map[creature.Class]int8{creature.ClassArmsman: aliceLvl},
		XP:            aliceXP,
	}); err != nil {
		t.Fatalf("seed Alice: %v", err)
	}

	templates := repo.NewMemoryMobTemplateRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	trainers := repo.NewMemoryTrainerRepo()

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
		cat:        cat,
		sessions:   sessions,
		alice:      alice,
		aOut:       aConn,
		keeperTpl:  tpl.ID,
	}
}

func (f *trainFixture) trainCmd() *telnet.Command {
	return NewTrain(f.characters, f.mobs, f.templates, f.trainers, f.cat)
}

func TestTrain_NoTrainerInRoom(t *testing.T) {
	f := newTrainFixture(t, "armsman", 1500, 1)
	f.alice.CurrentRoomID = 2 // empty room

	runCmd(t, f.trainCmd(), f.alice, "")
	if !strings.Contains(f.aOut.String(), "no trainer here") {
		t.Fatalf("expected refusal:\n%s", f.aOut.String())
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
	if !strings.Contains(out, "Lan") {
		t.Fatalf("expected trainer name in line:\n%s", out)
	}
}

func TestTrain_PendingShowsClassLabel(t *testing.T) {
	// XP 1500 → level 2; classTotal 1 → 1 pending.
	f := newTrainFixture(t, "armsman", 1500, 1)

	runCmd(t, f.trainCmd(), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "Armsman") {
		t.Fatalf("expected catalog Class.Name 'Armsman' in line:\n%s", out)
	}
	if !strings.Contains(out, "still being mapped") {
		t.Fatalf("slice 2 stub message missing:\n%s", out)
	}
}

func TestTrain_UnknownClassFallsBackToID(t *testing.T) {
	// Trainer pointed at a class id that's NOT in the chargen catalog.
	// Slice 2 logs and degrades to the raw id.
	f := newTrainFixture(t, "ghoulkin", 1500, 1)

	runCmd(t, f.trainCmd(), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "ghoulkin") {
		t.Fatalf("expected fallback class id 'ghoulkin':\n%s", out)
	}
}

func TestTrain_NilCatalogDegrades(t *testing.T) {
	f := newTrainFixture(t, "armsman", 1500, 1)

	cmd := NewTrain(f.characters, f.mobs, f.templates, f.trainers, nil)
	runCmd(t, cmd, f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "armsman") {
		t.Fatalf("expected raw class id when catalog is nil:\n%s", out)
	}
}

func TestTrain_DoesNotMutate(t *testing.T) {
	f := newTrainFixture(t, "armsman", 1500, 1)

	before, _ := f.characters.FindByName(context.Background(), "Alice")
	runCmd(t, f.trainCmd(), f.alice, "")
	after, _ := f.characters.FindByName(context.Background(), "Alice")

	if after.XP != before.XP {
		t.Fatalf("slice 2 must not mutate XP: before=%d after=%d", before.XP, after.XP)
	}
	if after.ClassLevels[creature.ClassArmsman] != before.ClassLevels[creature.ClassArmsman] {
		t.Fatalf("slice 2 must not mutate ClassLevels")
	}
	if got := progression.LevelForXP(after.XP); got != 2 {
		t.Fatalf("XP curve sanity: LevelForXP=%d, want 2", got)
	}
}
