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

type featFixture struct {
	characters *repo.MemoryCharacterRepo
	audits     *repo.MemoryAdminAuditRepo
	cat        *chargen.Catalog
	alice      *telnet.Session
	aOut       *bufConn
}

func newFeatFixture(t *testing.T, pending int32) *featFixture {
	t.Helper()
	chars := repo.NewMemoryCharacterRepo()
	// Midlander background — has access to Bullheaded, Luck of Heroes,
	// Militia, etc. in the catalog.
	if _, err := chars.Create(context.Background(), repo.Character{
		AccountID:     100,
		Name:          "Alice",
		CurrentRoomID: 1,
		Background:    creature.BackgroundMidlander,
		ClassLevels:   map[creature.Class]int8{creature.ClassArmsman: 3},
		PendingFeats:  pending,
	}); err != nil {
		t.Fatalf("seed alice: %v", err)
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
	alice.CharacterName = "Alice"
	alice.CurrentRoomID = 1
	sessions.Bind(alice.AccountID, alice)
	return &featFixture{
		characters: chars,
		audits:     audits,
		cat:        cat,
		alice:      alice,
		aOut:       aConn,
	}
}

func (f *featFixture) cmd() *telnet.Command {
	return NewFeat(f.characters, f.cat, f.audits)
}

// firstGeneralFeatID returns a feat id available to the Midlander
// background — the catalog ships only background-restricted feats
// today, so tests pick the first one whose Backgrounds list includes
// "midlander".
func firstGeneralFeatID(t *testing.T, cat *chargen.Catalog) string {
	t.Helper()
	for _, f := range cat.Feats() {
		for _, bg := range f.Backgrounds {
			if bg == "midlander" {
				return f.ID
			}
		}
	}
	t.Fatal("no midlander-eligible feat in catalog")
	return ""
}

func TestFeat_BareMenuListsFeats(t *testing.T) {
	f := newFeatFixture(t, 1)
	runCmd(t, f.cmd(), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "Feat slots") || !strings.Contains(out, "1") {
		t.Fatalf("menu missing feat-slots line:\n%s", out)
	}
	if !strings.Contains(out, "Available feats") {
		t.Fatalf("menu missing 'Available feats':\n%s", out)
	}
}

func TestFeat_HappyPathPicksAndDecrements(t *testing.T) {
	f := newFeatFixture(t, 2)
	id := firstGeneralFeatID(t, f.cat)
	runCmd(t, f.cmd(), f.alice, id)
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.PendingFeats != 1 {
		t.Errorf("PendingFeats = %d, want 1", got.PendingFeats)
	}
	wantHash := chargen.HashID(id)
	found := false
	for _, h := range got.Feats {
		if h == wantHash {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("feat hash %d not in Feats: %v", wantHash, got.Feats)
	}
}

func TestFeat_DuplicateRefusalDoesNotMutate(t *testing.T) {
	f := newFeatFixture(t, 2)
	id := firstGeneralFeatID(t, f.cat)
	runCmd(t, f.cmd(), f.alice, id)
	f.aOut.Reset()
	// Second pick of the same feat should refuse.
	runCmd(t, f.cmd(), f.alice, id)
	out := f.aOut.String()
	if !strings.Contains(strings.ToLower(out), "already know") {
		t.Fatalf("expected duplicate refusal:\n%s", out)
	}
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.PendingFeats != 1 {
		t.Errorf("dup refusal mutated pending: %d", got.PendingFeats)
	}
	if len(got.Feats) != 1 {
		t.Errorf("dup refusal grew Feats: %v", got.Feats)
	}
}

func TestFeat_DuplicateBeatsEmptyPool(t *testing.T) {
	// Pool=0 AND feat already known: dup-check must run first so the
	// player gets the informative "already know" message rather than
	// the misleading "no slots".
	f := newFeatFixture(t, 1)
	id := firstGeneralFeatID(t, f.cat)
	runCmd(t, f.cmd(), f.alice, id) // drains pool to 0, learns feat
	f.aOut.Reset()
	runCmd(t, f.cmd(), f.alice, id) // pool is now 0 AND feat is known
	out := f.aOut.String()
	if !strings.Contains(strings.ToLower(out), "already know") {
		t.Fatalf("expected dup message before empty-pool message:\n%s", out)
	}
	if strings.Contains(out, "No feat picks") {
		t.Fatalf("got empty-pool message instead of dup:\n%s", out)
	}
}

func TestFeat_EmptyPoolRefusal(t *testing.T) {
	f := newFeatFixture(t, 0)
	id := firstGeneralFeatID(t, f.cat)
	runCmd(t, f.cmd(), f.alice, id)
	out := f.aOut.String()
	if !strings.Contains(out, "No feat picks") {
		t.Fatalf("expected empty-pool refusal:\n%s", out)
	}
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if len(got.Feats) != 0 {
		t.Errorf("empty-pool refusal mutated feats: %v", got.Feats)
	}
}

func TestFeat_UnknownIDRefusal(t *testing.T) {
	f := newFeatFixture(t, 1)
	runCmd(t, f.cmd(), f.alice, "frobnicate")
	out := f.aOut.String()
	if !strings.Contains(out, "No such feat") {
		t.Fatalf("expected unknown refusal:\n%s", out)
	}
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.PendingFeats != 1 {
		t.Errorf("unknown refusal mutated pending: %d", got.PendingFeats)
	}
}

func TestFeat_AuditOnSuccessOnly(t *testing.T) {
	f := newFeatFixture(t, 1)
	runCmd(t, f.cmd(), f.alice, "frobnicate") // refusal — no audit
	if rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10}); len(rows) != 0 {
		t.Fatalf("refusal must not audit: %d rows", len(rows))
	}
	id := firstGeneralFeatID(t, f.cat)
	runCmd(t, f.cmd(), f.alice, id)
	rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10})
	if len(rows) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(rows))
	}
	if rows[0].Verb != "feat" {
		t.Errorf("verb = %q, want feat", rows[0].Verb)
	}
	if rows[0].Target != id {
		t.Errorf("target = %q, want %q", rows[0].Target, id)
	}
}

func TestFeat_InfoIsReadOnly(t *testing.T) {
	f := newFeatFixture(t, 1)
	id := firstGeneralFeatID(t, f.cat)
	runCmd(t, f.cmd(), f.alice, "info "+id)
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.PendingFeats != 1 {
		t.Errorf("info mutated pending: %d", got.PendingFeats)
	}
	if len(got.Feats) != 0 {
		t.Errorf("info mutated feats: %v", got.Feats)
	}
}
