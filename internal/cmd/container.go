package cmd

import (
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// defaultContainerDepth is the global cap on how deep one container
// chain can nest (bag in pack in chest = 3). Per-item ContainerStats
// can lower this, but never raise it. ContainerStats.DepthCap of 0
// means "use this default" — matches the field's existing comment in
// internal/repo/item.go.
const defaultContainerDepth = 3

// childrenOf indexes a flat item slice by parent_item_id so the
// recursive helpers can walk subtrees in O(children) instead of
// rescanning the slice for every node.
func childrenOf(items []repo.Item) map[int64][]repo.Item {
	idx := make(map[int64][]repo.Item, len(items))
	for _, it := range items {
		if it.ParentItemID != 0 {
			idx[it.ParentItemID] = append(idx[it.ParentItemID], it)
		}
	}
	return idx
}

// recursiveWeight returns the effective weight of `it` including its
// transitive contents. ContainerStats.WeightMult applies to the
// weight of everything inside that container subtree (not the
// container's own shell). A bag-of-holding with WeightMult 0.1
// reduces the apparent weight of its contents by 90%; nested bags
// compound (a bag-of-holding inside another bag-of-holding gives
// 0.01x). idx may be nil — childrenOf gets called once at the
// outermost recursive entry and reused.
func recursiveWeight(it repo.Item, idx map[int64][]repo.Item) float64 {
	w := it.Weight
	mult := 1.0
	if cs, ok := it.Stats.(*repo.ContainerStats); ok && cs != nil && cs.WeightMult > 0 {
		mult = cs.WeightMult
	}
	for _, child := range idx[it.ID] {
		w += mult * recursiveWeight(child, idx)
	}
	return w
}

// rawWeight returns the real-world weight of an item plus everything
// transitively inside it, with no WeightMult applied anywhere. This
// is what a container's CapacityLbs gates against — the bag-of-
// holding fantasy is "real cap, easier to carry," so the cap is in
// real pounds even though the carrier feels less. WeightMult only
// affects encumbrance via recursiveWeight.
func rawWeight(it repo.Item, idx map[int64][]repo.Item) float64 {
	w := it.Weight
	for _, child := range idx[it.ID] {
		w += rawWeight(child, idx)
	}
	return w
}

// rawContentsWeight is rawWeight summed over the container's direct
// children (the shell itself isn't counted). Used by capacity gates.
func rawContentsWeight(container repo.Item, idx map[int64][]repo.Item) float64 {
	var w float64
	for _, child := range idx[container.ID] {
		w += rawWeight(child, idx)
	}
	return w
}

// depthBelow returns the longest chain length under itemID. A leaf
// item (or non-container) returns 0; a container holding two leaves
// returns 1; a container holding a container holding a leaf returns 2.
func depthBelow(itemID int64, idx map[int64][]repo.Item) int {
	max := 0
	for _, child := range idx[itemID] {
		d := 1 + depthBelow(child.ID, idx)
		if d > max {
			max = d
		}
	}
	return max
}

// depthOf returns the depth of an item (number of container ancestors)
// by walking parent_item_id up. Top-level items return 0. byID is a
// flat lookup so the walk doesn't reissue queries.
func depthOf(itemID int64, byID map[int64]repo.Item) int {
	depth := 0
	cur := itemID
	// Bound the walk to defend against an accidental cycle in the
	// data layer. defaultContainerDepth*2 is generous and cheap.
	for i := 0; i < defaultContainerDepth*4; i++ {
		it, ok := byID[cur]
		if !ok || it.ParentItemID == 0 {
			return depth
		}
		depth++
		cur = it.ParentItemID
	}
	return depth
}

// isAncestor reports whether `ancestor` appears anywhere on the
// parent chain of `start`. Used to prevent A→B→A cycles when a
// player tries to put a container into one of its own descendants.
func isAncestor(ancestor, start int64, byID map[int64]repo.Item) bool {
	cur := start
	for i := 0; i < defaultContainerDepth*4; i++ {
		it, ok := byID[cur]
		if !ok {
			return false
		}
		if it.ParentItemID == 0 {
			return false
		}
		if it.ParentItemID == ancestor {
			return true
		}
		cur = it.ParentItemID
	}
	return false
}

// putRefusal categorizes why a put was rejected. The verb layer maps
// each into a specific player-facing line.
type putRefusal int

const (
	putOK putRefusal = iota
	putNotAContainer
	putLiquidContainer
	putSelf
	putCycle
	putTooDeep
	putTooHeavy
	putNoStats
)

// canPut decides whether `child` can be placed inside `container`.
// `allOwned` is the carrier's full transitive owned slice (top-level
// + nested) so depth + capacity checks see the whole picture; `byID`
// is its lookup index. If the destination container is on a room
// floor, callers can still call this — the depth/capacity logic only
// reads the container subtree, which is independent of where the
// container itself is reachable from.
func canPut(container, child repo.Item, allKnown []repo.Item, byID map[int64]repo.Item) putRefusal {
	if container.Type != repo.ItemTypeContainer {
		return putNotAContainer
	}
	cs, ok := container.Stats.(*repo.ContainerStats)
	if !ok || cs == nil {
		return putNoStats
	}
	if cs.LiquidPints > 0 {
		// Liquid container — solid items don't fit. Pour/fill is a
		// follow-up slice.
		return putLiquidContainer
	}
	if container.ID == child.ID {
		return putSelf
	}
	if isAncestor(child.ID, container.ID, byID) {
		// container is somewhere below child — putting child in
		// would close a loop.
		return putCycle
	}

	idx := childrenOf(allKnown)

	// Depth check. Effective cap is min(stats.DepthCap, default) — but
	// 0 on stats means "use default", so:
	maxDepth := defaultContainerDepth
	if cs.DepthCap > 0 && cs.DepthCap < maxDepth {
		maxDepth = cs.DepthCap
	}
	// depthOf(container) is how deep the container itself sits below
	// some root; +1 for the new child slot; +depthBelow(child) for
	// any subtree the child brings with it.
	if depthOf(container.ID, byID)+1+depthBelow(child.ID, idx) > maxDepth {
		return putTooDeep
	}

	// Capacity check. CapacityLbs is the bag's real-world weight
	// limit; WeightMult does NOT enlarge it (a bag-of-holding still
	// only holds its stated volume — it just feels lighter to the
	// carrier). A 0 capacity means "no cap" — the YAML schema treats
	// unset numeric fields as 0, and a hard refusal there would
	// surprise builders.
	if cs.CapacityLbs > 0 {
		if rawContentsWeight(container, idx)+rawWeight(child, idx) > cs.CapacityLbs {
			return putTooHeavy
		}
	}

	return putOK
}
