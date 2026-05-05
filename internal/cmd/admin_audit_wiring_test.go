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

// auditList is the small read helper this file uses to assert on
// repo state. Returning the full slice keeps each test's invariants
// inline (length, verb, target).
func auditList(t *testing.T, r *repo.MemoryAdminAuditRepo) []repo.AdminAuditEntry {
	t.Helper()
	got, err := r.List(context.Background(), repo.AdminAuditFilter{})
	if err != nil {
		t.Fatalf("audit list: %v", err)
	}
	return got
}

func TestAudit_Goto_RecordsOnSuccess(t *testing.T) {
	sessions, admin, _, _, _, rooms, exits, items, mobs, chars := adminPair(t)
	audits := repo.NewMemoryAdminAuditRepo()
	g := NewGoto(rooms, exits, items, mobs, chars, sessions, noonClock(t), audits)

	runCmd(t, g, admin, "Player")

	got := auditList(t, audits)
	if len(got) != 1 || got[0].Verb != "goto" || got[0].Target != "Player" {
		t.Fatalf("expected single goto/Player row; got %+v", got)
	}
	if got[0].ActorName != "Admin" || got[0].ActorCharacterID != 1 {
		t.Fatalf("actor not stamped: %+v", got[0])
	}
}

func TestAudit_Goto_NoRecordOnRefusal(t *testing.T) {
	sessions, admin, player, _, _, rooms, exits, items, mobs, chars := adminPair(t)
	rooms.Insert(repo.Room{ID: 99, Name: "Sealed Vault", Flags: repo.RoomFlags{NoTeleport: true}})
	player.CurrentRoomID = 99
	audits := repo.NewMemoryAdminAuditRepo()
	g := NewGoto(rooms, exits, items, mobs, chars, sessions, noonClock(t), audits)

	runCmd(t, g, admin, "Player")

	if got := auditList(t, audits); len(got) != 0 {
		t.Fatalf("refused goto wrote audit row(s): %+v", got)
	}
}

func TestAudit_Transfer_RecordsOnSuccess(t *testing.T) {
	sessions, admin, _, _, _, rooms, exits, items, mobs, chars := adminPair(t)
	audits := repo.NewMemoryAdminAuditRepo()
	tr := NewTransfer(rooms, exits, items, mobs, chars, sessions, noonClock(t), audits)

	runCmd(t, tr, admin, "Player")

	got := auditList(t, audits)
	if len(got) != 1 || got[0].Verb != "transfer" || got[0].Target != "Player" {
		t.Fatalf("expected transfer/Player row; got %+v", got)
	}
}

func TestAudit_Summon_RecordsOnSuccess(t *testing.T) {
	sessions, admin, _, _, _, rooms, exits, items, mobs, chars := adminPair(t)
	audits := repo.NewMemoryAdminAuditRepo()
	s := NewSummon(rooms, exits, items, mobs, chars, sessions, noonClock(t), audits)

	runCmd(t, s, admin, "Player")

	got := auditList(t, audits)
	if len(got) != 1 || got[0].Verb != "summon" || got[0].Target != "Player" {
		t.Fatalf("expected summon/Player row; got %+v", got)
	}
}

func TestAudit_Teleport_RecordsBothForms(t *testing.T) {
	sessions, admin, _, _, _, rooms, exits, items, mobs, chars := adminPair(t)
	audits := repo.NewMemoryAdminAuditRepo()
	tp := NewTeleport(rooms, exits, items, mobs, chars, sessions, noonClock(t), audits)

	runCmd(t, tp, admin, "2")        // self form
	runCmd(t, tp, admin, "Player 1") // other form

	got := auditList(t, audits)
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d: %+v", len(got), got)
	}
	for _, e := range got {
		if e.Verb != "teleport" {
			t.Fatalf("unexpected verb: %+v", e)
		}
	}
}

func TestAudit_Wizinvis_RecordsToggleState(t *testing.T) {
	s, _ := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	s.CharacterID = 42
	s.CharacterName = "Lan"
	audits := repo.NewMemoryAdminAuditRepo()
	wi := NewWizinvis(audits)

	runCmd(t, wi, s, "wizinvis")
	runCmd(t, wi, s, "wizinvis")

	got := auditList(t, audits)
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
	// List is newest-first.
	if got[0].Target != "off" || got[1].Target != "on" {
		t.Fatalf("toggle states not recorded in order: %+v", got)
	}
}

