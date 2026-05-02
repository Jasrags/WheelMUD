package cmd

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// findCmd looks a command up in a slice produced by NewMoveFamily.
// The family returns commands in a fixed order but tests should not
// depend on that ordering.
func findCmd(t *testing.T, cmds []*telnet.Command, name string) *telnet.Command {
	t.Helper()
	for _, c := range cmds {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no command named %q in family", name)
	return nil
}

func TestMove_BlockedDirection(t *testing.T) {
	rooms, exits, items, mobs := seedWorld(t)
	chars := repo.NewMemoryCharacterRepo()

	s, conn := bufSession(t)
	s.CurrentRoomID = 1
	// Plaza has only n/s exits in seedWorld; "up" is blocked.
	family := NewMoveFamily(rooms, exits, items, mobs, chars, nil)
	up := findCmd(t, family, "up")

	ctx := &telnet.Context{Ctx: context.Background(), Session: s, Name: "up"}
	if err := up.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(conn.String(), "You can't go that way") {
		t.Fatalf("expected blocked message, got: %q", conn.String())
	}
	if s.CurrentRoomID != 1 {
		t.Fatalf("CurrentRoomID drifted to %d", s.CurrentRoomID)
	}
}

func TestMove_SectorGatesAirUnderwater(t *testing.T) {
	rooms, exits, items, mobs := seedWorld(t)
	chars := repo.NewMemoryCharacterRepo()

	// Add an underwater room east of the plaza.
	rooms.Insert(repo.Room{ID: 3, Name: "Submerged Reef", Sector: repo.SectorUnderwater})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 3, Direction: repo.DirEast})

	s, conn := bufSession(t)
	s.CurrentRoomID = 1
	family := NewMoveFamily(rooms, exits, items, mobs, chars, nil)
	east := findCmd(t, family, "east")

	ctx := &telnet.Context{Ctx: context.Background(), Session: s, Name: "east"}
	if err := east.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(conn.String(), "cannot swim") {
		t.Fatalf("expected swim refusal; got %q", conn.String())
	}
	if s.CurrentRoomID != 1 {
		t.Fatalf("CurrentRoomID drifted to %d", s.CurrentRoomID)
	}
}

func TestMove_FlySpeedAllowsAir(t *testing.T) {
	rooms, exits, items, mobs := seedWorld(t)
	chars := repo.NewMemoryCharacterRepo()

	rooms.Insert(repo.Room{ID: 4, Name: "Open Sky", Sector: repo.SectorAir})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 4, Direction: repo.DirUp})

	flyer, _ := chars.Create(context.Background(), repo.Character{
		AccountID: 1, Name: "Wing",
		Core: creature.Core{Name: "Wing", Speed: creature.Speed{BaseFt: 30, FlyFt: 60}},
	})

	s, conn := bufSession(t)
	s.CharacterID = flyer.ID
	s.CharacterName = flyer.Name
	s.CurrentRoomID = 1
	family := NewMoveFamily(rooms, exits, items, mobs, chars, nil)
	up := findCmd(t, family, "up")

	ctx := &telnet.Context{Ctx: context.Background(), Session: s, Name: "up"}
	if err := up.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if s.CurrentRoomID != 4 {
		t.Fatalf("flyer should have reached air room; got CurrentRoomID = %d, output: %q", s.CurrentRoomID, conn.String())
	}
}

func TestMove_SuccessUpdatesSessionAndPersists(t *testing.T) {
	rooms, exits, items, mobs := seedWorld(t)
	chars := repo.NewMemoryCharacterRepo()
	acct := int64(1)
	c, _ := chars.Create(context.Background(), repo.Character{AccountID: acct, Name: "Pippin"})

	s, conn := bufSession(t)
	s.CharacterID = c.ID
	s.CurrentRoomID = 1
	family := NewMoveFamily(rooms, exits, items, mobs, chars, nil)
	north := findCmd(t, family, "north")

	ctx := &telnet.Context{Ctx: context.Background(), Session: s, Name: "north"}
	if err := north.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if s.CurrentRoomID != 2 {
		t.Fatalf("CurrentRoomID = %d, want 2", s.CurrentRoomID)
	}
	// RecordRoom must have stored 2 too.
	stored, err := chars.FindByName(context.Background(), "Pippin")
	if err != nil {
		t.Fatalf("FindByName: %v", err)
	}
	if stored.CurrentRoomID != 2 {
		t.Fatalf("persisted CurrentRoomID = %d, want 2", stored.CurrentRoomID)
	}
	// And the new room was rendered automatically.
	if !strings.Contains(conn.String(), "North Road") {
		t.Fatalf("expected 'North Road' in output; got %q", conn.String())
	}
}

