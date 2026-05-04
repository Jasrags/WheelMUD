package world

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// CoordSummary reports what DeriveCoords did. Returned to the caller
// (boot code, admin `coords rebuild`) so it can log progress and
// surface issues without re-walking the graph.
type CoordSummary struct {
	// Anchors is the count of rooms with CoordsAnchor=true that
	// seeded the BFS. If zero, the runner falls back to a synthetic
	// anchor at repo.StarterRoomID (coords 0,0,0).
	Anchors int
	// SyntheticAnchor is true when no builder anchors existed and the
	// starter room was used as a default anchor. Helps callers tell
	// "fresh world, builder hasn't pinned anything yet" from "anchors
	// configured."
	SyntheticAnchor bool
	// Assigned is the count of rooms whose coords were updated in
	// the database. Anchors are never counted here even when their
	// coords already match what the BFS would have written.
	Assigned int
	// Conflicts lists rooms reachable from multiple paths whose BFS-
	// computed coords disagree. First-arrival wins; the conflicting
	// later visit is recorded but not persisted. Empty under the
	// happy path of a strictly grid-aligned graph.
	Conflicts []int64
	// Orphans lists rooms that no BFS reached. They are unreachable
	// from any anchor (or from the starter, in synthetic mode) — most
	// commonly a sub-zone with no exit back to the main world, or a
	// freshly-disconnected component after a deletion. Their coords
	// are left unchanged.
	Orphans []int64
}

// dirDeltas maps each canonical direction code to a unit (dx,dy,dz)
// step. Diagonals combine the cardinal deltas; vertical exits move
// only along z. Diagonal coverage matches the migration-0007
// directional widening (n/s/e/w/u/d/ne/nw/se/sw).
var dirDeltas = map[string][3]int{
	repo.DirNorth:     {0, 1, 0},
	repo.DirSouth:     {0, -1, 0},
	repo.DirEast:      {1, 0, 0},
	repo.DirWest:      {-1, 0, 0},
	repo.DirUp:        {0, 0, 1},
	repo.DirDown:      {0, 0, -1},
	repo.DirNortheast: {1, 1, 0},
	repo.DirNorthwest: {-1, 1, 0},
	repo.DirSoutheast: {1, -1, 0},
	repo.DirSouthwest: {-1, -1, 0},
}

