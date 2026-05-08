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

type stillFixture struct {
	chars    *repo.MemoryCharacterRepo
	audits   *repo.MemoryAdminAuditRepo
	sessions *session.Registry
	admin    *telnet.Session
	target   *telnet.Session
	aOut     *bufConn
	tOut     *bufConn
}

func newStillFixture(t *testing.T, channeling *creature.Channeling) *stillFixture {
	t.Helper()
	chars := repo.NewMemoryCharacterRepo()
	if _, err := chars.Create(context.Background(), repo.Character{
		AccountID:     1,
		Name:          "Adminny",
		CurrentRoomID: 1,
		AuthLevel:     uint8(telnet.AuthAdmin),
	}); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if _, err := chars.Create(context.Background(), repo.Character{
		AccountID:     2,
		Name:          "Bob",
		CurrentRoomID: 1,
		Channeling:    channeling,
		AuthLevel:     uint8(telnet.AuthPlayer),
	}); err != nil {
		t.Fatalf("seed bob: %v", err)
	}
	sessions := session.NewRegistry()
	admin, aConn := bufSession(t)
	admin.AccountID = 1
	admin.AuthLevel = telnet.AuthAdmin
	admin.CharacterID = 1
	admin.CharacterName = "Adminny"
	admin.CurrentRoomID = 1
	sessions.Bind(admin.AccountID, admin)

	target, tConn := bufSession(t)
	target.AccountID = 2
	target.AuthLevel = telnet.AuthPlayer
	target.CharacterID = 2
	target.CharacterName = "Bob"
	target.CurrentRoomID = 1
	sessions.Bind(target.AccountID, target)

	return &stillFixture{
		chars:    chars,
		audits:   repo.NewMemoryAdminAuditRepo(),
		sessions: sessions,
		admin:    admin,
		target:   target,
		aOut:     aConn,
		tOut:     tConn,
	}
}

func TestStill_HappyPath(t *testing.T) {
	f := newStillFixture(t, &creature.Channeling{
		GenderSource: creature.SourceSaidin,
		Slots: [10]creature.SlotPool{
			{Cur: 4, Max: 4}, {Cur: 4, Max: 4},
		},
	})
	c := NewStill(f.chars, f.sessions, f.audits)
	runCmd(t, c, f.admin, "Bob")

	got, _ := f.chars.FindByName(context.Background(), "Bob")
	if got.Channeling == nil || !got.Channeling.Stilled {
		t.Fatalf("Bob not stilled: %+v", got.Channeling)
	}
	if got.Channeling.Slots[0].Cur != 0 {
		t.Fatalf("slots not zeroed: %d", got.Channeling.Slots[0].Cur)
	}
	if !strings.Contains(f.aOut.String(), "sever Bob") {
		t.Fatalf("admin echo missing: %s", f.aOut.String())
	}
	if !strings.Contains(f.tOut.String(), "torn from your reach") {
		t.Fatalf("target notify missing: %s", f.tOut.String())
	}
	rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10})
	if len(rows) != 1 || rows[0].Verb != "still" || rows[0].Target != "Bob" {
		t.Fatalf("audit row: %+v", rows)
	}
}

func TestStill_NonChannelerRefuses(t *testing.T) {
	f := newStillFixture(t, nil)
	c := NewStill(f.chars, f.sessions, f.audits)
	runCmd(t, c, f.admin, "Bob")

	got, _ := f.chars.FindByName(context.Background(), "Bob")
	if got.Channeling != nil {
		t.Fatalf("channeling sprouted: %+v", got.Channeling)
	}
	if !strings.Contains(f.aOut.String(), "cannot channel") {
		t.Fatalf("refusal missing: %s", f.aOut.String())
	}
	rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10})
	if len(rows) != 0 {
		t.Fatalf("refusal must not audit: %+v", rows)
	}
}

func TestStill_OfflineTargetRefuses(t *testing.T) {
	f := newStillFixture(t, &creature.Channeling{
		GenderSource: creature.SourceSaidin,
	})
	c := NewStill(f.chars, f.sessions, f.audits)
	runCmd(t, c, f.admin, "Mallory")

	if !strings.Contains(f.aOut.String(), "No such player online") {
		t.Fatalf("offline refusal missing: %s", f.aOut.String())
	}
	rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10})
	if len(rows) != 0 {
		t.Fatalf("refusal must not audit: %+v", rows)
	}
}

func TestStill_AlreadyStilled(t *testing.T) {
	f := newStillFixture(t, &creature.Channeling{
		GenderSource: creature.SourceSaidin,
		Stilled:      true,
	})
	c := NewStill(f.chars, f.sessions, f.audits)
	runCmd(t, c, f.admin, "Bob")

	if !strings.Contains(f.aOut.String(), "already stilled") {
		t.Fatalf("idempotence message missing: %s", f.aOut.String())
	}
	rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10})
	if len(rows) != 0 {
		t.Fatalf("no-op must not audit: %+v", rows)
	}
}

func TestUnstill_HappyPath(t *testing.T) {
	f := newStillFixture(t, &creature.Channeling{
		GenderSource: creature.SourceSaidin,
		Stilled:      true,
		Slots:        [10]creature.SlotPool{{Cur: 0, Max: 4}},
	})
	c := NewUnstill(f.chars, f.sessions, f.audits)
	runCmd(t, c, f.admin, "Bob")

	got, _ := f.chars.FindByName(context.Background(), "Bob")
	if got.Channeling.Stilled {
		t.Fatalf("Bob still stilled")
	}
	// Slots stay zero — refresh pulse refills them on its own cadence.
	if got.Channeling.Slots[0].Cur != 0 {
		t.Fatalf("unstill must NOT auto-refill slots: got %d", got.Channeling.Slots[0].Cur)
	}
	rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{Limit: 10})
	if len(rows) != 1 || rows[0].Verb != "unstill" {
		t.Fatalf("audit row: %+v", rows)
	}
}