func TestAudit_Spawn_RecordsItemAndMob(t *testing.T) {
	items := repo.NewMemoryItemRepo()
	mobTemplates := repo.NewMemoryMobTemplateRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	chars := repo.NewMemoryCharacterRepo()
	audits := repo.NewMemoryAdminAuditRepo()

	if _, err := mobTemplates.Create(context.Background(), creature.MobTemplate{
		ExternalID: "tr.village_dog",
		Core:       creature.Core{Name: "a village dog", HPMax: 8},
	}); err != nil {
		t.Fatalf("seed dog: %v", err)
	}
	if _, err := items.Create(context.Background(), repo.Item{
		ExternalID: "tr.inn_lantern", Name: "a brass lantern",
		ShortDesc: "A small brass lantern.", RoomID: 99,
		Type: repo.ItemTypeLight, Weight: 2, Quality: repo.QualityNormal,
		Stats: &repo.LightStats{RadiusFt: 20, FuelTicks: 600},
	}); err != nil {
		t.Fatalf("seed lantern: %v", err)
	}

	sessions := session.NewRegistry()
	admin, _ := bufSession(t)
	admin.AuthLevel = telnet.AuthAdmin
	admin.CharacterID = 1
	admin.CharacterName = "Admin"
	admin.CurrentRoomID = 1

	c := NewSpawn(items, mobTemplates, mobs, chars, sessions, audits)

	runCmd(t, c, admin, "mob tr.village_dog 2")
	runCmd(t, c, admin, "item tr.inn_lantern 1")
	runCmd(t, c, admin, "mob tr.no_such_thing") // refusal — must NOT audit

	got := auditList(t, audits)
	if len(got) != 2 {
		t.Fatalf("want 2 rows (no row for unknown template); got %d: %+v", len(got), got)
	}
	for _, e := range got {
		if e.Verb != "spawn" {
			t.Fatalf("unexpected verb: %+v", e)
		}
		if !strings.HasPrefix(e.Target, "tr.") {
			t.Fatalf("target should be the requested ext; got %+v", e)
		}
	}
}

func TestAudit_Shutdown_RecordsScheduleAndCancel(t *testing.T) {
	s, _ := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	s.CharacterID = 1
	s.CharacterName = "Admin"
	ctl := &fakeShutdownCtl{}
	audits := repo.NewMemoryAdminAuditRepo()
	c := NewShutdown(ctl, audits)

	runCmd(t, c, s, "60 maintenance window")
	runCmd(t, c, s, "cancel")

	got := auditList(t, audits)
	if len(got) != 2 {
		t.Fatalf("want 2 rows; got %d: %+v", len(got), got)
	}
	verbs := []string{got[0].Verb, got[1].Verb}
	wantSet := map[string]bool{"shutdown": false, "shutdown:cancel": false}
	for _, v := range verbs {
		if _, ok := wantSet[v]; !ok {
			t.Fatalf("unexpected verb %q in %+v", v, got)
		}
		wantSet[v] = true
	}
	for v, seen := range wantSet {
		if !seen {
			t.Fatalf("missing audit row for %q (got %+v)", v, got)
		}
	}
}

func TestAudit_Reboot_RecordsSchedule(t *testing.T) {
	s, _ := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	s.CharacterID = 1
	s.CharacterName = "Admin"
	ctl := &fakeShutdownCtl{}
	audits := repo.NewMemoryAdminAuditRepo()
	c := NewReboot(ctl, audits)

	runCmd(t, c, s, "5m upgrades")

	got := auditList(t, audits)
	if len(got) != 1 || got[0].Verb != "reboot" {
		t.Fatalf("expected single reboot row; got %+v", got)
	}
	if got[0].Args != "upgrades" {
		t.Fatalf("reason not stored in args: %+v", got[0])
	}
}

func TestAudit_Shutdown_NoRecordOnControllerError(t *testing.T) {
	s, _ := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	ctl := &fakeShutdownCtl{err: ErrShutdownPending}
	audits := repo.NewMemoryAdminAuditRepo()
	c := NewShutdown(ctl, audits)

	runCmd(t, c, s, "60")

	if got := auditList(t, audits); len(got) != 0 {
		t.Fatalf("controller error should not audit; got %+v", got)
	}
}
