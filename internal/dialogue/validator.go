package dialogue

import (
	"errors"
	"fmt"
	"strings"
)

// hasInvalidFlag reports whether s is non-empty but degenerate
// (whitespace-only). Empty string is the legitimate "absent" sentinel
// for Show fields, so it returns false. Any non-empty string that
// trims to "" is rejected — that's almost always an authoring typo
// like `require_flag: " "` and silently passing it would gate the
// response on a flag the runtime can't toggle.
func hasInvalidFlag(s string) bool {
	return s != "" && strings.TrimSpace(s) == ""
}

// ErrInvalidTree is the canonical typed error returned by Validate.
// Callers wrap with their own context (zone path, mob id) before
// surfacing.
var ErrInvalidTree = errors.New("invalid dialogue tree")

// Validate enforces V1 invariants on a parsed Tree:
//
//   - Tree is non-nil and has a non-empty Root.
//   - Nodes map is non-empty.
//   - The Root NodeID exists in Nodes.
//   - Every Node ID matches its map key and is non-empty.
//   - Every Response.Next, if non-empty, points to a known node.
//   - Every Effect.Kind is a known kind, and the kind's required Args
//     are present.
//   - Every Show.RequireFlag / ForbidFlag is non-empty when set
//     (defense against accidental empty-string flag matches).
//
// Validation runs at YAML load time (fail-loud at boot) and again at
// the cmd-layer when a tree is decoded — defense in depth, since a
// hand-edited row could otherwise crash the talk verb.
func Validate(t *Tree) error {
	if t == nil {
		return fmt.Errorf("%w: tree is nil", ErrInvalidTree)
	}
	if t.Root == "" {
		return fmt.Errorf("%w: empty root", ErrInvalidTree)
	}
	if len(t.Nodes) == 0 {
		return fmt.Errorf("%w: no nodes", ErrInvalidTree)
	}
	if _, ok := t.Nodes[t.Root]; !ok {
		return fmt.Errorf("%w: root node %q not in nodes", ErrInvalidTree, t.Root)
	}
	for key, node := range t.Nodes {
		if key == "" {
			return fmt.Errorf("%w: empty node key", ErrInvalidTree)
		}
		if node.ID == "" {
			return fmt.Errorf("%w: node %q has empty ID", ErrInvalidTree, key)
		}
		if node.ID != key {
			return fmt.Errorf("%w: node ID %q does not match map key %q", ErrInvalidTree, node.ID, key)
		}
		for i, resp := range node.Responses {
			if resp.Next != "" {
				if _, ok := t.Nodes[resp.Next]; !ok {
					return fmt.Errorf("%w: node %q response[%d] dangling next %q", ErrInvalidTree, key, i, resp.Next)
				}
			}
			for j, eff := range resp.Effects {
				if err := validateEffect(eff, t); err != nil {
					return fmt.Errorf("%w: node %q response[%d] effect[%d]: %s", ErrInvalidTree, key, i, j, err.Error())
				}
			}
			// Show fields use empty string as the "absent" sentinel.
			// We can't distinguish a YAML-explicit `require_flag: ""`
			// from an absent block at this layer; both look like the
			// zero value. The Decoder wraps RequireFlag/ForbidFlag with
			// strings.TrimSpace at YAML load time so a whitespace-only
			// flag also lands as "" here. Any path that sets the field
			// MUST set a real name — the explicit checks below catch a
			// hand-edited DB row carrying e.g. {require_flag:" "}.
			if hasInvalidFlag(resp.Show.RequireFlag) {
				return fmt.Errorf("%w: node %q response[%d] show.require_flag is whitespace-only", ErrInvalidTree, key, i)
			}
			if hasInvalidFlag(resp.Show.ForbidFlag) {
				return fmt.Errorf("%w: node %q response[%d] show.forbid_flag is whitespace-only", ErrInvalidTree, key, i)
			}
			if resp.Show.RequireFlag != "" && resp.Show.ForbidFlag != "" && resp.Show.RequireFlag == resp.Show.ForbidFlag {
				return fmt.Errorf("%w: node %q response[%d] show: same flag %q in require and forbid", ErrInvalidTree, key, i, resp.Show.RequireFlag)
			}
		}
	}
	return nil
}

func validateEffect(e Effect, t *Tree) error {
	switch e.Kind {
	case EffectSetFlag, EffectClearFlag:
		if e.Args["name"] == "" {
			return fmt.Errorf("%s requires args.name", e.Kind)
		}
	case EffectGoto:
		dest := NodeID(e.Args["node"])
		if dest == "" {
			return fmt.Errorf("goto requires args.node")
		}
		if _, ok := t.Nodes[dest]; !ok {
			return fmt.Errorf("goto target %q not in nodes", dest)
		}
	case EffectPushMode:
		mode := e.Args["mode"]
		if mode == "" {
			return fmt.Errorf("push_mode requires args.mode")
		}
	case EffectEnd:
		// no args required
	case EffectAcceptQuest, EffectAdvanceQuest:
		if e.Args["quest_id"] == "" {
			return fmt.Errorf("%s requires args.quest_id", e.Kind)
		}
	default:
		return fmt.Errorf("unknown effect kind %q", e.Kind)
	}
	return nil
}
