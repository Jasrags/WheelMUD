package mob

import (
	"context"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// bfsConfig bundles the per-call inputs to a BFS over the world's
// walkable, in-zone exit graph. The handler-level repos are stable
// across calls; this struct carries the per-pulse state.
type bfsConfig struct {
	exits    repo.ExitRepo
	zoneOf   func(ctx context.Context, roomID int64) (int64, bool)
	fromRoom int64 // BFS origin
	zoneID   int64 // gate: only rooms inside this zone are reachable
	maxHops  int   // 0 disables; capped at bfsHopHardCap below
}

// bfsHopHardCap pins the maximum BFS depth even when a misbehaving
// caller passes a larger maxHops. The runtime check defends against
// a templates-table typo that the migration's [0, 256] clamp didn't
// catch (e.g. an admin SQL update).
const bfsHopHardCap = 64

// bfsReachable returns the set of room IDs reachable from cfg.fromRoom
// via walkable in-zone exits within cfg.maxHops hops, excluding
// fromRoom itself. The traversal is depth-first-fair (BFS by hop
// level) so callers can interpret the slice as "any of these is at
// most maxHops away". Iteration order is BFS-deterministic given a
// stable ExitRepo.ListFrom ordering — i.e. callers that need
// reproducible target selection should seed their RNG, not rely on
// map iteration.
func bfsReachable(ctx context.Context, cfg bfsConfig) ([]int64, error) {
	if cfg.maxHops <= 0 || cfg.fromRoom == 0 {
		return nil, nil
	}
	if cfg.maxHops > bfsHopHardCap {
		cfg.maxHops = bfsHopHardCap
	}
	visited := map[int64]bool{cfg.fromRoom: true}
	queue := []int64{cfg.fromRoom}
	var out []int64
	for hop := 0; hop < cfg.maxHops && len(queue) > 0; hop++ {
		next := queue[:0:0]
		for _, room := range queue {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			exits, err := cfg.exits.ListFrom(ctx, room)
			if err != nil {
				return nil, err
			}
			for _, e := range exits {
				if !exitWalkable(e) || visited[e.ToRoomID] {
					continue
				}
				dz, ok := cfg.zoneOf(ctx, e.ToRoomID)
				if !ok || dz != cfg.zoneID {
					continue
				}
				visited[e.ToRoomID] = true
				out = append(out, e.ToRoomID)
				next = append(next, e.ToRoomID)
			}
		}
		queue = next
	}
	return out, nil
}

// bfsNextStep finds the shortest walkable in-zone path from fromRoom
// to targetRoom and returns the exit the mob should take to advance
// one hop. found=false means no path exists within maxHops, or the
// target is the same as fromRoom (mob has already arrived).
func bfsNextStep(ctx context.Context, cfg bfsConfig, targetRoom int64) (repo.Exit, bool, error) {
	if targetRoom == 0 || cfg.fromRoom == 0 || targetRoom == cfg.fromRoom {
		return repo.Exit{}, false, nil
	}
	if cfg.maxHops <= 0 {
		return repo.Exit{}, false, nil
	}
	if cfg.maxHops > bfsHopHardCap {
		cfg.maxHops = bfsHopHardCap
	}
	// parent[roomID] = the (parentRoomID, edge) that first reached
	// roomID. Reconstruct the path by walking parents from target
	// back to source; the *first edge after source* is the next step.
	parents := map[int64]crumb{cfg.fromRoom: {}}
	queue := []int64{cfg.fromRoom}
	for hop := 0; hop < cfg.maxHops && len(queue) > 0; hop++ {
		next := queue[:0:0]
		for _, room := range queue {
			if ctx.Err() != nil {
				return repo.Exit{}, false, ctx.Err()
			}
			exits, err := cfg.exits.ListFrom(ctx, room)
			if err != nil {
				return repo.Exit{}, false, err
			}
			for _, e := range exits {
				if !exitWalkable(e) {
					continue
				}
				if _, seen := parents[e.ToRoomID]; seen {
					continue
				}
				dz, ok := cfg.zoneOf(ctx, e.ToRoomID)
				if !ok || dz != cfg.zoneID {
					continue
				}
				parents[e.ToRoomID] = crumb{parent: room, edge: e}
				if e.ToRoomID == targetRoom {
					return reconstructFirstEdge(parents, cfg.fromRoom, targetRoom), true, nil
				}
				next = append(next, e.ToRoomID)
			}
		}
		queue = next
	}
	return repo.Exit{}, false, nil
}

// reconstructFirstEdge walks the BFS parent map from target back to
// source and returns the first edge taken from source. Caller has
// already confirmed target is in parents (a real path exists).
func reconstructFirstEdge(parents map[int64]crumb, source, target int64) repo.Exit {
	cur := target
	for {
		c := parents[cur]
		if c.parent == source {
			return c.edge
		}
		cur = c.parent
	}
}

// crumb is the BFS predecessor record. Hoisted out of bfsNextStep so
// reconstructFirstEdge can name the type without redeclaring it.
type crumb struct {
	parent int64
	edge   repo.Exit
}