// DeriveCoords walks the room graph by BFS from every CoordsAnchor
// room and assigns (x,y,z) to non-anchor rooms based on cardinal,
// diagonal, and vertical exit deltas.
//
// Algorithm:
//
//  1. Load the room set in id order. Anchor selection is therefore
//     deterministic across boots.
//  2. Anchors = rooms with CoordsAnchor==true. If none, the starter
//     room (repo.StarterRoomID) becomes a synthetic anchor at (0,0,0).
//     This is the bootstrap path for a fresh world the builder hasn't
//     pinned.
//  3. Initialize the BFS with every anchor's existing coords. Walk
//     every cardinal/diagonal/vertical exit; for each unvisited
//     destination, compute coords as src + dirDelta, persist via
//     UpdateCoords if the destination is non-anchor (CoordsAnchor=false),
//     and enqueue.
//  4. If an already-visited destination is reached again with
//     conflicting coords, record it in Conflicts but don't overwrite
//     (first-arrival wins). Anchor destinations are never in
//     Conflicts even when they disagree with the propagation; they
//     are anchors.
//  5. Rooms not reached at all are Orphans.
//
// The runner only writes for rooms with CoordsAnchor==false. Anchors
// are read but never overwritten; their coords drive propagation but
// stay frozen on disk. The runner is idempotent: a no-op rebuild on a
// stable world returns Assigned=0.
func DeriveCoords(ctx context.Context, rooms repo.RoomRepo, exits repo.ExitRepo) (CoordSummary, error) {
	all, err := rooms.ListAll(ctx)
	if err != nil {
		return CoordSummary{}, fmt.Errorf("list rooms: %w", err)
	}
	if len(all) == 0 {
		return CoordSummary{}, nil
	}

	byID := make(map[int64]repo.Room, len(all))
	for _, r := range all {
		byID[r.ID] = r
	}

	// Anchor pass: collect explicitly-flagged anchors. If none, the
	// starter is the default. Sort anchor ids so the BFS order (and
	// therefore which path "wins" for a contested room) is stable
	// across boots.
	type anchor struct {
		id      int64
		x, y, z int
	}
	var anchors []anchor
	for _, r := range all {
		if r.CoordsAnchor {
			anchors = append(anchors, anchor{r.ID, r.CoordX, r.CoordY, r.CoordZ})
		}
	}
	summary := CoordSummary{Anchors: len(anchors)}
	if len(anchors) == 0 {
		// Synthetic anchor at the starter. If the starter doesn't
		// exist (degenerate test fixture), we have nothing to anchor
		// to and every room becomes an orphan.
		if starter, ok := byID[repo.StarterRoomID]; ok {
			anchors = append(anchors, anchor{starter.ID, 0, 0, 0})
			summary.SyntheticAnchor = true
		}
	}
	sort.Slice(anchors, func(i, j int) bool { return anchors[i].id < anchors[j].id })

	type pos struct {
		x, y, z int
	}
	visited := make(map[int64]pos, len(all))
	queue := make([]int64, 0, len(all))

	for _, a := range anchors {
		// If two anchors disagree on coords for a single id (impossible
		// today; guarded so a future "alias" feature can't blow up),
		// the lower id wins per the deterministic sort above.
		if _, seen := visited[a.id]; seen {
			continue
		}
		visited[a.id] = pos{a.x, a.y, a.z}
		queue = append(queue, a.id)
	}

	for head := 0; head < len(queue); head++ {
		fromID := queue[head]
		fromPos := visited[fromID]
		fromExits, err := exits.ListFrom(ctx, fromID)
		if err != nil {
			return CoordSummary{}, fmt.Errorf("list exits from %d: %w", fromID, err)
		}
		for _, e := range fromExits {
			delta, ok := dirDeltas[e.Direction]
			if !ok {
				// Unknown direction code shouldn't reach the DB
				// (CHECK constraint in 0007), but skip rather than
				// abort if it ever does.
				slog.Warn("coords_derive: unknown direction, skipping",
					"from_room_id", fromID, "direction", e.Direction)
				continue
			}
			next := pos{fromPos.x + delta[0], fromPos.y + delta[1], fromPos.z + delta[2]}
			toID := e.ToRoomID
			if existing, seen := visited[toID]; seen {
				if existing != next {
					summary.Conflicts = append(summary.Conflicts, toID)
				}
				continue
			}
			visited[toID] = next
			// Persist only for non-anchor rooms. Anchors propagate
			// but never get overwritten.
			if dest, ok := byID[toID]; ok && !dest.CoordsAnchor {
				if dest.CoordX != next.x || dest.CoordY != next.y || dest.CoordZ != next.z {
					if err := rooms.UpdateCoords(ctx, toID, next.x, next.y, next.z); err != nil {
						return CoordSummary{}, fmt.Errorf("update coords for %d: %w", toID, err)
					}
					summary.Assigned++
				}
			}
			queue = append(queue, toID)
		}
	}

	// Orphan pass: rooms in the world that the BFS never reached.
	// Their coords are left untouched; admin `coords issues` surfaces
	// them so a builder can investigate (typically a sub-zone with
	// no exit, or a freshly-disconnected component).
	for id := range byID {
		if _, seen := visited[id]; !seen {
			summary.Orphans = append(summary.Orphans, id)
		}
	}
	sort.Slice(summary.Orphans, func(i, j int) bool { return summary.Orphans[i] < summary.Orphans[j] })
	sort.Slice(summary.Conflicts, func(i, j int) bool { return summary.Conflicts[i] < summary.Conflicts[j] })
	// Conflict ids may repeat if multiple exits arrive at the same
	// already-visited room with different coords. Dedupe so the count
	// matches "rooms with disagreements", not "edge events".
	summary.Conflicts = dedupSortedInt64(summary.Conflicts)
	return summary, nil
}

func dedupSortedInt64(in []int64) []int64 {
	if len(in) <= 1 {
		return in
	}
	out := in[:1]
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
