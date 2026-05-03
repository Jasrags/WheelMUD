package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// stripColor counts brackets without ANSI noise. Tests assert on the
// literal cell glyphs (e.g. `[*]`) which cfmt wraps in escape codes —
// the bracketed substring itself is preserved between escapes, so
// strings.Contains works without any preprocessing.

func runMap(t *testing.T, depth string, rooms repo.RoomRepo, exits repo.ExitRepo, currentRoomID int64) string {
	t.Helper()
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthPlayer
	s.CurrentRoomID = currentRoomID
	args := []string{}
	if depth != "" {
		args = append(args, depth)
	}
	c := &telnet.Context{Ctx: context.Background(), Session: s, Name: "map", Args: args}
	if err := NewMap(rooms, exits).Run(c); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return conn.String()
}

func TestMap_NotInRoom(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	out := runMap(t, "", rooms, exits, 0)
	if !strings.Contains(out, "nowhere in particular") {
		t.Fatalf("expected 'nowhere in particular'; got %q", out)
	}
}

func TestMap_SingleRoomOnlyShowsStar(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	rooms.Insert(repo.Room{ID: 1, ExternalID: "solo", Name: "Solo"})
	out := runMap(t, "", rooms, exits, 1)
	if !strings.Contains(out, "[*]") {
		t.Fatalf("missing current-room glyph; got %q", out)
	}
	for _, leak := range []string{"[ ]", "[?]", "[^]", "[v]", "[%]"} {
		if strings.Contains(out, leak) {
			t.Errorf("unexpected glyph %q in single-room map: %q", leak, out)
		}
	}
}

func TestMap_CardinalCross(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	for i, name := range []string{"Center", "North", "South", "East", "West"} {
		rooms.Insert(repo.Room{ID: int64(i + 1), ExternalID: name, Name: name})
	}
	pairs := []struct {
		from, to int64
		dir      string
	}{
		{1, 2, repo.DirNorth}, {2, 1, repo.DirSouth},
		{1, 3, repo.DirSouth}, {3, 1, repo.DirNorth},
		{1, 4, repo.DirEast}, {4, 1, repo.DirWest},
		{1, 5, repo.DirWest}, {5, 1, repo.DirEast},
	}
	for _, p := range pairs {
		exits.Insert(repo.Exit{FromRoomID: p.from, ToRoomID: p.to, Direction: p.dir})
	}
	out := runMap(t, "", rooms, exits, 1)
	if !strings.Contains(out, "[*]") {
		t.Fatalf("missing center glyph: %q", out)
	}
	if strings.Count(out, "[ ]") != 4 {
		t.Errorf("expected 4 visited neighbors; got %q", out)
	}
	if !strings.Contains(out, "-") {
		t.Errorf("expected horizontal connector '-'; got %q", out)
	}
	if !strings.Contains(out, "|") {
		t.Errorf("expected vertical connector '|'; got %q", out)
	}
}

// TestMap_Depth1Boundary pins the off-by-one: depth 1 must reach
// exactly one neighbor (not zero, not two). Catches a regression where
// the BFS uses `item.d > depth` instead of `item.d >= depth` to guard
// the fan-out.
func TestMap_Depth1Boundary(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	for i := 1; i <= 3; i++ {
		rooms.Insert(repo.Room{ID: int64(i), ExternalID: "r", Name: "r"})
	}
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirEast})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 3, Direction: repo.DirEast})

	out := runMap(t, "1", rooms, exits, 1)
	if got := strings.Count(out, "[ ]"); got != 1 {
		t.Errorf("depth-1 expected exactly 1 visited neighbor; got %d in %q", got, out)
	}
}

func TestMap_DepthLimitTruncates(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	for i := 1; i <= 5; i++ {
		rooms.Insert(repo.Room{ID: int64(i), ExternalID: "r", Name: "r"})
	}
	for i := int64(1); i < 5; i++ {
		exits.Insert(repo.Exit{FromRoomID: i, ToRoomID: i + 1, Direction: repo.DirEast})
		exits.Insert(repo.Exit{FromRoomID: i + 1, ToRoomID: i, Direction: repo.DirWest})
	}
	// depth 2 from room 1 should reach rooms 2 and 3, not 4 or 5.
	out := runMap(t, "2", rooms, exits, 1)
	got := strings.Count(out, "[ ]")
	if got != 2 {
		t.Errorf("depth-2 expected 2 visited rooms; got %d in %q", got, out)
	}
}

