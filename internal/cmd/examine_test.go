package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

func TestExamine_MissingTargetMessage(t *testing.T) {
	_, _, items, mobs := emptyRoomWorld(t)
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CurrentRoomID = 1
	cmd := NewExamine(items, mobs)
	runCmd(t, cmd, s, "ghost")
	if !strings.Contains(conn.String(), "don't see anything") {
		t.Fatalf("expected miss message; got %q", conn.String())
	}
}

func TestExamine_NoRoomMessage(t *testing.T) {
	_, _, items, mobs := emptyRoomWorld(t)
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	// CurrentRoomID == 0 (unset)
	cmd := NewExamine(items, mobs)
	runCmd(t, cmd, s, "anything")
	if !strings.Contains(conn.String(), "nothing here") {
		t.Fatalf("expected nothing-here message; got %q", conn.String())
	}
}

func TestExamine_RendersItemShortDesc(t *testing.T) {
	rooms, exits, items, mobs := seedWorld(t)
	_ = rooms
	_ = exits
	// Patch the seeded pebble with a ShortDesc so we can assert on it.
	items.Insert(repo.Item{ID: 99, Name: "a polished stone", NameLower: "a polished stone", ShortDesc: "It catches the light unevenly.", RoomID: 1})

	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CurrentRoomID = 1

	runCmd(t, NewExamine(items, mobs), s, "polished")
	out := conn.String()
	for _, want := range []string{"a polished stone", "catches the light"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output; got %q", want, out)
		}
	}
}

func TestExamine_PrefersMobOverItem(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, Name: "Hall"})
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobInstanceRepo()

	// Both share the keyword "guard".
	items.Insert(repo.Item{ID: 1, Name: "a guard's helm", NameLower: "a guard's helm", ShortDesc: "ITEM", RoomID: 1})
	if _, err := mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: 1,
		Core: creature.Core{
			Name:          "a town guard",
			CurrentRoomID: 1,
			HPCurrent:     10,
			HPMax:         10,
		},
	}); err != nil {
		t.Fatalf("seed mob: %v", err)
	}

	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CurrentRoomID = 1

	runCmd(t, NewExamine(items, mobs), s, "guard")
	out := conn.String()
	if !strings.Contains(out, "a town guard") {
		t.Fatalf("expected mob name; got %q", out)
	}
	if strings.Contains(out, "ITEM") {
		t.Fatalf("expected mob to win over item; got %q", out)
	}
}

func TestExamine_RendersMobConditionsAndHP(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, Name: "Hall"})
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobInstanceRepo()

	if _, err := mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: 1,
		Core: creature.Core{
			Name:          "a wounded trolloc",
			CurrentRoomID: 1,
			HPCurrent:     3,
			HPMax:         20,
			Conditions:    creature.CondProne | creature.CondShaken,
			Affects: []creature.Affect{
				{Name: "weakness"},
			},
		},
	}); err != nil {
		t.Fatalf("seed mob: %v", err)
	}

	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CurrentRoomID = 1

	runCmd(t, NewExamine(items, mobs), s, "trolloc")
	out := conn.String()
	for _, want := range []string{"a wounded trolloc", "barely standing", "Conditions:", "prone", "shaken", "Affects:", "weakness"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q; got %q", want, out)
		}
	}
}

func TestHPDescriptor(t *testing.T) {
	cases := []struct {
		cur, max int32
		want     string
	}{
		{0, 0, "unknown"},
		{0, 10, "death"},
		{10, 10, "perfect"},
		{8, 10, "scratches"},
		{6, 10, "wounded"},
		{3, 10, "badly"},
		{1, 10, "barely"},
	}
	for _, tc := range cases {
		got := hpDescriptor(tc.cur, tc.max)
		if !strings.Contains(got, tc.want) {
			t.Errorf("hpDescriptor(%d,%d) = %q; expected to contain %q", tc.cur, tc.max, got, tc.want)
		}
	}
}

func emptyRoomWorld(t *testing.T) (*repo.MemoryRoomRepo, *repo.MemoryExitRepo, *repo.MemoryItemRepo, *repo.MemoryMobInstanceRepo) {
	t.Helper()
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, Name: "Hall"})
	return rooms, repo.NewMemoryExitRepo(), repo.NewMemoryItemRepo(), repo.NewMemoryMobInstanceRepo()
}
