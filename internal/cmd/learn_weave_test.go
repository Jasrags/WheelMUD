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

type weaveFixture struct {
	characters *repo.MemoryCharacterRepo
	audits     *repo.MemoryAdminAuditRepo
	cat        *chargen.Catalog
	alice      *telnet.Session
	aOut       *bufConn
}

func newWeaveFixture(t *testing.T, pending int32, channeler bool, affinities creature.PowerSet) *weaveFixture {
	t.Helper()
	chars := repo.NewMemoryCharacterRepo()
	c := repo.Character{
		AccountID:     100,
		Name:          "Moiraine",
		CurrentRoomID: 1,
		ClassLevels:   map[creature.Class]int8{creature.ClassInitiate: 1},
		PendingWeaves: pending,
	}
	if channeler {
		c.Channeling = &creature.Channeling{
			ChannelerType: creature.ChannelerInitiate,
			Affinities:    affinities,
		}
	} else {
		c.ClassLevels = map[creature.Class]int8{creature.ClassArmsman: 1}
	}
	if _, err := chars.Create(context.Background(), c); err != nil {
		t.Fatalf("seed: %v", err)
	}
	audits := repo.NewMemoryAdminAuditRepo()
	fsys, _ := chargen.SourceFS()
	cat, err := chargen.Load(fsys)
	if err != nil {
		t.Fatalf("load chargen catalog: %v", err)
	}
	sessions := session.NewRegistry()
	alice, aConn := bufSession(t)
	alice.AccountID = 100
	alice.AuthLevel = telnet.AuthPlayer
	alice.CharacterID = 1
	alice.CharacterName = "Moiraine"
	alice.CurrentRoomID = 1
	sessions.Bind(alice.AccountID, alice)
	return &weaveFixture{
		characters: chars,
		audits:     audits,
		cat:        cat,
		alice:      alice,
		aOut:       aConn,
	}
}

func (f *weaveFixture) cmd() *telnet.Command {
	return NewLearn(f.characters, f.cat, f.audits, nil, nil, nil)
}

// findWeaveByPower returns the first catalog weave matching power.
func findWeaveByPower(t *testing.T, cat *chargen.Catalog, power string) *chargen.Weave {
	t.Helper()
	for _, w := range cat.Weaves() {
		if strings.EqualFold(w.Power, power) {
			return w
		}
	}
	t.Fatalf("no weave with power %q", power)
	return nil
}

func TestLearnWeave_NonChannelerRefused(t *testing.T) {
	f := newWeaveFixture(t, 1, false, 0)
	runCmd(t, f.cmd(), f.alice, "weave")
	out := f.aOut.String()
	if !strings.Contains(out, "cannot weave") {
		t.Fatalf("expected non-channeler refusal:\n%s", out)
	}
}

func TestLearnWeave_BareMenuListsAffinityWeaves(t *testing.T) {
	f := newWeaveFixture(t, 1, true,
		creature.PowerSet(1<<creature.PowerFire))
	runCmd(t, f.cmd(), f.alice, "weave")
	out := f.aOut.String()
	if !strings.Contains(out, "Weaves available") {
		t.Fatalf("menu missing weaves-available row:\n%s", out)
	}
	if !strings.Contains(out, "Affinities") || !strings.Contains(out, "Fire") {
		t.Fatalf("menu missing Fire affinity:\n%s", out)
	}
}

func TestLearnWeave_HappyPathAppendsAndDecrements(t *testing.T) {
	f := newWeaveFixture(t, 2, true,
		creature.PowerSet(1<<creature.PowerFire))
	w := findWeaveByPower(t, f.cat, "Fire")
	runCmd(t, f.cmd(), f.alice, "weave "+w.ID)
	got, _ := f.characters.FindByName(context.Background(), "Moiraine")
	if got.PendingWeaves != 1 {
		t.Errorf("PendingWeaves = %d, want 1", got.PendingWeaves)
	}
	if got.Channeling == nil {
		t.Fatalf("Channeling cleared after spend")
	}
	found := false
	for _, id := range got.Channeling.WeavesKnownIDs {
		if id == w.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("WeavesKnownIDs = %v, missing %q", got.Channeling.WeavesKnownIDs, w.ID)
	}
}