func TestMap_CycleHandled(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	for i := 1; i <= 4; i++ {
		rooms.Insert(repo.Room{ID: int64(i), ExternalID: "r", Name: "r"})
	}
	// Square loop: 1 -n-> 2 -e-> 3 -s-> 4 -w-> 1
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 3, Direction: repo.DirEast})
	exits.Insert(repo.Exit{FromRoomID: 3, ToRoomID: 4, Direction: repo.DirSouth})
	exits.Insert(repo.Exit{FromRoomID: 4, ToRoomID: 1, Direction: repo.DirWest})

	out := runMap(t, "5", rooms, exits, 1)
	// Should terminate, render exactly one [*] and three [ ] cells.
	if strings.Count(out, "[*]") != 1 {
		t.Errorf("expected 1 [*]; got %q", out)
	}
	if strings.Count(out, "[ ]") != 3 {
		t.Errorf("expected 3 [ ] cells (cycle, first-visit-wins); got %q", out)
	}
}

func TestMap_UpDownGlyphs(t *testing.T) {
	// Verticals decorate non-current cells. Current cell always wins as
	// `[*]` so the player can find themselves at a glance — the look
	// command shows the room's u/d exits explicitly.
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	for i := 1; i <= 4; i++ {
		rooms.Insert(repo.Room{ID: int64(i), ExternalID: "r", Name: "r"})
	}
	// 1 -e-> 2; 2 -u-> 3; 2 -d-> 4. Player stands in room 1, room 2
	// is the neighbor with both verticals.
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirEast})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 3, Direction: repo.DirUp})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 4, Direction: repo.DirDown})

	out := runMap(t, "", rooms, exits, 1)
	if !strings.Contains(out, "[%]") {
		t.Errorf("expected [%%] on neighbor with both up and down; got %q", out)
	}
}

func TestMap_HiddenExitNotFollowed(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	rooms.Insert(repo.Room{ID: 1, ExternalID: "a", Name: "A"})
	rooms.Insert(repo.Room{ID: 2, ExternalID: "b", Name: "B"})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth, Flags: repo.ExitFlags{Hidden: true}})

	out := runMap(t, "", rooms, exits, 1)
	if strings.Contains(out, "[ ]") {
		t.Errorf("hidden exit should not produce a visited cell; got %q", out)
	}
}

func TestMap_NoMapRoomRendersUnknown(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	rooms.Insert(repo.Room{ID: 1, ExternalID: "a", Name: "A"})
	rooms.Insert(repo.Room{ID: 2, ExternalID: "secret", Name: "Secret",
		Flags: repo.RoomFlags{NoMap: true}})
	rooms.Insert(repo.Room{ID: 3, ExternalID: "behind", Name: "Behind"})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 3, Direction: repo.DirNorth})

	out := runMap(t, "5", rooms, exits, 1)
	if !strings.Contains(out, "[?]") {
		t.Errorf("expected NoMap room rendered as [?]; got %q", out)
	}
	// BFS must not recurse through the NoMap room — Behind is unreachable.
	if strings.Contains(out, "[ ]") {
		t.Errorf("BFS leaked through NoMap; rendered an extra visited cell: %q", out)
	}
}

func TestMap_ArgClampingAndBadInput(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	rooms.Insert(repo.Room{ID: 1, ExternalID: "x", Name: "X"})

	// non-numeric: friendly error, no crash
	out := runMap(t, "abc", rooms, exits, 1)
	if !strings.Contains(out, "Usage:") {
		t.Errorf("expected usage hint on bad input; got %q", out)
	}

	// 0 clamps to 1 (renders depth 1)
	out = runMap(t, "0", rooms, exits, 1)
	if !strings.Contains(out, "depth 1") {
		t.Errorf("expected clamped depth 1 in header; got %q", out)
	}

	// 99 clamps to 5
	out = runMap(t, "99", rooms, exits, 1)
	if !strings.Contains(out, "depth 5") {
		t.Errorf("expected clamped depth 5 in header; got %q", out)
	}
}

func TestMap_LegendPresent(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	rooms.Insert(repo.Room{ID: 1, ExternalID: "x", Name: "X"})
	out := runMap(t, "", rooms, exits, 1)
	for _, want := range []string{"= you", "= unmapped", "up/down"} {
		if !strings.Contains(out, want) {
			t.Errorf("legend missing %q; got %q", want, out)
		}
	}
}

func TestMap_EmitsANSIEscapes(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	rooms.Insert(repo.Room{ID: 1, ExternalID: "x", Name: "X"})
	out := runMap(t, "", rooms, exits, 1)
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("output has no ANSI escapes; got %q", out)
	}
}