func TestMove_PublishesPlayerEnteredAndLeft(t *testing.T) {
	rooms, exits, items, mobs := seedWorld(t)
	chars := repo.NewMemoryCharacterRepo()
	acct := int64(1)
	c, _ := chars.Create(context.Background(), repo.Character{AccountID: acct, Name: "Eowyn"})

	bus := eventbus.New()
	defer bus.Stop()

	var mu sync.Mutex
	var entered []world.PlayerEntered
	var left []world.PlayerLeft
	eventbus.Subscribe(bus, func(_ context.Context, ev world.PlayerEntered) {
		mu.Lock()
		entered = append(entered, ev)
		mu.Unlock()
	})
	eventbus.Subscribe(bus, func(_ context.Context, ev world.PlayerLeft) {
		mu.Lock()
		left = append(left, ev)
		mu.Unlock()
	})

	s, _ := bufSession(t)
	s.CharacterID = c.ID
	s.CurrentRoomID = 1
	family := NewMoveFamily(rooms, exits, items, mobs, chars, bus)
	north := findCmd(t, family, "north")

	ctx := &telnet.Context{Ctx: context.Background(), Session: s, Name: "north"}
	if err := north.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(left) != 1 || left[0].FromRoomID != 1 || left[0].ToRoomID != 2 || left[0].CharacterID != c.ID {
		t.Fatalf("PlayerLeft = %+v", left)
	}
	if len(entered) != 1 || entered[0].FromRoomID != 1 || entered[0].ToRoomID != 2 || entered[0].CharacterID != c.ID {
		t.Fatalf("PlayerEntered = %+v", entered)
	}
}

func TestMove_DiagonalDirections(t *testing.T) {
	// Each diagonal verb gets its own command in the family. Wire a
	// throwaway exit per direction off room 1 and assert the command
	// resolves and moves the session to the matching destination.
	rooms, exits, items, mobs := seedWorld(t)
	chars := repo.NewMemoryCharacterRepo()

	cases := []struct {
		name string
		dir  string
		dest int64
	}{
		{"northeast", repo.DirNortheast, 10},
		{"northwest", repo.DirNorthwest, 11},
		{"southeast", repo.DirSoutheast, 12},
		{"southwest", repo.DirSouthwest, 13},
	}
	for _, tc := range cases {
		rooms.Insert(repo.Room{ID: tc.dest, Name: tc.name + " room"})
		exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: tc.dest, Direction: tc.dir})
	}

	family := NewMoveFamily(rooms, exits, items, mobs, chars, nil)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := bufSession(t)
			s.CurrentRoomID = 1

			cmd := findCmd(t, family, tc.name)
			ctx := &telnet.Context{Ctx: context.Background(), Session: s, Name: tc.name}
			if err := cmd.Run(ctx); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if s.CurrentRoomID != tc.dest {
				t.Fatalf("CurrentRoomID = %d, want %d", s.CurrentRoomID, tc.dest)
			}
		})
	}
}

func TestMove_NoCharacterStillMovesInProcess(t *testing.T) {
	// Edge case: a session that somehow has CurrentRoomID set but no
	// CharacterID (shouldn't happen in production but is cheap to cover).
	// The move should still work in-memory; we just skip persistence.
	rooms, exits, items, mobs := seedWorld(t)
	chars := repo.NewMemoryCharacterRepo()

	s, _ := bufSession(t)
	s.CurrentRoomID = 1
	family := NewMoveFamily(rooms, exits, items, mobs, chars, nil)
	north := findCmd(t, family, "north")

	ctx := &telnet.Context{Ctx: context.Background(), Session: s, Name: "north"}
	if err := north.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if s.CurrentRoomID != 2 {
		t.Fatalf("CurrentRoomID = %d, want 2", s.CurrentRoomID)
	}
}
