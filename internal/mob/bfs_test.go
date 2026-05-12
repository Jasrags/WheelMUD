package mob

import (
	"context"
	"sort"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// graphFixture stages a small in-memory exit graph for bfs_test.go.
// Topology (all bidirectional, all in zone 1):
//
//	1 -- 2 -- 3 -- 5
//	     |
//	     4
//
// Room 6 lives in zone 2, reachable from room 5 via a cross-zone exit
// so we can prove BFS refuses to leave the zone.
type graphFixture struct {
	exits  *repo.MemoryExitRepo
	zoneOf func(ctx context.Context, roomID int64) (int64, bool)
}

func newGraphFixture() *graphFixture {
	r := repo.NewMemoryRoomRepo()
	r.Insert(repo.Room{ID: 1, ZoneID: 1})
	r.Insert(repo.Room{ID: 2, ZoneID: 1})
	r.Insert(repo.Room{ID: 3, ZoneID: 1})
	r.Insert(repo.Room{ID: 4, ZoneID: 1})
	r.Insert(repo.Room{ID: 5, ZoneID: 1})
	r.Insert(repo.Room{ID: 6, ZoneID: 2}) // cross-zone

	e := repo.NewMemoryExitRepo()
	add := func(from, to int64, dir string) {
		e.Insert(repo.Exit{FromRoomID: from, ToRoomID: to, Direction: dir})
	}
	add(1, 2, repo.DirEast)
	add(2, 1, repo.DirWest)
	add(2, 3, repo.DirEast)
	add(3, 2, repo.DirWest)
	add(3, 5, repo.DirEast)
	add(5, 3, repo.DirWest)
	add(2, 4, repo.DirSouth)
	add(4, 2, repo.DirNorth)
	add(5, 6, repo.DirEast) // cross-zone
	add(6, 5, repo.DirWest)

	cache := map[int64]int64{}
	return &graphFixture{
		exits: e,
		zoneOf: func(ctx context.Context, roomID int64) (int64, bool) {
			if z, ok := cache[roomID]; ok {
				return z, true
			}
			room, err := r.FindByID(ctx, roomID)
			if err != nil {
				return 0, false
			}
			cache[roomID] = room.ZoneID
			return room.ZoneID, true
		},
	}
}

func sortedInt64(in []int64) []int64 {
	out := append([]int64(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func TestBFSReachable_DepthCap(t *testing.T) {
	f := newGraphFixture()
	ctx := context.Background()

	t.Run("radius_1_one_neighbor", func(t *testing.T) {
		got, err := bfsReachable(ctx, bfsConfig{
			exits: f.exits, zoneOf: f.zoneOf,
			fromRoom: 1, zoneID: 1, maxHops: 1,
		})
		if err != nil {
			t.Fatalf("bfsReachable: %v", err)
		}
		want := []int64{2}
		if got := sortedInt64(got); !equalInt64Slice(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("radius_2_three_neighbors", func(t *testing.T) {
		got, err := bfsReachable(ctx, bfsConfig{
			exits: f.exits, zoneOf: f.zoneOf,
			fromRoom: 1, zoneID: 1, maxHops: 2,
		})
		if err != nil {
			t.Fatalf("bfsReachable: %v", err)
		}
		want := []int64{2, 3, 4}
		if got := sortedInt64(got); !equalInt64Slice(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("radius_4_all_in_zone", func(t *testing.T) {
		got, err := bfsReachable(ctx, bfsConfig{
			exits: f.exits, zoneOf: f.zoneOf,
			fromRoom: 1, zoneID: 1, maxHops: 4,
		})
		if err != nil {
			t.Fatalf("bfsReachable: %v", err)
		}
		// Room 6 is cross-zone; must NOT appear.
		want := []int64{2, 3, 4, 5}
		if got := sortedInt64(got); !equalInt64Slice(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
}

func TestBFSReachable_ZeroHopsEmpty(t *testing.T) {
	f := newGraphFixture()
	got, err := bfsReachable(context.Background(), bfsConfig{
		exits: f.exits, zoneOf: f.zoneOf,
		fromRoom: 1, zoneID: 1, maxHops: 0,
	})
	if err != nil {
		t.Fatalf("bfsReachable: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestBFSReachable_BlockedExitsExcluded(t *testing.T) {
	f := newGraphFixture()
	// Close the 1→2 exit. Room 1 is now isolated within zone 1.
	f.exits = repo.NewMemoryExitRepo()
	f.exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirEast, Flags: repo.ExitFlags{Closed: true}})

	got, err := bfsReachable(context.Background(), bfsConfig{
		exits: f.exits, zoneOf: f.zoneOf,
		fromRoom: 1, zoneID: 1, maxHops: 3,
	})
	if err != nil {
		t.Fatalf("bfsReachable: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty (only exit is closed)", got)
	}
}

func TestBFSNextStep_ShortestPath(t *testing.T) {
	f := newGraphFixture()
	ctx := context.Background()

	t.Run("one_hop_returns_direct_exit", func(t *testing.T) {
		step, found, err := bfsNextStep(ctx, bfsConfig{
			exits: f.exits, zoneOf: f.zoneOf,
			fromRoom: 1, zoneID: 1, maxHops: 3,
		}, 2)
		if err != nil || !found {
			t.Fatalf("bfsNextStep: found=%v err=%v", found, err)
		}
		if step.ToRoomID != 2 {
			t.Fatalf("step.ToRoomID = %d, want 2", step.ToRoomID)
		}
	})

	t.Run("two_hops_returns_first_edge", func(t *testing.T) {
		// 1 → 3 must go through 2; the first edge is the 1→2 exit.
		step, found, err := bfsNextStep(ctx, bfsConfig{
			exits: f.exits, zoneOf: f.zoneOf,
			fromRoom: 1, zoneID: 1, maxHops: 3,
		}, 3)
		if err != nil || !found {
			t.Fatalf("bfsNextStep: found=%v err=%v", found, err)
		}
		if step.ToRoomID != 2 {
			t.Fatalf("first hop = %d, want 2 (path 1→2→3)", step.ToRoomID)
		}
	})

	t.Run("self_target_returns_not_found", func(t *testing.T) {
		_, found, err := bfsNextStep(ctx, bfsConfig{
			exits: f.exits, zoneOf: f.zoneOf,
			fromRoom: 1, zoneID: 1, maxHops: 3,
		}, 1)
		if err != nil || found {
			t.Fatalf("self-target: found=%v err=%v", found, err)
		}
	})

	t.Run("unreachable_target_returns_not_found", func(t *testing.T) {
		// Room 6 lives in zone 2; BFS within zone 1 can't reach it.
		_, found, err := bfsNextStep(ctx, bfsConfig{
			exits: f.exits, zoneOf: f.zoneOf,
			fromRoom: 1, zoneID: 1, maxHops: 8,
		}, 6)
		if err != nil || found {
			t.Fatalf("cross-zone target: found=%v err=%v", found, err)
		}
	})

	t.Run("target_outside_radius_returns_not_found", func(t *testing.T) {
		// Room 5 is 3 hops from 1; radius=2 stops before reaching it.
		_, found, err := bfsNextStep(ctx, bfsConfig{
			exits: f.exits, zoneOf: f.zoneOf,
			fromRoom: 1, zoneID: 1, maxHops: 2,
		}, 5)
		if err != nil || found {
			t.Fatalf("radius=2 target=5: found=%v err=%v", found, err)
		}
	})
}

func equalInt64Slice(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
