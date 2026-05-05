package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// spawnFixture returns a pre-seeded command bundle plus an admin
// session in room 1. A "village dog" mob template and an
// "tr.inn_lantern" item template are seeded so happy-path tests
// don't have to rebuild them.
type spawnFixture struct {
	cmd          *telnet.Command
	items        *repo.MemoryItemRepo
	mobTemplates *repo.MemoryMobTemplateRepo
	mobs         *repo.MemoryMobInstanceRepo
	admin        *telnet.Session
	other        *telnet.Session
	adminOut     *bufConn
	otherOut     *bufConn
	dogTemplate  creature.MobTemplate
	lantern      repo.Item
}

func newSpawnFixture(t *testing.T) *spawnFixture {
	t.Helper()
	items := repo.NewMemoryItemRepo()
	mobTemplates := repo.NewMemoryMobTemplateRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	characters := repo.NewMemoryCharacterRepo()

	dog, err := mobTemplates.Create(context.Background(), creature.MobTemplate{
		ExternalID: "tr.village_dog",
		Core: creature.Core{
			Name:  "a village dog",
			HPMax: 8,
		},
	})
	if err != nil {
		t.Fatalf("seed dog template: %v", err)
	}

	lantern, err := items.Create(context.Background(), repo.Item{
		ExternalID: "tr.inn_lantern", Name: "a brass lantern",
		ShortDesc: "A small brass lantern.",
		RoomID:    99, // seed room — unrelated to the spawn target
		Type:      repo.ItemTypeLight, Weight: 2, Quality: repo.QualityNormal,
		Stats: &repo.LightStats{RadiusFt: 20, FuelTicks: 600},
	})
	if err != nil {
		t.Fatalf("seed lantern: %v", err)
	}

	sessions, admin, other, aOut, oOut := commPair(t)
	admin.AuthLevel = telnet.AuthAdmin
	admin.CharacterName = "Admin"

	cmd := NewSpawn(items, mobTemplates, mobs, characters, sessions)
	return &spawnFixture{
		cmd: cmd, items: items, mobTemplates: mobTemplates, mobs: mobs,
		admin: admin, other: other, adminOut: aOut, otherOut: oOut,
		dogTemplate: dog, lantern: lantern,
	}
}

func TestSpawn_MobHappyPath(t *testing.T) {
	f := newSpawnFixture(t)
	runCmd(t, f.cmd, f.admin, "mob tr.village_dog 3")

	if !strings.Contains(f.adminOut.String(), "Spawned 3 × a village dog") {
		t.Fatalf("admin echo missing; got %q", f.adminOut.String())
	}
	if !strings.Contains(f.otherOut.String(), "3 copies of a village dog") {
		t.Fatalf("room broadcast missing; got %q", f.otherOut.String())
	}
	got, err := f.mobs.ListInRoom(context.Background(), f.admin.CurrentRoomID)
	if err != nil {
		t.Fatalf("ListInRoom: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d mobs, want 3", len(got))
	}
	for _, m := range got {
		if m.TemplateID != f.dogTemplate.ID {
			t.Errorf("template id mismatch: %+v", m)
		}
	}
}

func TestSpawn_MobDefaultCount(t *testing.T) {
	f := newSpawnFixture(t)
	runCmd(t, f.cmd, f.admin, "mob tr.village_dog")
	got, _ := f.mobs.ListInRoom(context.Background(), f.admin.CurrentRoomID)
	if len(got) != 1 {
		t.Fatalf("default count should be 1; got %d", len(got))
	}
}

func TestSpawn_MobCountClamped(t *testing.T) {
	f := newSpawnFixture(t)
	runCmd(t, f.cmd, f.admin, "mob tr.village_dog 9999")
	got, _ := f.mobs.ListInRoom(context.Background(), f.admin.CurrentRoomID)
	if len(got) != spawnCountMax {
		t.Fatalf("count should clamp to %d; got %d", spawnCountMax, len(got))
	}
}

func TestSpawn_MobUnknownTemplate(t *testing.T) {
	f := newSpawnFixture(t)
	runCmd(t, f.cmd, f.admin, "mob tr.no_such_thing")
	if !strings.Contains(f.adminOut.String(), "No mob template with that id") {
		t.Fatalf("missing refusal; got %q", f.adminOut.String())
	}
}

func TestSpawn_ItemHappyPath(t *testing.T) {
	f := newSpawnFixture(t)
	runCmd(t, f.cmd, f.admin, "item tr.inn_lantern 2")

	if !strings.Contains(f.adminOut.String(), "Spawned 2 × a brass lantern") {
		t.Fatalf("admin echo missing; got %q", f.adminOut.String())
	}
	got, err := f.items.ListInRoom(context.Background(), f.admin.CurrentRoomID)
	if err != nil {
		t.Fatalf("ListInRoom: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	// External ids must be unique runtime ids, distinct from the seed.
	seen := map[string]bool{}
	for _, it := range got {
		if it.ExternalID == "tr.inn_lantern" {
			t.Errorf("spawn reused template external id; want suffix: %s", it.ExternalID)
		}
		if seen[it.ExternalID] {
			t.Errorf("duplicate external_id %q", it.ExternalID)
		}
		seen[it.ExternalID] = true
		if it.Type != repo.ItemTypeLight || it.Weight != 2 {
			t.Errorf("typed fields not copied: %+v", it)
		}
	}
}

func TestSpawn_ItemStatsDeepCloned(t *testing.T) {
	f := newSpawnFixture(t)
	runCmd(t, f.cmd, f.admin, "item tr.inn_lantern 2")
	got, _ := f.items.ListInRoom(context.Background(), f.admin.CurrentRoomID)
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	a, ok := got[0].Stats.(*repo.LightStats)
	if !ok {
		t.Fatalf("first stats: want *LightStats, got %T", got[0].Stats)
	}
	b, ok := got[1].Stats.(*repo.LightStats)
	if !ok {
		t.Fatalf("second stats: want *LightStats, got %T", got[1].Stats)
	}
	if a == b {
		t.Fatalf("stats pointers must not alias")
	}
	a.FuelTicks = 1
	if b.FuelTicks == 1 {
		t.Fatalf("stats not deep-copied: mutating one changed the other")
	}
}

func TestSpawn_BadCount(t *testing.T) {
	f := newSpawnFixture(t)
	runCmd(t, f.cmd, f.admin, "mob tr.village_dog 0")
	if !strings.Contains(f.adminOut.String(), "positive integer") {
		t.Fatalf("expected positive-int refusal; got %q", f.adminOut.String())
	}
}

func TestSpawn_BadKind(t *testing.T) {
	f := newSpawnFixture(t)
	runCmd(t, f.cmd, f.admin, "creature tr.village_dog")
	if !strings.Contains(f.adminOut.String(), "'mob' or 'item'") {
		t.Fatalf("expected kind refusal; got %q", f.adminOut.String())
	}
}

func TestSpawn_NoRoomRefuses(t *testing.T) {
	f := newSpawnFixture(t)
	f.admin.CurrentRoomID = 0
	runCmd(t, f.cmd, f.admin, "mob tr.village_dog")
	if !strings.Contains(f.adminOut.String(), "must be in a room") {
		t.Fatalf("expected no-room refusal; got %q", f.adminOut.String())
	}
}
