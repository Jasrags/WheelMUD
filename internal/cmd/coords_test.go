package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// tCtx returns a background context for cmd-package tests. Local to
// this file rather than shared because tests in this package
// historically inline context.Background(); the helper just keeps the
// repeated calls below quiet.
func tCtx() context.Context { return context.Background() }

// coordsFixture builds a small world for the cmd tests:
//   - id=1 (starter): non-anchor, connected via n exit to r2.
//   - id=2 (north_road): non-anchor.
//   - id=10 (lonely_anchor): explicit anchor with authored coords,
//     in its own connected component so BFS visits it as an anchor
//     but never propagates from it.
//
// With one anchor present (id=10), DeriveCoords does NOT fall back
// to a synthetic anchor at the starter — anchors=1, synthetic=false.
// To exercise the synthetic-anchor path (where the starter seeds the
// BFS) tests pin the starter as an anchor too. See
// coordsFixtureWithStarterAnchor.
func coordsFixture(t *testing.T) (*repo.MemoryRoomRepo, *repo.MemoryExitRepo) {
	t.Helper()
	rr := repo.NewMemoryRoomRepo()
	er := repo.NewMemoryExitRepo()
	ctx := tCtx()
	if _, err := rr.Create(ctx, repo.Room{ID: repo.StarterRoomID, ExternalID: "starter", Name: "Plaza"}); err != nil {
		t.Fatalf("create starter: %v", err)
	}
	if _, err := rr.Create(ctx, repo.Room{ID: 2, ExternalID: "north_road", Name: "North Road"}); err != nil {
		t.Fatalf("create r2: %v", err)
	}
	if _, err := rr.Create(ctx, repo.Room{
		ID: 10, ExternalID: "lonely_anchor", Name: "Lonely Anchor",
		CoordX: 50, CoordY: 50, CoordsAnchor: true,
	}); err != nil {
		t.Fatalf("create r10: %v", err)
	}
	if _, err := er.Create(ctx, repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: "n"}); err != nil {
		t.Fatalf("create exit: %v", err)
	}
	return rr, er
}

// coordsFixtureWithStarterAnchor is the "starter is the anchor"
// scenario — used by tests that want BFS propagation along the n
// edge to actually assign r2's coords.
func coordsFixtureWithStarterAnchor(t *testing.T) (*repo.MemoryRoomRepo, *repo.MemoryExitRepo) {
	t.Helper()
	rr := repo.NewMemoryRoomRepo()
	er := repo.NewMemoryExitRepo()
	ctx := tCtx()
	if _, err := rr.Create(ctx, repo.Room{
		ID: repo.StarterRoomID, ExternalID: "starter", Name: "Plaza",
		CoordsAnchor: true,
	}); err != nil {
		t.Fatalf("create starter: %v", err)
	}
	if _, err := rr.Create(ctx, repo.Room{ID: 2, ExternalID: "north_road", Name: "North Road"}); err != nil {
		t.Fatalf("create r2: %v", err)
	}
	if _, err := er.Create(ctx, repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: "n"}); err != nil {
		t.Fatalf("create exit: %v", err)
	}
	return rr, er
}

func TestCoords_Rebuild_AssignsAndReports(t *testing.T) {
	rr, er := coordsFixtureWithStarterAnchor(t)
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	cmd := NewCoords(rr, er)

	runCmd(t, cmd, s, "rebuild")

	out := conn.String()
	if !strings.Contains(out, "Coord derivation:") {
		t.Errorf("missing summary header: %q", out)
	}
	if !strings.Contains(out, "Assigned:") {
		t.Errorf("missing Assigned line: %q", out)
	}
	// r2 should now sit at (0,1,0) — propagated from synthetic
	// starter anchor via the n exit.
	r2, _ := rr.FindByID(tCtx(), 2)
	if r2.CoordY != 1 {
		t.Errorf("r2.y = %d, want 1 after rebuild", r2.CoordY)
	}
}

func TestCoords_Show_DistinguishesAnchorFromAuto(t *testing.T) {
	rr, er := coordsFixture(t)
	// Run derivation first so r2 has real coords.
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	cmd := NewCoords(rr, er)
	runCmd(t, cmd, s, "rebuild")

	// Reset capture for the show command.
	conn.Reset()
	runCmd(t, cmd, s, "show north_road")
	if !strings.Contains(conn.String(), "auto-derived") {
		t.Errorf("expected 'auto-derived' provenance for r2, got: %q", conn.String())
	}

	conn.Reset()
	runCmd(t, cmd, s, "show lonely_anchor")
	if !strings.Contains(conn.String(), "anchor (authored)") {
		t.Errorf("expected 'anchor (authored)' provenance for r10, got: %q", conn.String())
	}
}

func TestCoords_Show_ByNumericID(t *testing.T) {
	rr, er := coordsFixture(t)
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	cmd := NewCoords(rr, er)

	runCmd(t, cmd, s, "show 10")
	if !strings.Contains(conn.String(), "lonely_anchor") {
		t.Errorf("numeric id lookup miss: %q", conn.String())
	}
}

func TestCoords_Show_MissingRoomReportsCleanly(t *testing.T) {
	rr, er := coordsFixture(t)
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	cmd := NewCoords(rr, er)

	runCmd(t, cmd, s, "show no_such_room")
	if !strings.Contains(conn.String(), "No such room") {
		t.Errorf("missing-room error not surfaced: %q", conn.String())
	}
}

func TestCoords_Issues_ListsOrphans(t *testing.T) {
	rr, er := coordsFixture(t)
	// r10 is an anchor in its own connected component (no exits in
	// or out), so the BFS will visit it as anchor but won't reach it
	// FROM the starter — and the starter won't reach it either. With
	// anchors present, SyntheticAnchor stays false; r10 gets its own
	// anchor visit. To produce an orphan, add a third room with no
	// edges:
	if _, err := rr.Create(tCtx(), repo.Room{ID: 99, ExternalID: "void", Name: "Void"}); err != nil {
		t.Fatalf("create void: %v", err)
	}

	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	cmd := NewCoords(rr, er)

	runCmd(t, cmd, s, "issues")
	if !strings.Contains(conn.String(), "Orphans") {
		t.Errorf("orphan section missing: %q", conn.String())
	}
	if !strings.Contains(conn.String(), "void") {
		t.Errorf("void room not listed in orphans: %q", conn.String())
	}
}

func TestCoords_Issues_HappyPath(t *testing.T) {
	rr := repo.NewMemoryRoomRepo()
	er := repo.NewMemoryExitRepo()
	if _, err := rr.Create(tCtx(), repo.Room{ID: repo.StarterRoomID, ExternalID: "starter", Name: "Plaza"}); err != nil {
		t.Fatalf("create starter: %v", err)
	}
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	cmd := NewCoords(rr, er)

	runCmd(t, cmd, s, "issues")
	if !strings.Contains(conn.String(), "No coord issues") {
		t.Errorf("happy-path summary missing: %q", conn.String())
	}
}

func TestCoords_NoArgs_ShowsUsage(t *testing.T) {
	rr, er := coordsFixture(t)
	s, conn := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	cmd := NewCoords(rr, er)

	runCmd(t, cmd, s, "")
	if !strings.Contains(conn.String(), "Usage:") {
		t.Errorf("usage banner missing: %q", conn.String())
	}
}
