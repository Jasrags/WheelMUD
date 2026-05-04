package world

import (
	"context"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// fixture builds a world graph from a compact spec. Each room becomes
// a repo.Room in the memory repos; each exit gets registered too.
// Anchor rooms set CoordsAnchor=true with explicit coords; non-anchor
// rooms start at (0,0,0) and rely on the runner to fill them in.
type fixtureRoom struct {
	id      int64
	anchor  bool
	x, y, z int
}

type fixtureExit struct {
	from, to int64
	dir      string
}

func buildFixture(t *testing.T, rooms []fixtureRoom, edges []fixtureExit) (*repo.MemoryRoomRepo, *repo.MemoryExitRepo) {
	t.Helper()
	rr := repo.NewMemoryRoomRepo()
	er := repo.NewMemoryExitRepo()
	ctx := context.Background()
	for _, fr := range rooms {
		room := repo.Room{
			ID:           fr.id,
			ExternalID:   shortID(fr.id),
			Name:         shortID(fr.id),
			CoordX:       fr.x,
			CoordY:       fr.y,
			CoordZ:       fr.z,
			CoordsAnchor: fr.anchor,
		}
		if _, err := rr.Create(ctx, room); err != nil {
			t.Fatalf("create room %d: %v", fr.id, err)
		}
	}
	for _, fe := range edges {
		if _, err := er.Create(ctx, repo.Exit{
			FromRoomID: fe.from,
			ToRoomID:   fe.to,
			Direction:  fe.dir,
		}); err != nil {
			t.Fatalf("create exit %d->%d %s: %v", fe.from, fe.to, fe.dir, err)
		}
	}
	return rr, er
}

func shortID(id int64) string {
	return "r" + itoa(id)
}

func itoa(id int64) string {
	if id == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for id > 0 {
		i--
		b[i] = byte('0' + id%10)
		id /= 10
	}
	return string(b[i:])
}

// TestDeriveCoords_LinearChain: starter at (0,0,0), n→r2, e→r3.
// After derivation, r2=(0,1,0), r3=(1,0,0). Synthetic anchor path.
func TestDeriveCoords_LinearChain(t *testing.T) {
	rr, er := buildFixture(t, []fixtureRoom{
		{id: repo.StarterRoomID},
		{id: 2},
		{id: 3},
	}, []fixtureExit{
		{from: 1, to: 2, dir: "n"},
		{from: 2, to: 1, dir: "s"},
		{from: 1, to: 3, dir: "e"},
		{from: 3, to: 1, dir: "w"},
	})

	sum, err := DeriveCoords(context.Background(), rr, er)
	if err != nil {
		t.Fatalf("DeriveCoords: %v", err)
	}
	if !sum.SyntheticAnchor {
		t.Errorf("SyntheticAnchor = false, want true (no explicit anchors)")
	}
	if sum.Assigned != 2 {
		t.Errorf("Assigned = %d, want 2", sum.Assigned)
	}
	r2, _ := rr.FindByID(context.Background(), 2)
	if r2.CoordX != 0 || r2.CoordY != 1 {
		t.Errorf("r2 coords = (%d,%d,%d), want (0,1,0)", r2.CoordX, r2.CoordY, r2.CoordZ)
	}
	r3, _ := rr.FindByID(context.Background(), 3)
	if r3.CoordX != 1 || r3.CoordY != 0 {
		t.Errorf("r3 coords = (%d,%d,%d), want (1,0,0)", r3.CoordX, r3.CoordY, r3.CoordZ)
	}
}

// TestDeriveCoords_AnchorPropagation: an explicit anchor at non-zero
// origin propagates to its neighbours.
func TestDeriveCoords_AnchorPropagation(t *testing.T) {
	rr, er := buildFixture(t, []fixtureRoom{
		{id: 1, anchor: true, x: 10, y: 20, z: 0},
		{id: 2},
		{id: 3},
	}, []fixtureExit{
		{from: 1, to: 2, dir: "n"},
		{from: 1, to: 3, dir: "u"},
	})
	sum, err := DeriveCoords(context.Background(), rr, er)
	if err != nil {
		t.Fatalf("DeriveCoords: %v", err)
	}
	if sum.Anchors != 1 || sum.SyntheticAnchor {
		t.Errorf("anchors=%d synthetic=%v, want 1/false", sum.Anchors, sum.SyntheticAnchor)
	}
	r2, _ := rr.FindByID(context.Background(), 2)
	if r2.CoordX != 10 || r2.CoordY != 21 || r2.CoordZ != 0 {
		t.Errorf("r2 = (%d,%d,%d), want (10,21,0)", r2.CoordX, r2.CoordY, r2.CoordZ)
	}
	r3, _ := rr.FindByID(context.Background(), 3)
	if r3.CoordZ != 1 {
		t.Errorf("r3.z = %d, want 1 (up from anchor z=0)", r3.CoordZ)
	}
}

// TestDeriveCoords_AnchorNotOverwritten: an anchor reached via BFS
// from another anchor keeps its authored coords, even when the BFS
// path would assign different ones.
func TestDeriveCoords_AnchorNotOverwritten(t *testing.T) {
	rr, er := buildFixture(t, []fixtureRoom{
		{id: 1, anchor: true, x: 0, y: 0, z: 0},
		{id: 2, anchor: true, x: 99, y: 99, z: 99}, // wildly off the grid
	}, []fixtureExit{
		{from: 1, to: 2, dir: "n"},
		{from: 2, to: 1, dir: "s"},
	})
	if _, err := DeriveCoords(context.Background(), rr, er); err != nil {
		t.Fatalf("DeriveCoords: %v", err)
	}
	r2, _ := rr.FindByID(context.Background(), 2)
	if r2.CoordX != 99 || r2.CoordY != 99 || r2.CoordZ != 99 {
		t.Errorf("anchor r2 coords overwritten: got (%d,%d,%d), want (99,99,99)",
			r2.CoordX, r2.CoordY, r2.CoordZ)
	}
}

// TestDeriveCoords_FirstArrivalWinsConflictReported: a graph cycle
// where two paths give the same room different coords. First-arrival
// keeps the room's coord; the conflicted later path is reported.
//
// Layout (ids):  1 -n-> 2
//                |       |
//                e       e
//                v       v
//                3 -n-> 4
//
// Path A: 1 → n → 2 → e → 4   gives 4 = (1, 1, 0)
// Path B: 1 → e → 3 → n → 4   gives 4 = (1, 1, 0)
// These agree, so this graph is grid-aligned. To force a conflict,
// add a non-grid cycle: 1 → n → 2 → s → 5; 1 → s → 5. After (1→n)
// then (2→s), 5 should be at (0,0,0); but (1→s) gives 5=(0,-1,0).
// First arrival from BFS layer 1 wins.
func TestDeriveCoords_FirstArrivalWinsConflictReported(t *testing.T) {
	rr, er := buildFixture(t, []fixtureRoom{
		{id: 1}, {id: 2}, {id: 5},
	}, []fixtureExit{
		{from: 1, to: 2, dir: "n"},
		{from: 2, to: 5, dir: "s"}, // back to (0,0,0) by delta math
		{from: 1, to: 5, dir: "s"}, // direct: gives (0,-1,0)
	})
	sum, err := DeriveCoords(context.Background(), rr, er)
	if err != nil {
		t.Fatalf("DeriveCoords: %v", err)
	}
	if len(sum.Conflicts) != 1 || sum.Conflicts[0] != 5 {
		t.Errorf("Conflicts = %v, want [5]", sum.Conflicts)
	}
	r5, _ := rr.FindByID(context.Background(), 5)
	// BFS layer 1 visits both n and s before queue advances; map
	// iteration over fromExits is sorted by direction (ListFrom
	// contract), so 'n' fires before 's'. After 1→n we enqueue 2;
	// after 1→s we visit 5 with coord (0,-1,0) and persist it.
	// Then 2 dequeues, walks 2→s, computes (0,0,0) for 5, finds
	// it already visited at (0,-1,0), records the conflict.
	if r5.CoordY != -1 {
		t.Errorf("r5.y = %d, want -1 (first-arrival via 1→s)", r5.CoordY)
	}
}

// TestDeriveCoords_OrphanReported: a room with no exits in or out
// is unreachable from any anchor and gets reported.
func TestDeriveCoords_OrphanReported(t *testing.T) {
	rr, er := buildFixture(t, []fixtureRoom{
		{id: 1}, {id: 2}, {id: 99}, // 99 has no edges
	}, []fixtureExit{
		{from: 1, to: 2, dir: "n"},
	})
	sum, err := DeriveCoords(context.Background(), rr, er)
	if err != nil {
		t.Fatalf("DeriveCoords: %v", err)
	}
	if len(sum.Orphans) != 1 || sum.Orphans[0] != 99 {
		t.Errorf("Orphans = %v, want [99]", sum.Orphans)
	}
}

// TestDeriveCoords_Idempotent: a second run on a stable world is a
// no-op (Assigned=0).
func TestDeriveCoords_Idempotent(t *testing.T) {
	rr, er := buildFixture(t, []fixtureRoom{
		{id: 1}, {id: 2},
	}, []fixtureExit{
		{from: 1, to: 2, dir: "n"},
	})
	if _, err := DeriveCoords(context.Background(), rr, er); err != nil {
		t.Fatalf("first run: %v", err)
	}
	sum, err := DeriveCoords(context.Background(), rr, er)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if sum.Assigned != 0 {
		t.Errorf("Assigned on rerun = %d, want 0 (idempotent)", sum.Assigned)
	}
}

// TestDeriveCoords_EmptyWorld: no rooms returns a zero summary
// without erroring.
func TestDeriveCoords_EmptyWorld(t *testing.T) {
	rr := repo.NewMemoryRoomRepo()
	er := repo.NewMemoryExitRepo()
	sum, err := DeriveCoords(context.Background(), rr, er)
	if err != nil {
		t.Fatalf("DeriveCoords: %v", err)
	}
	if sum.Assigned != 0 || sum.Anchors != 0 || sum.SyntheticAnchor {
		t.Errorf("empty-world summary = %+v, want zero", sum)
	}
}
