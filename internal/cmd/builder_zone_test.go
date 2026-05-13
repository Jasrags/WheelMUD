package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// builderZoneTestFixture wires the repos shared across the grant /
// revoke / grants verb tests. Seeds Alice (admin) + Bob (player) into
// the character repo and one zone "emonds_field" into the zone repo.
type builderZoneTestFixture struct {
	admin      *telnet.Session
	alice      repo.Character
	bob        repo.Character
	zone       repo.Zone
	characters *repo.MemoryCharacterRepo
	zones      *repo.MemoryZoneRepo
	builders   *repo.MemoryBuilderZoneRepo
	audits     *repo.MemoryAdminAuditRepo
	sessions   *session.Registry
}

func newBuilderZoneFixture(t *testing.T) *builderZoneTestFixture {
	t.Helper()
	ctx := context.Background()
	characters := repo.NewMemoryCharacterRepo()
	zones := repo.NewMemoryZoneRepo()
	builders := repo.NewMemoryBuilderZoneRepo()
	audits := repo.NewMemoryAdminAuditRepo()

	// Seed accounts indirectly by going through Create; the memory
	// repo doesn't enforce account FK so we use AccountID=1 / 2.
	alice, err := characters.Create(ctx, repo.Character{AccountID: 1, Name: "Alice"})
	if err != nil {
		t.Fatalf("create Alice: %v", err)
	}
	bob, err := characters.Create(ctx, repo.Character{AccountID: 2, Name: "Bob"})
	if err != nil {
		t.Fatalf("create Bob: %v", err)
	}
	zone, err := zones.Create(ctx, repo.Zone{ExternalID: "emonds_field", Name: "Emond's Field", ResetMode: repo.ZoneResetEmpty})
	if err != nil {
		t.Fatalf("create zone: %v", err)
	}

	admin, _ := bufSession(t)
	admin.AccountID = 1
	admin.AuthLevel = telnet.AuthAdmin
	admin.CharacterID = alice.ID
	admin.CharacterName = "Alice"

	return &builderZoneTestFixture{
		admin:      admin,
		alice:      alice,
		bob:        bob,
		zone:       zone,
		characters: characters,
		zones:      zones,
		builders:   builders,
		audits:     audits,
		sessions:   session.NewRegistry(),
	}
}

func TestGrant_RecordsGrantAndAudits(t *testing.T) {
	f := newBuilderZoneFixture(t)
	grant := NewGrant(f.builders, f.characters, f.zones, f.sessions, f.audits)

	runCmd(t, grant, f.admin, "Bob emonds_field")

	ok, err := f.builders.Has(context.Background(), f.bob.ID, f.zone.ID)
	if err != nil {
		t.Fatalf("has: %v", err)
	}
	if !ok {
		t.Fatal("grant: builders.Has returned false after grant")
	}
	if n := f.audits.Len(); n != 1 {
		t.Fatalf("audit count = %d, want 1", n)
	}
	entries, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{})
	if entries[0].Verb != "grant" || entries[0].Target != "Bob" {
		t.Fatalf("audit row = %+v, want grant/Bob", entries[0])
	}
	if !strings.Contains(entries[0].Args, "zone=emonds_field") {
		t.Fatalf("audit args = %q, want zone=emonds_field", entries[0].Args)
	}
}

func TestGrant_UnknownPlayerRefuses(t *testing.T) {
	f := newBuilderZoneFixture(t)
	grant := NewGrant(f.builders, f.characters, f.zones, f.sessions, f.audits)

	runCmd(t, grant, f.admin, "Ghost emonds_field")

	rows, _ := f.builders.ListForCharacter(context.Background(), f.bob.ID)
	if len(rows) != 0 {
		t.Fatal("no grant should have been recorded")
	}
	if n := f.audits.Len(); n != 0 {
		t.Fatalf("audit count = %d, want 0 on failure", n)
	}
}

func TestGrant_UnknownZoneRefuses(t *testing.T) {
	f := newBuilderZoneFixture(t)
	grant := NewGrant(f.builders, f.characters, f.zones, f.sessions, f.audits)

	runCmd(t, grant, f.admin, "Bob nowhere")

	rows, _ := f.builders.ListForCharacter(context.Background(), f.bob.ID)
	if len(rows) != 0 {
		t.Fatal("no grant should have been recorded")
	}
	if n := f.audits.Len(); n != 0 {
		t.Fatalf("audit count = %d, want 0 on failure", n)
	}
}

