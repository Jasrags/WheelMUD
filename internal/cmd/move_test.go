package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// findCmd looks a command up in a slice produced by NewMoveFamily. The
// move family returns the six commands in a fixed order but tests should
// not depend on that ordering.
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
	family := NewMoveFamily(rooms, exits, items, mobs, chars)
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

func TestMove_SuccessUpdatesSessionAndPersists(t *testing.T) {
	rooms, exits, items, mobs := seedWorld(t)
	chars := repo.NewMemoryCharacterRepo()
	acct := int64(1)
	c, _ := chars.Create(context.Background(), repo.Character{AccountID: acct, Name: "Pippin"})

	s, conn := bufSession(t)
	s.CharacterID = c.ID
	s.CurrentRoomID = 1
	family := NewMoveFamily(rooms, exits, items, mobs, chars)
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

func TestMove_NoCharacterStillMovesInProcess(t *testing.T) {
	// Edge case: a session that somehow has CurrentRoomID set but no
	// CharacterID (shouldn't happen in production but is cheap to cover).
	// The move should still work in-memory; we just skip persistence.
	rooms, exits, items, mobs := seedWorld(t)
	chars := repo.NewMemoryCharacterRepo()

	s, _ := bufSession(t)
	s.CurrentRoomID = 1
	family := NewMoveFamily(rooms, exits, items, mobs, chars)
	north := findCmd(t, family, "north")

	ctx := &telnet.Context{Ctx: context.Background(), Session: s, Name: "north"}
	if err := north.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if s.CurrentRoomID != 2 {
		t.Fatalf("CurrentRoomID = %d, want 2", s.CurrentRoomID)
	}
}
