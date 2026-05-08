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
	return NewLearn(f.characters, f.cat, f.audits)
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
