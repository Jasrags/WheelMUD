package cmd

import (
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// makeBag returns a container item with the given capacity / depth /
// mult so each test can spell out the relevant axis cleanly.
func makeBag(id int64, name string, capLbs float64, depthCap int, mult float64) repo.Item {
	return repo.Item{
		ID:   id,
		Name: name,
		Type: repo.ItemTypeContainer,
		Stats: &repo.ContainerStats{
			CapacityLbs: capLbs,
			DepthCap:    depthCap,
			WeightMult:  mult,
		},
	}
}

func makeLeaf(id int64, name string, weight float64, parent int64) repo.Item {
	return repo.Item{
		ID:           id,
		Name:         name,
		Weight:       weight,
		ParentItemID: parent,
	}
}

func TestRecursiveWeight_NoMult(t *testing.T) {
	bag := makeBag(1, "sack", 50, 0, 1.0)
	rock := makeLeaf(2, "rock", 5, 1)
	gem := makeLeaf(3, "gem", 1, 1)
	idx := childrenOf([]repo.Item{rock, gem})
	got := recursiveWeight(bag, idx)
	if got != 6 {
		t.Fatalf("recursiveWeight = %v, want 6", got)
	}
}

func TestRecursiveWeight_WeightMultCompounds(t *testing.T) {
	// outer 0.5 mult, inner 0.5 mult. Leaf weight 4 should appear as
	// 4 * 0.5 * 0.5 = 1 to the outermost carrier.
	outer := makeBag(1, "outer", 0, 0, 0.5)
	inner := makeBag(2, "inner", 0, 0, 0.5)
	inner.ParentItemID = 1
	leaf := makeLeaf(3, "ingot", 4, 2)
	idx := childrenOf([]repo.Item{outer, inner, leaf})
	got := recursiveWeight(outer, idx)
	if got != 1.0 {
		t.Fatalf("recursiveWeight = %v, want 1.0", got)
	}
}

func TestDepthOfAndDepthBelow(t *testing.T) {
	a := makeBag(1, "chest", 0, 0, 1)
	b := makeBag(2, "pack", 0, 0, 1)
	b.ParentItemID = 1
	c := makeBag(3, "pouch", 0, 0, 1)
	c.ParentItemID = 2
	leaf := makeLeaf(4, "coin", 0.1, 3)

	all := []repo.Item{a, b, c, leaf}
	byID := map[int64]repo.Item{1: a, 2: b, 3: c, 4: leaf}
	idx := childrenOf(all)

	if d := depthOf(c.ID, byID); d != 2 {
		t.Errorf("depthOf(pouch) = %d, want 2", d)
	}
	if d := depthOf(a.ID, byID); d != 0 {
		t.Errorf("depthOf(chest) = %d, want 0", d)
	}
	if d := depthBelow(a.ID, idx); d != 3 {
		t.Errorf("depthBelow(chest) = %d, want 3", d)
	}
	if d := depthBelow(leaf.ID, idx); d != 0 {
		t.Errorf("depthBelow(leaf) = %d, want 0", d)
	}
}

func TestCanPut_CapacityAndDepth(t *testing.T) {
	tests := []struct {
		name      string
		container repo.Item
		child     repo.Item
		all       []repo.Item
		want      putRefusal
	}{
		{
			name:      "ok-empty-bag",
			container: makeBag(1, "bag", 10, 0, 1),
			child:     makeLeaf(2, "torch", 1, 0),
			all:       []repo.Item{makeBag(1, "bag", 10, 0, 1)},
			want:      putOK,
		},
		{
			name:      "too-heavy",
			container: makeBag(1, "bag", 5, 0, 1),
			child:     makeLeaf(2, "boulder", 10, 0),
			all:       []repo.Item{makeBag(1, "bag", 5, 0, 1)},
			want:      putTooHeavy,
		},
		{
			name: "weight-mult-does-not-enlarge-cap",
			// CapacityLbs is real-world cap; WeightMult only makes
			// the bag feel lighter to the carrier. A 5 lb cap still
			// rejects a 40 lb boulder even with mult 0.1.
			container: makeBag(1, "boh", 5, 0, 0.1),
			child:     makeLeaf(2, "boulder", 40, 0),
			all:       []repo.Item{makeBag(1, "boh", 5, 0, 0.1)},
			want:      putTooHeavy,
		},
		{
			name:      "weight-mult-encumbrance-only",
			container: makeBag(1, "boh", 100, 0, 0.1),
			child:     makeLeaf(2, "anvil", 50, 0),
			all:       []repo.Item{makeBag(1, "boh", 100, 0, 0.1)},
			want:      putOK,
		},
		{
			name:      "self-loop",
			container: makeBag(1, "bag", 10, 0, 1),
			child:     makeBag(1, "bag", 10, 0, 1),
			all:       []repo.Item{makeBag(1, "bag", 10, 0, 1)},
			want:      putSelf,
		},
		{
			name:      "not-a-container",
			container: makeLeaf(1, "rock", 1, 0),
			child:     makeLeaf(2, "feather", 0.1, 0),
			all:       []repo.Item{},
			want:      putNotAContainer,
		},
		{
			name: "liquid-only-rejects-solid",
			container: repo.Item{
				ID: 1, Name: "waterskin", Type: repo.ItemTypeContainer,
				Stats: &repo.ContainerStats{LiquidPints: 4},
			},
			child: makeLeaf(2, "torch", 1, 0),
			all:   []repo.Item{},
			want:  putLiquidContainer,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			byID := make(map[int64]repo.Item)
			for _, it := range tt.all {
				byID[it.ID] = it
			}
			byID[tt.container.ID] = tt.container
			byID[tt.child.ID] = tt.child
			got := canPut(tt.container, tt.child, tt.all, byID)
			if got != tt.want {
				t.Fatalf("canPut = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRawWeight_IgnoresMult(t *testing.T) {
	// rawWeight is the cap-side basis: real-world pounds, no mult.
	bag := makeBag(1, "boh", 100, 0, 0.1)
	anvil := makeLeaf(2, "anvil", 50, 1)
	idx := childrenOf([]repo.Item{anvil})
	if got := rawContentsWeight(bag, idx); got != 50 {
		t.Fatalf("rawContentsWeight = %v, want 50 (mult must NOT apply)", got)
	}
	// recursiveWeight (encumbrance basis) DOES apply mult.
	if got := recursiveWeight(bag, idx); got != 5 {
		t.Fatalf("recursiveWeight(boh+anvil) = %v, want 5 (1*0 + 0.1*50)", got)
	}
}

func TestCanPut_DepthRefusal(t *testing.T) {
	// outer at depth 0 (cap default 3), pack at 1, pouch at 2.
	// putting another bag (which itself has depth 0) into pouch
	// would land at depth 3 — at the cap, not over. Putting a
	// non-empty bag (depthBelow = 1) would push to depth 4.
	outer := makeBag(1, "chest", 0, 0, 1)
	pack := makeBag(2, "pack", 0, 0, 1)
	pack.ParentItemID = 1
	pouch := makeBag(3, "pouch", 0, 0, 1)
	pouch.ParentItemID = 2

	innerBag := makeBag(4, "innerbag", 0, 0, 1)
	leaf := makeLeaf(5, "ingot", 1, 4)

	all := []repo.Item{outer, pack, pouch, innerBag, leaf}
	byID := map[int64]repo.Item{1: outer, 2: pack, 3: pouch, 4: innerBag, 5: leaf}

	// Putting innerBag (with one leaf inside) into pouch:
	// depthOf(pouch)=2, +1 slot, +depthBelow(innerBag)=1 → 4 > 3 cap.
	if got := canPut(pouch, innerBag, all, byID); got != putTooDeep {
		t.Fatalf("expected putTooDeep, got %v", got)
	}
}
