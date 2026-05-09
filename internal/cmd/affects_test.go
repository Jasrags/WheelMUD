package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/affects"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// affectsFixture seeds an admin + player session bound to a shared
// chars repo whose rows match the session ids. Mirrors adminPair's
// shape but persists characters so RecordAffects / GetByID round-
// trip through the repo.
func affectsFixture(t *testing.T) (
	chars *repo.MemoryCharacterRepo,
	sessions *session.Registry,
	admin, player *telnet.Session,
	aOut, pOut *bufConn,
	audits *repo.MemoryAdminAuditRepo,
) {
	t.Helper()
	chars = repo.NewMemoryCharacterRepo()
	ctx := context.Background()
	if _, err := chars.Create(ctx, repo.Character{
		AccountID:     100,
		Name:          "Admin",
		CurrentRoomID: 1,
		AuthLevel:     uint8(telnet.AuthAdmin),
	}); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if _, err := chars.Create(ctx, repo.Character{
		AccountID:     200,
		Name:          "Bob",
		CurrentRoomID: 1,
		AuthLevel:     uint8(telnet.AuthPlayer),
	}); err != nil {
		t.Fatalf("seed bob: %v", err)
	}
	sessions = session.NewRegistry()

	a, aConn := bufSession(t)
	a.AccountID = 100
	a.AuthLevel = telnet.AuthAdmin
	a.CharacterID = 1
	a.CharacterName = "Admin"
	a.CurrentRoomID = 1
	sessions.Bind(a.AccountID, a)

	p, pConn := bufSession(t)
	p.AccountID = 200
	p.AuthLevel = telnet.AuthPlayer
	p.CharacterID = 2
	p.CharacterName = "Bob"
	p.CurrentRoomID = 1
	sessions.Bind(p.AccountID, p)

	return chars, sessions, a, p, aConn, pConn, repo.NewMemoryAdminAuditRepo()
}

func TestAffect_HappyPath(t *testing.T) {
	chars, sessions, admin, _, aOut, _, audits := affectsFixture(t)

	runCmd(t, NewAffect(chars, sessions, audits), admin, "Bob blessed Saves.Will=+1 Defense=+2 duration=20")

	bob, err := chars.GetByID(context.Background(), 2)
	if err != nil {
		t.Fatalf("get bob: %v", err)
	}
	if len(bob.Core.Affects) != 1 {
		t.Fatalf("want 1 affect, got %d: %+v", len(bob.Core.Affects), bob.Core.Affects)
	}
	a := bob.Core.Affects[0]
	if a.Name != "blessed" || a.Source != adminAffectSource || a.DurationTicks != 20 {
		t.Fatalf("unexpected affect: %+v", a)
	}
	if len(a.Modifiers) != 2 {
		t.Fatalf("want 2 mods, got %+v", a.Modifiers)
	}
	if !strings.Contains(aOut.String(), "Applied blessed to Bob") {
		t.Fatalf("admin ack missing: %s", aOut.String())
	}
	rows := auditList(t, audits)
	if len(rows) != 1 || rows[0].Verb != "affect" || rows[0].Target != "Bob" {
		t.Fatalf("audit row mismatch: %+v", rows)
	}
}

func TestAffect_DefaultDuration(t *testing.T) {
	chars, sessions, admin, _, _, _, audits := affectsFixture(t)

	runCmd(t, NewAffect(chars, sessions, audits), admin, "Bob shielded Defense=+1")

	bob, _ := chars.GetByID(context.Background(), 2)
	if bob.Core.Affects[0].DurationTicks != 10 {
		t.Fatalf("want default 10 ticks, got %d", bob.Core.Affects[0].DurationTicks)
	}
}

func TestAffect_UnknownFieldRefuses(t *testing.T) {
	chars, sessions, admin, _, aOut, _, audits := affectsFixture(t)

	runCmd(t, NewAffect(chars, sessions, audits), admin, "Bob bug str.fake=+99")

	bob, _ := chars.GetByID(context.Background(), 2)
	if len(bob.Core.Affects) != 0 {
		t.Fatalf("affect persisted on refusal: %+v", bob.Core.Affects)
	}
	if !strings.Contains(aOut.String(), "unknown field") {
		t.Fatalf("refusal text missing: %s", aOut.String())
	}
	if !strings.Contains(aOut.String(), "Fields:") {
		t.Fatalf("allow-list hint missing: %s", aOut.String())
	}
	if rows := auditList(t, audits); len(rows) != 0 {
		t.Fatalf("refusal wrote audit row(s): %+v", rows)
	}
}

