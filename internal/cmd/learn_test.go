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

// learnFixture seeds Alice as Armsman L3 with a chosen pending-skill-
// point pool and an empty Skills map. Cap is L3+3 = 6.
type learnFixture struct {
	characters *repo.MemoryCharacterRepo
	audits     *repo.MemoryAdminAuditRepo
	cat        *chargen.Catalog
	alice      *telnet.Session
	aOut       *bufConn
}

func newLearnFixture(t *testing.T, pending int32, level int8) *learnFixture {
	t.Helper()

	chars := repo.NewMemoryCharacterRepo()
	if _, err := chars.Create(context.Background(), repo.Character{
		AccountID:          100,
		Name:               "Alice",
		CurrentRoomID:      1,
		ClassLevels:        map[creature.Class]int8{creature.ClassArmsman: level},
		PendingSkillPoints: pending,
	}); err != nil {
		t.Fatalf("seed alice: %v", err)
	}

	audits := repo.NewMemoryAdminAuditRepo()

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

	return &learnFixture{
		characters: chars,
		audits:     audits,
		cat:        cat,
		alice:      alice,
		aOut:       aConn,
	}
}

func (f *learnFixture) learnCmd() *telnet.Command {
	return NewLearn(f.characters, f.cat, f.audits)
}

func TestLearn_BareMenuListsAllowedSkills(t *testing.T) {
	f := newLearnFixture(t, 5, 3)
	runCmd(t, f.learnCmd(), f.alice, "")
	out := f.aOut.String()
	// Armsman class skills include "climb" and "swim".
	if !strings.Contains(out, "Climb") || !strings.Contains(out, "Swim") {
		t.Fatalf("menu missing armsman class skills:\n%s", out)
	}
	if !strings.Contains(out, "Skill points") || !strings.Contains(out, "5") {
		t.Fatalf("menu missing skill-point count:\n%s", out)
	}
}

func TestLearn_DefaultRanksOne(t *testing.T) {
	f := newLearnFixture(t, 5, 3)
	runCmd(t, f.learnCmd(), f.alice, "climb")
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	key := chargen.HashID("climb")
	if got.Skills[key].Ranks != 1 {
		t.Errorf("climb ranks = %d, want 1", got.Skills[key].Ranks)
	}
	if got.PendingSkillPoints != 4 {
		t.Errorf("PendingSkillPoints = %d, want 4", got.PendingSkillPoints)
	}
	if !got.Skills[key].IsClassSkill {
		t.Error("class skill flag not set")
	}
}

func TestLearn_ExplicitRanksDecrementsByCost(t *testing.T) {
	f := newLearnFixture(t, 5, 3)
	runCmd(t, f.learnCmd(), f.alice, "climb 3")
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	key := chargen.HashID("climb")
	if got.Skills[key].Ranks != 3 {
		t.Errorf("climb ranks = %d, want 3", got.Skills[key].Ranks)
	}
	if got.PendingSkillPoints != 2 {
		t.Errorf("PendingSkillPoints = %d, want 2", got.PendingSkillPoints)
	}
}

func TestLearn_AccumulatesAcrossCalls(t *testing.T) {
	f := newLearnFixture(t, 5, 3)
	runCmd(t, f.learnCmd(), f.alice, "climb 2")
	runCmd(t, f.learnCmd(), f.alice, "climb")
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	key := chargen.HashID("climb")
	if got.Skills[key].Ranks != 3 {
		t.Errorf("climb ranks = %d, want 3", got.Skills[key].Ranks)
	}
	if got.PendingSkillPoints != 2 {
		t.Errorf("PendingSkillPoints = %d, want 2", got.PendingSkillPoints)
	}
}