func TestLearnWeave_OffAffinityRefused(t *testing.T) {
	// Affinity = Fire only; pick a Water weave → should miss the
	// allowed-list and refuse with "No such weave" (the menu filter
	// hides the option entirely; trying its id directly still
	// refuses because matchWeaveToken only knows allowed entries).
	f := newWeaveFixture(t, 1, true,
		creature.PowerSet(1<<creature.PowerFire))
	w := findWeaveByPower(t, f.cat, "Water")
	runCmd(t, f.cmd(), f.alice, "weave "+w.ID)
	out := f.aOut.String()
	if !strings.Contains(out, "No such weave") {
		t.Fatalf("expected off-affinity refusal:\n%s", out)
	}
	got, _ := f.characters.FindByName(context.Background(), "Moiraine")
	if got.PendingWeaves != 1 {
		t.Errorf("refusal mutated pending: %d", got.PendingWeaves)
	}
}

func TestLearnWeave_DuplicateRefused(t *testing.T) {
	f := newWeaveFixture(t, 2, true,
		creature.PowerSet(1<<creature.PowerFire))
	w := findWeaveByPower(t, f.cat, "Fire")
	runCmd(t, f.cmd(), f.alice, "weave "+w.ID)
	f.aOut.Reset()
	runCmd(t, f.cmd(), f.alice, "weave "+w.ID)
	out := f.aOut.String()
	if !strings.Contains(strings.ToLower(out), "already know") {
		t.Fatalf("expected dup refusal:\n%s", out)
	}
	got, _ := f.characters.FindByName(context.Background(), "Moiraine")
	if got.PendingWeaves != 1 {
		t.Errorf("dup refusal mutated pending: %d", got.PendingWeaves)
	}
}

func TestLearnWeave_EmptyPoolRefused(t *testing.T) {
	f := newWeaveFixture(t, 0, true,
		creature.PowerSet(1<<creature.PowerFire))
	w := findWeaveByPower(t, f.cat, "Fire")
	runCmd(t, f.cmd(), f.alice, "weave "+w.ID)
	out := f.aOut.String()
	if !strings.Contains(out, "No weaves to learn") {
		t.Fatalf("expected empty-pool refusal:\n%s", out)
	}
	got, _ := f.characters.FindByName(context.Background(), "Moiraine")
	if got.PendingWeaves != 0 {
		t.Errorf("empty-pool refusal mutated pending: %d", got.PendingWeaves)
	}
	if got.Channeling != nil && len(got.Channeling.WeavesKnownIDs) != 0 {
		t.Errorf("empty-pool refusal mutated WeavesKnownIDs: %v", got.Channeling.WeavesKnownIDs)
	}
}

// teacherFixture extends weaveFixture with a weave-teacher seeded
// in the same room as the player. Phase E #28 mid-game path tests.
type teacherFixture struct {
	*weaveFixture
	mobs     *repo.MemoryMobInstanceRepo
	tpls     *repo.MemoryMobTemplateRepo
	teachers *repo.MemoryWeaveTeacherRepo
}

