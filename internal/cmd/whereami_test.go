package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

func TestWhereAmI_RendersAllFields(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{
		ID: 1, ExternalID: "plaza.fountain", Name: "Town Plaza",
		Sector: repo.SectorCity, LightLevel: 100,
		Flags:  repo.RoomFlags{NoTeleport: true, Silent: true},
		CoordX: 5, CoordY: -2, CoordZ: 0,
		ExtraDescs: map[string]string{"fountain": "marble basin"},
	})
	cmd := NewWhereAmI(rooms)

	s, conn := bufSession(t)
	s.CurrentRoomID = 1
	s.AuthLevel = telnet.AuthAdmin

	ctx := &telnet.Context{Ctx: context.Background(), Session: s, Name: "whereami"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := conn.String()
	for _, want := range []string{
		"Room 1", "plaza.fountain", "Town Plaza",
		"city", "Light:", "100",
		"(5, -2, 0)",
		"noteleport", "silent",
		"Keywords:", "fountain",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestWhereAmI_NowhereSession(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	cmd := NewWhereAmI(rooms)
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin

	ctx := &telnet.Context{Ctx: context.Background(), Session: s, Name: "whereami"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(conn.String(), "nowhere") {
		t.Fatalf("expected nowhere message; got %q", conn.String())
	}
}