func TestAffect_OfflineTargetRefuses(t *testing.T) {
	chars, sessions, admin, _, aOut, _, audits := affectsFixture(t)

	runCmd(t, NewAffect(chars, sessions, audits), admin, "Ghost blessed Defense=+1")

	if !strings.Contains(aOut.String(), "No such player online") {
		t.Fatalf("refusal missing: %s", aOut.String())
	}
	if rows := auditList(t, audits); len(rows) != 0 {
		t.Fatalf("refusal wrote audit row(s): %+v", rows)
	}
}

func TestDispel_NamedAffect(t *testing.T) {
	chars, sessions, admin, _, aOut, _, audits := affectsFixture(t)
	// Seed two affects directly.
	seed := []creature.Affect{
		{Source: adminAffectSource, Name: "blessed", DurationTicks: 5},
		{Source: adminAffectSource, Name: "haste", DurationTicks: 5},
	}
	if err := chars.RecordAffects(context.Background(), 2, seed); err != nil {
		t.Fatalf("seed affects: %v", err)
	}

	runCmd(t, NewDispel(chars, sessions, audits), admin, "Bob blessed")

	bob, _ := chars.GetByID(context.Background(), 2)
	if len(bob.Core.Affects) != 1 || bob.Core.Affects[0].Name != "haste" {
		t.Fatalf("dispel did not drop blessed: %+v", bob.Core.Affects)
	}
	if !strings.Contains(aOut.String(), "Cleared blessed from Bob") {
		t.Fatalf("ack missing: %s", aOut.String())
	}
	rows := auditList(t, audits)
	if len(rows) != 1 || rows[0].Verb != "dispel" {
		t.Fatalf("audit mismatch: %+v", rows)
	}
}

func TestDispel_All(t *testing.T) {
	chars, sessions, admin, _, _, _, audits := affectsFixture(t)
	seed := []creature.Affect{
		{Source: adminAffectSource, Name: "blessed", DurationTicks: 5},
		{Source: adminAffectSource, Name: "haste", DurationTicks: 5},
	}
	_ = chars.RecordAffects(context.Background(), 2, seed)

	runCmd(t, NewDispel(chars, sessions, audits), admin, "Bob")

	bob, _ := chars.GetByID(context.Background(), 2)
	if len(bob.Core.Affects) != 0 {
		t.Fatalf("dispel-all did not clear: %+v", bob.Core.Affects)
	}
	if rows := auditList(t, audits); len(rows) != 1 {
		t.Fatalf("audit mismatch: %+v", rows)
	}
}

func TestDispel_NoMatchIsNoOp(t *testing.T) {
	chars, sessions, admin, _, aOut, _, audits := affectsFixture(t)
	// Bob has no affects.

	runCmd(t, NewDispel(chars, sessions, audits), admin, "Bob blessed")

	if !strings.Contains(aOut.String(), "no matching affect") {
		t.Fatalf("no-op refusal missing: %s", aOut.String())
	}
	if rows := auditList(t, audits); len(rows) != 0 {
		t.Fatalf("no-op wrote audit row(s): %+v", rows)
	}
}

func TestAffects_ListsLiveAffects(t *testing.T) {
	chars, _, _, player, _, pOut, _ := affectsFixture(t)
	seed := []creature.Affect{{
		Source:        adminAffectSource,
		Name:          "blessed",
		Modifiers:     []creature.StatMod{{Field: affects.FieldSavesWill, Delta: 1}},
		DurationTicks: 4,
	}}
	if err := chars.RecordAffects(context.Background(), 2, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	runCmd(t, NewAffects(chars), player, "")

	out := pOut.String()
	if !strings.Contains(out, "blessed") || !strings.Contains(out, "Saves.Will +1") {
		t.Fatalf("listing missing fields: %s", out)
	}
	// 4 ticks * 30s = 120s
	if !strings.Contains(out, "~120s left") {
		t.Fatalf("duration readout missing: %s", out)
	}
	if !strings.Contains(out, "[admin]") {
		t.Fatalf("source label missing: %s", out)
	}
}

func TestAffects_EmptyList(t *testing.T) {
	chars, _, _, player, _, pOut, _ := affectsFixture(t)

	runCmd(t, NewAffects(chars), player, "")

	if !strings.Contains(pOut.String(), "no affects") {
		t.Fatalf("empty marker missing: %s", pOut.String())
	}
}