func TestLearn_CapRefusalDoesNotMutate(t *testing.T) {
	// L3 → cap 6. Buying 7 ranks must refuse.
	f := newLearnFixture(t, 10, 3)
	runCmd(t, f.learnCmd(), f.alice, "climb 7")
	out := f.aOut.String()
	if !strings.Contains(out, "cap of 6") {
		t.Fatalf("expected cap refusal:\n%s", out)
	}
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	key := chargen.HashID("climb")
	if got.Skills[key].Ranks != 0 {
		t.Errorf("cap refusal mutated ranks: %d", got.Skills[key].Ranks)
	}
	if got.PendingSkillPoints != 10 {
		t.Errorf("cap refusal mutated pending: %d", got.PendingSkillPoints)
	}
}

func TestLearn_BudgetRefusalDoesNotMutate(t *testing.T) {
	// 2 pts available, asking for 3 → refused.
	f := newLearnFixture(t, 2, 3)
	runCmd(t, f.learnCmd(), f.alice, "climb 3")
	out := f.aOut.String()
	if !strings.Contains(out, "Not enough skill points") {
		t.Fatalf("expected budget refusal:\n%s", out)
	}
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.PendingSkillPoints != 2 {
		t.Errorf("budget refusal mutated pending: %d", got.PendingSkillPoints)
	}
}

func TestLearn_CrossClassRefusal(t *testing.T) {
	// "stealth" is NOT an armsman class skill.
	f := newLearnFixture(t, 5, 3)
	runCmd(t, f.learnCmd(), f.alice, "stealth")
	out := f.aOut.String()
	if !strings.Contains(out, "not available") {
		t.Fatalf("expected cross-class refusal:\n%s", out)
	}
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.PendingSkillPoints != 5 {
		t.Errorf("refusal mutated pending: %d", got.PendingSkillPoints)
	}
}

func TestLearn_UnknownTokenRefusal(t *testing.T) {
	f := newLearnFixture(t, 5, 3)
	runCmd(t, f.learnCmd(), f.alice, "frobnicate")
	out := f.aOut.String()
	if !strings.Contains(out, "not available") {
		t.Fatalf("expected unknown-skill refusal:\n%s", out)
	}
}

func TestLearn_AuditOnSuccessOnly(t *testing.T) {
	f := newLearnFixture(t, 5, 3)
	runCmd(t, f.learnCmd(), f.alice, "stealth") // refusal — must not audit
	if rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10}); len(rows) != 0 {
		t.Fatalf("refusal must not audit: %d rows", len(rows))
	}
	runCmd(t, f.learnCmd(), f.alice, "climb 2")
	rows, err := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(rows))
	}
	if rows[0].Verb != "learn" {
		t.Errorf("verb = %q, want \"learn\"", rows[0].Verb)
	}
	if rows[0].Target != "climb" {
		t.Errorf("target = %q, want \"climb\"", rows[0].Target)
	}
	if rows[0].Args != "ranks=2" {
		t.Errorf("args = %q, want \"ranks=2\"", rows[0].Args)
	}
}

func TestLearn_NumericMenuTokenResolves(t *testing.T) {
	f := newLearnFixture(t, 5, 3)
	// Bare menu first to know the order; allowed = sorted armsman
	// class skills. We don't assert which is "1" because order is
	// alphabetical and stable; just verify a numeric pick increments
	// SOME class skill by 1 and decrements pending.
	runCmd(t, f.learnCmd(), f.alice, "1")
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.PendingSkillPoints != 4 {
		t.Errorf("PendingSkillPoints = %d, want 4 (numeric pick)", got.PendingSkillPoints)
	}
	if len(got.Skills) != 1 {
		t.Errorf("Skills = %d entries, want 1", len(got.Skills))
	}
}

func TestLearn_InfoIsReadOnly(t *testing.T) {
	f := newLearnFixture(t, 5, 3)
	runCmd(t, f.learnCmd(), f.alice, "info climb")
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.PendingSkillPoints != 5 {
		t.Errorf("info mutated pending: %d", got.PendingSkillPoints)
	}
	if len(got.Skills) != 0 {
		t.Errorf("info mutated skills: %+v", got.Skills)
	}
}
