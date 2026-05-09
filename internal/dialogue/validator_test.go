package dialogue

import (
	"errors"
	"testing"
)

func goodTree() *Tree {
	return &Tree{
		Root: "root",
		Nodes: map[NodeID]Node{
			"root": {
				ID:     "root",
				Prompt: "Greetings.",
				Responses: []Response{
					{Match: []string{"hello", "hi"}, Reply: "Well met.", Next: "farewell"},
					{Match: []string{"quest"}, Effects: []Effect{{Kind: EffectSetFlag, Args: map[string]string{"name": "started"}}}, Next: "farewell"},
				},
			},
			"farewell": {
				ID:     "farewell",
				Prompt: "Travel safely.",
			},
		},
	}
}

func TestValidate_OK(t *testing.T) {
	if err := Validate(goodTree()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_RejectCases(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Tree)
	}{
		{"nil", func(_ *Tree) {}},
		{"empty root", func(t *Tree) { t.Root = "" }},
		{"no nodes", func(t *Tree) { t.Nodes = nil }},
		{"root missing", func(t *Tree) { t.Root = "missing" }},
		{"node id mismatch", func(t *Tree) {
			n := t.Nodes["root"]
			n.ID = "different"
			t.Nodes["root"] = n
		}},
		{"dangling next", func(t *Tree) {
			n := t.Nodes["root"]
			n.Responses[0].Next = "ghost"
			t.Nodes["root"] = n
		}},
		{"unknown effect", func(t *Tree) {
			n := t.Nodes["root"]
			n.Responses[0].Effects = []Effect{{Kind: "warp_reality"}}
			t.Nodes["root"] = n
		}},
		{"set_flag missing name", func(t *Tree) {
			n := t.Nodes["root"]
			n.Responses[1].Effects = []Effect{{Kind: EffectSetFlag}}
			t.Nodes["root"] = n
		}},
		{"goto missing node arg", func(t *Tree) {
			n := t.Nodes["root"]
			n.Responses[0].Effects = []Effect{{Kind: EffectGoto, Args: map[string]string{}}}
			t.Nodes["root"] = n
		}},
		{"goto dangling target", func(t *Tree) {
			n := t.Nodes["root"]
			n.Responses[0].Effects = []Effect{{Kind: EffectGoto, Args: map[string]string{"node": "ghost"}}}
			t.Nodes["root"] = n
		}},
		{"push_mode missing mode arg", func(t *Tree) {
			n := t.Nodes["root"]
			n.Responses[0].Effects = []Effect{{Kind: EffectPushMode}}
			t.Nodes["root"] = n
		}},
		{"show same flag in require and forbid", func(t *Tree) {
			n := t.Nodes["root"]
			n.Responses[0].Show = Show{RequireFlag: "x", ForbidFlag: "x"}
			t.Nodes["root"] = n
		}},
		{"show require_flag whitespace only", func(t *Tree) {
			n := t.Nodes["root"]
			n.Responses[0].Show = Show{RequireFlag: "   "}
			t.Nodes["root"] = n
		}},
		{"show forbid_flag whitespace only", func(t *Tree) {
			n := t.Nodes["root"]
			n.Responses[0].Show = Show{ForbidFlag: "\t"}
			t.Nodes["root"] = n
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var tree *Tree
			if tc.name != "nil" {
				tree = goodTree()
				tc.mut(tree)
			}
			err := Validate(tree)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, ErrInvalidTree) {
				t.Fatalf("expected ErrInvalidTree, got %v", err)
			}
		})
	}
}