func newTeacherFixture(t *testing.T, pp int32, knownAffinity creature.PowerSet,
	teacherMaxLevel int8, teacherFilter creature.PowerSet,
) *teacherFixture {
	t.Helper()
	wf := newWeaveFixture(t, 0, true, knownAffinity)
	// Stamp PracticePoints onto the seeded character.
	got, _ := wf.characters.FindByName(context.Background(), "Moiraine")
	if err := wf.characters.RecordLevelUp(context.Background(), got.ID,
		repo.LevelUpFields{
			ClassLevels:         got.ClassLevels,
			HPCurrent:           got.Core.HPCurrent,
			HPMax:               got.Core.HPMax,
			BAB:                 got.Core.BAB,
			Saves:               got.Core.Saves,
			PracticePointsDelta: pp,
		}); err != nil {
		t.Fatalf("seed PP: %v", err)
	}

	tpls := repo.NewMemoryMobTemplateRepo()
	tpl, err := tpls.Create(context.Background(), creature.MobTemplate{
		ExternalID: "tr.master_hesper",
		Core:       creature.Core{Name: "Master Hesper", HPMax: 1},
	})
	if err != nil {
		t.Fatalf("seed template: %v", err)
	}
	mobs := repo.NewMemoryMobInstanceRepo()
	mob, err := mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: tpl.ID,
		Core:       creature.Core{Name: "Master Hesper", CurrentRoomID: 1},
	})
	if err != nil {
		t.Fatalf("seed mob: %v", err)
	}
	if err := mobs.UpdateRoom(context.Background(), mob.ID, 1); err != nil {
		t.Fatalf("set mob room: %v", err)
	}
	teachers := repo.NewMemoryWeaveTeacherRepo()
	if _, err := teachers.Create(context.Background(), repo.WeaveTeacher{
		MobTemplateID:  tpl.ID,
		MaxLevelTaught: teacherMaxLevel,
		AffinityFilter: teacherFilter,
	}); err != nil {
		t.Fatalf("seed teacher: %v", err)
	}
	return &teacherFixture{weaveFixture: wf, mobs: mobs, tpls: tpls, teachers: teachers}
}

func (f *teacherFixture) cmd() *telnet.Command {
	return NewLearn(f.characters, f.cat, f.audits, f.mobs, f.tpls, f.teachers)
}

func TestLearnWeave_TeacherMenuShowsPracticePoints(t *testing.T) {
	f := newTeacherFixture(t, 3, creature.PowerSet(1<<creature.PowerFire), 0, 0)
	runCmd(t, f.cmd(), f.alice, "weave")
	out := f.aOut.String()
	if !strings.Contains(out, "Teacher") || !strings.Contains(out, "Master Hesper") {
		t.Fatalf("teacher byline missing:\n%s", out)
	}
	if !strings.Contains(out, "Practice points") {
		t.Fatalf("practice-points line missing:\n%s", out)
	}
	if !strings.Contains(out, "cost") {
		t.Fatalf("per-weave cost column missing:\n%s", out)
	}
}

func TestLearnWeave_TeacherHappyPathDrainsPP(t *testing.T) {
	f := newTeacherFixture(t, 3, creature.PowerSet(1<<creature.PowerFire), 0, 0)
	w := findWeaveByPower(t, f.cat, "Fire")
	runCmd(t, f.cmd(), f.alice, "weave "+w.ID)

	got, _ := f.characters.FindByName(context.Background(), "Moiraine")
	if got.PracticePoints != 2 {
		t.Errorf("PracticePoints = %d, want 2 (3 - cost 1)", got.PracticePoints)
	}
	if got.PendingWeaves != 0 {
		t.Errorf("teacher path leaked into pending_weaves: %d", got.PendingWeaves)
	}
	found := false
	for _, id := range got.Channeling.WeavesKnownIDs {
		if id == w.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("WeavesKnownIDs missing %q: %v", w.ID, got.Channeling.WeavesKnownIDs)
	}
	rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10})
	if len(rows) != 1 || !strings.Contains(rows[0].Args, "kind=weave_study") {
		t.Fatalf("audit row: %+v", rows)
	}
	if !strings.Contains(rows[0].Args, "cost=1") {
		t.Errorf("audit args missing cost: %s", rows[0].Args)
	}
}

