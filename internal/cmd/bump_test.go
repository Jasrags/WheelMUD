package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

type bumpFixture struct {
	characters *repo.MemoryCharacterRepo
	audits     *repo.MemoryAdminAuditRepo
	alice      *telnet.Session
	aOut       *bufConn
}

func newBumpFixture(t *testing.T, pending int32, str int8) *bumpFixture {
	t.Helper()
	chars := repo.NewMemoryCharacterRepo()
	seed := repo.Character{
		AccountID:           100,
		Name:                "Alice",
		CurrentRoomID:       1,
		ClassLevels:         map[creature.Class]int8{creature.ClassArmsman: 4},
		PendingAbilityBumps: pending,
	}
	seed.Core.Abilities.Str.Current = str
	seed.Core.Abilities.Dex.Current = 12
	if _, err := chars.Create(context.Background(), seed); err != nil {
		t.Fatalf("seed alice: %v", err)
	}
	audits := repo.NewMemoryAdminAuditRepo()
	sessions := session.NewRegistry()
	alice, aConn := bufSession(t)
	alice.AccountID = 100
	alice.AuthLevel = telnet.AuthPlayer
	alice.CharacterID = 1
	alice.CharacterName = "Alice"
	alice.CurrentRoomID = 1
	sessions.Bind(alice.AccountID, alice)
	return &bumpFixture{characters: chars, audits: audits, alice: alice, aOut: aConn}
}

func (f *bumpFixture) cmd() *telnet.Command {
	return NewBump(f.characters, f.audits)
}

func TestBump_BareMenuShowsScores(t *testing.T) {
	f := newBumpFixture(t, 1, 14)
	runCmd(t, f.cmd(), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "Bumps available") || !strings.Contains(out, "1") {
		t.Fatalf("menu missing bump count:\n%s", out)
	}
	if !strings.Contains(out, "Strength") || !strings.Contains(out, "Dexterity") {
		t.Fatalf("menu missing ability rows:\n%s", out)
	}
}

func TestBump_HappyPath(t *testing.T) {
	f := newBumpFixture(t, 2, 14)
	runCmd(t, f.cmd(), f.alice, "str")
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.Core.Abilities.Str.Current != 15 {
		t.Errorf("Str = %d, want 15", got.Core.Abilities.Str.Current)
	}
	if got.PendingAbilityBumps != 1 {
		t.Errorf("PendingAbilityBumps = %d, want 1", got.PendingAbilityBumps)
	}
	if got.Core.Abilities.Dex.Current != 12 {
		t.Errorf("Dex clobbered: %d, want 12", got.Core.Abilities.Dex.Current)
	}
}

func TestBump_FullNameAccepted(t *testing.T) {
	f := newBumpFixture(t, 1, 14)
	runCmd(t, f.cmd(), f.alice, "strength")
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.Core.Abilities.Str.Current != 15 {
		t.Errorf("Str = %d, want 15", got.Core.Abilities.Str.Current)
	}
}

func TestBump_CapAt20Refusal(t *testing.T) {
	f := newBumpFixture(t, 1, 20)
	runCmd(t, f.cmd(), f.alice, "str")
	out := f.aOut.String()
	if !strings.Contains(out, "past 20") {
		t.Fatalf("expected cap refusal:\n%s", out)
	}
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.Core.Abilities.Str.Current != 20 {
		t.Errorf("cap refusal mutated str: %d", got.Core.Abilities.Str.Current)
	}
	if got.PendingAbilityBumps != 1 {
		t.Errorf("cap refusal mutated pending: %d", got.PendingAbilityBumps)
	}
}

func TestBump_EmptyPoolRefusal(t *testing.T) {
	f := newBumpFixture(t, 0, 14)
	runCmd(t, f.cmd(), f.alice, "str")
	out := f.aOut.String()
	if !strings.Contains(out, "No ability bumps") {
		t.Fatalf("expected empty-pool refusal:\n%s", out)
	}
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.Core.Abilities.Str.Current != 14 {
		t.Errorf("empty-pool refusal mutated str: %d", got.Core.Abilities.Str.Current)
	}
	if got.PendingAbilityBumps != 0 {
		t.Errorf("empty-pool refusal mutated pending: %d", got.PendingAbilityBumps)
	}
}

func TestBump_UnknownAbilityRefusal(t *testing.T) {
	f := newBumpFixture(t, 1, 14)
	runCmd(t, f.cmd(), f.alice, "luk")
	out := f.aOut.String()
	if !strings.Contains(strings.ToLower(out), "unknown ability") {
		t.Fatalf("expected unknown refusal:\n%s", out)
	}
	got, _ := f.characters.FindByName(context.Background(), "Alice")
	if got.PendingAbilityBumps != 1 {
		t.Errorf("unknown refusal mutated pending: %d", got.PendingAbilityBumps)
	}
}

func TestBump_AuditOnSuccessOnly(t *testing.T) {
	f := newBumpFixture(t, 1, 20) // cap → refusal first
	runCmd(t, f.cmd(), f.alice, "str")
	if rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10}); len(rows) != 0 {
		t.Fatalf("cap refusal must not audit: %d rows", len(rows))
	}
	g := newBumpFixture(t, 1, 14)
	runCmd(t, g.cmd(), g.alice, "dex")
	rows, _ := g.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10})
	if len(rows) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(rows))
	}
	if rows[0].Verb != "bump" || rows[0].Target != "dex" {
		t.Errorf("audit row = %+v, want verb=bump target=dex", rows[0])
	}
	if !strings.Contains(rows[0].Args, "score=13") {
		t.Errorf("args = %q, want score=13", rows[0].Args)
	}
}