func TestRevoke_RemovesAndAudits(t *testing.T) {
	f := newBuilderZoneFixture(t)
	if err := f.builders.Grant(context.Background(), f.bob.ID, f.zone.ID, f.alice.ID, time.Time{}); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
	revoke := NewRevoke(f.builders, f.characters, f.zones, f.sessions, f.audits)

	runCmd(t, revoke, f.admin, "Bob emonds_field")

	ok, _ := f.builders.Has(context.Background(), f.bob.ID, f.zone.ID)
	if ok {
		t.Fatal("revoke: grant should be gone")
	}
	if n := f.audits.Len(); n != 1 {
		t.Fatalf("audit count = %d, want 1", n)
	}
}

func TestRevoke_MissingGrantNoAudit(t *testing.T) {
	f := newBuilderZoneFixture(t)
	revoke := NewRevoke(f.builders, f.characters, f.zones, f.sessions, f.audits)

	runCmd(t, revoke, f.admin, "Bob emonds_field")

	if n := f.audits.Len(); n != 0 {
		t.Fatalf("audit count = %d, want 0 on missing grant", n)
	}
}

func TestCanEditZone(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(s *telnet.Session)
		zoneID  int64
		nilSess bool
		want    bool
	}{
		{
			name:    "nil session is never editor",
			nilSess: true,
			zoneID:  42,
			want:    false,
		},
		{
			name:   "AuthAdmin bypasses grant table",
			setup:  func(s *telnet.Session) { s.AuthLevel = telnet.AuthAdmin },
			zoneID: 42,
			want:   true,
		},
		{
			name: "AuthPlayer with matching grant",
			setup: func(s *telnet.Session) {
				s.AuthLevel = telnet.AuthPlayer
				s.SetBuilderZones(map[int64]struct{}{42: {}})
			},
			zoneID: 42,
			want:   true,
		},
		{
			name: "AuthPlayer without grant",
			setup: func(s *telnet.Session) {
				s.AuthLevel = telnet.AuthPlayer
				s.SetBuilderZones(map[int64]struct{}{99: {}})
			},
			zoneID: 42,
			want:   false,
		},
		{
			name:   "AuthPlayer with no grants at all",
			setup:  func(s *telnet.Session) { s.AuthLevel = telnet.AuthPlayer },
			zoneID: 42,
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s *telnet.Session
			if !tt.nilSess {
				s, _ = bufSession(t)
				if tt.setup != nil {
					tt.setup(s)
				}
			}
			if got := CanEditZone(s, tt.zoneID); got != tt.want {
				t.Fatalf("CanEditZone = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGrant_RefreshesOnlineTargetSession(t *testing.T) {
	f := newBuilderZoneFixture(t)
	// Bind Bob's live session into the registry so the grant verb's
	// refreshBuilderZonesForOnline finds him.
	bobSess, _ := bufSession(t)
	bobSess.AccountID = 200
	bobSess.AuthLevel = telnet.AuthPlayer
	bobSess.SetInWorld(f.bob.ID, "Bob", 1)
	f.sessions.Bind(bobSess.AccountID, bobSess)

	grant := NewGrant(f.builders, f.characters, f.zones, f.sessions, f.audits)
	runCmd(t, grant, f.admin, "Bob emonds_field")

	if !bobSess.IsBuilderFor(f.zone.ID) {
		t.Fatalf("bob session: IsBuilderFor(%d) = false after grant", f.zone.ID)
	}
}

func TestRevoke_RefreshesOnlineTargetSession(t *testing.T) {
	f := newBuilderZoneFixture(t)
	if err := f.builders.Grant(context.Background(), f.bob.ID, f.zone.ID, f.alice.ID, time.Time{}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	bobSess, _ := bufSession(t)
	bobSess.AccountID = 200
	bobSess.AuthLevel = telnet.AuthPlayer
	bobSess.SetInWorld(f.bob.ID, "Bob", 1)
	bobSess.SetBuilderZones(map[int64]struct{}{f.zone.ID: {}})
	f.sessions.Bind(bobSess.AccountID, bobSess)

	revoke := NewRevoke(f.builders, f.characters, f.zones, f.sessions, f.audits)
	runCmd(t, revoke, f.admin, "Bob emonds_field")

	if bobSess.IsBuilderFor(f.zone.ID) {
		t.Fatalf("bob session: IsBuilderFor(%d) = true after revoke", f.zone.ID)
	}
}

func TestGrants_NamesPlayer(t *testing.T) {
	f := newBuilderZoneFixture(t)
	if err := f.builders.Grant(context.Background(), f.bob.ID, f.zone.ID, f.alice.ID, time.Time{}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	sess, out := bufSession(t)
	sess.AuthLevel = telnet.AuthAdmin
	sess.CharacterID = f.alice.ID
	sess.CharacterName = "Alice"
	grants := NewGrants(f.builders, f.characters, f.zones)
	runCmd(t, grants, sess, "Bob")
	got := out.String()
	if !strings.Contains(got, "emonds_field") {
		t.Fatalf("grants <bob>: missing zone in output: %q", got)
	}
}