func TestLearnWeave_TeacherInsufficientPPRefused(t *testing.T) {
	f := newTeacherFixture(t, 0, creature.PowerSet(1<<creature.PowerFire), 0, 0)
	w := findWeaveByPower(t, f.cat, "Fire")
	runCmd(t, f.cmd(), f.alice, "weave "+w.ID)
	out := f.aOut.String()
	if !strings.Contains(out, "practice points") {
		t.Fatalf("expected PP refusal:\n%s", out)
	}
	got, _ := f.characters.FindByName(context.Background(), "Moiraine")
	if got.Channeling != nil && len(got.Channeling.WeavesKnownIDs) != 0 {
		t.Errorf("refusal mutated WeavesKnownIDs: %v", got.Channeling.WeavesKnownIDs)
	}
	rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10})
	if len(rows) != 0 {
		t.Errorf("refusal must not audit: %+v", rows)
	}
}

func TestLearnWeave_TeacherFilterExcludesPower(t *testing.T) {
	// Teacher only teaches Air; channeler has Fire affinity. The Fire
	// weave should drop out of the teacher-filtered allowed list and
	// the typed id should refuse.
	f := newTeacherFixture(t, 5,
		creature.PowerSet(1<<creature.PowerFire),
		0, creature.PowerSet(1<<creature.PowerAir))
	w := findWeaveByPower(t, f.cat, "Fire")
	runCmd(t, f.cmd(), f.alice, "weave "+w.ID)
	out := f.aOut.String()
	if !strings.Contains(out, "No such weave") {
		t.Fatalf("expected teacher-filter refusal:\n%s", out)
	}
}

func TestLearnWeave_TeacherDuplicateRefused(t *testing.T) {
	f := newTeacherFixture(t, 5, creature.PowerSet(1<<creature.PowerFire), 0, 0)
	w := findWeaveByPower(t, f.cat, "Fire")
	runCmd(t, f.cmd(), f.alice, "weave "+w.ID) // first study
	f.aOut.Reset()
	runCmd(t, f.cmd(), f.alice, "weave "+w.ID) // duplicate
	out := f.aOut.String()
	if !strings.Contains(strings.ToLower(out), "already know") {
		t.Fatalf("expected dup refusal:\n%s", out)
	}
	got, _ := f.characters.FindByName(context.Background(), "Moiraine")
	if got.PracticePoints != 4 {
		// 5 → 4 after the first study; the duplicate must not spend.
		t.Errorf("PracticePoints = %d, want 4 (one spend, no double)", got.PracticePoints)
	}
}

func TestLearnWeave_NoTeacherUsesPendingPath(t *testing.T) {
	// Regression: with the new factory args wired but no teacher in
	// the room, the verb must still drain pending_weaves verbatim.
	f := newWeaveFixture(t, 1, true, creature.PowerSet(1<<creature.PowerFire))
	mobs := repo.NewMemoryMobInstanceRepo()
	tpls := repo.NewMemoryMobTemplateRepo()
	teachers := repo.NewMemoryWeaveTeacherRepo()
	cmd := NewLearn(f.characters, f.cat, f.audits, mobs, tpls, teachers)
	w := findWeaveByPower(t, f.cat, "Fire")
	runCmd(t, cmd, f.alice, "weave "+w.ID)
	got, _ := f.characters.FindByName(context.Background(), "Moiraine")
	if got.PendingWeaves != 0 {
		t.Errorf("PendingWeaves = %d, want 0", got.PendingWeaves)
	}
}

func TestLearnWeave_AuditOnSuccessOnly(t *testing.T) {
	f := newWeaveFixture(t, 1, true,
		creature.PowerSet(1<<creature.PowerFire))
	runCmd(t, f.cmd(), f.alice, "weave nonsense") // refusal
	if rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10}); len(rows) != 0 {
		t.Fatalf("refusal must not audit: %d rows", len(rows))
	}
	w := findWeaveByPower(t, f.cat, "Fire")
	runCmd(t, f.cmd(), f.alice, "weave "+w.ID)
	rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10})
	if len(rows) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(rows))
	}
	if rows[0].Verb != "learn" || rows[0].Target != w.ID {
		t.Errorf("audit row = %+v, want verb=learn target=%s", rows[0], w.ID)
	}
	if !strings.Contains(rows[0].Args, "kind=weave") {
		t.Errorf("args = %q, want kind=weave", rows[0].Args)
	}
}
