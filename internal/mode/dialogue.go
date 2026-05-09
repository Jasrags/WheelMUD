package mode

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/dialogue"
	"github.com/Jasrags/WheelMUD/telnet"
)

// Dialogue is the per-session NPC conversation mode (§15 / Phase F #30).
// It is pushed by the `talk <mob>` verb against an already-validated
// dialogue.Tree decoded from `mob_templates.dialogue_json`.
//
// Per-session state — current node and a flag bag — lives on the mode
// instance, so it drops on PopMode (logout, `bye`, end-of-conversation,
// or any Effect that pops). V1 has no persistence; future quest work
// (#31) will move durable flags to a character column. Concurrency is
// not a concern here: the dispatcher invokes Handle / Prompt
// synchronously on the read goroutine.
//
// DialogueHooks bundles the cross-package effect closures the cmd-
// layer injects so internal/mode stays free of internal/cmd and
// internal/quest imports. All hook fields are optional; nil means
// "log a warning and treat the effect as a no-op."
//
// Each hook receives the dispatcher's ctx — the per-session
// context that is canceled when the read loop exits (EOF, idle,
// flood) or when shutdown drains. Hooks that do blocking I/O
// (DB reads, repo writes) MUST honor it so a stalled hook can't
// keep a torn-down session alive past drain.
//
//   - PushMode handles `effects: kind: push_mode`. nil today (no V1
//     push_mode targets).
//   - AcceptQuest handles `effects: kind: accept_quest`. Wired by
//     main.go to internal/quest.Engine.AcceptQuest.
//   - AdvanceQuest handles `effects: kind: advance_quest`. Wired by
//     main.go to internal/quest.Engine.AdvanceTalkTo. The mode
//     forwards both the questID and the conversation's NPC
//     ExternalID so the engine can verify the active step matches.
type DialogueHooks struct {
	PushMode     func(ctx context.Context, s *telnet.Session, modeName string, args map[string]string) error
	AcceptQuest  func(ctx context.Context, s *telnet.Session, questID string) error
	AdvanceQuest func(ctx context.Context, s *telnet.Session, questID, npcExternalID string) error

	// RunScript fires a `effects: kind: script` Lua catalog
	// invocation on behalf of the acting character. Phase F #32
	// slice 2. Wired by main.go to a closure around lua.Runner.Run
	// that supplies the V2 quest API + push_mode hooks. nil here
	// means "no Lua runner wired" — applyEffects logs a warning
	// and continues so a misconfigured boot doesn't lock the
	// player into a stuck dialogue. Errors returned by RunScript
	// are non-fatal: we log + continue (mirrors AcceptQuest /
	// AdvanceQuest's repo-error policy).
	RunScript func(ctx context.Context, s *telnet.Session, scriptName string) error
}

type Dialogue struct {
	npcName        string
	npcExternalID  string // mob_template ExternalID — used by AdvanceQuest hook
	tree           *dialogue.Tree
	currentID      dialogue.NodeID
	flags          map[string]bool
	visible        []dialogue.Response // re-computed each render; numbered choice index → entry
	hooks          DialogueHooks
}

// NewDialogue constructs a Dialogue mode rooted at tree.Root. The
// caller is responsible for validating the tree first (the world
// loader and the cmd-layer both call dialogue.Validate); we re-check
// here as defense-in-depth so a hand-edited DB row can't crash the
// mode push.
//
// npcExternalID is the mob_template's ExternalID (e.g. "tr.elder")
// so AdvanceQuest can verify the active step's Mob matches this
// conversation. Empty string is fine — quest effects just won't fire.
func NewDialogue(npcName, npcExternalID string, tree *dialogue.Tree, hooks DialogueHooks) (*Dialogue, error) {
	if err := dialogue.Validate(tree); err != nil {
		return nil, err
	}
	return &Dialogue{
		npcName:       npcName,
		npcExternalID: npcExternalID,
		tree:          tree,
		currentID:     tree.Root,
		flags:         make(map[string]bool),
		hooks:         hooks,
	}, nil
}

// Prompt renders the current node prompt + the numbered list of
// visible responses. Pure: it inspects state but does not mutate.
// The visible-list cache that Handle.pick reads is refreshed at the
// top of Handle, so a WriteAsync redraw between dispatches can call
// Prompt freely without affecting the next pick semantics.
func (m *Dialogue) Prompt(_ context.Context, _ *telnet.Session) string {
	node, ok := m.tree.Nodes[m.currentID]
	if !ok {
		return promptFallback
	}
	visible := m.visibleResponses(node)
	var b strings.Builder
	b.WriteString("\r\n")
	b.WriteString(node.Prompt)
	b.WriteString("\r\n")
	for i, r := range visible {
		fmt.Fprintf(&b, "  %d) %s\r\n", i+1, responseLabel(r))
	}
	if len(visible) == 0 {
		b.WriteString("  (no replies; type 'bye' to leave)\r\n")
	}
	b.WriteString("> ")
	return b.String()
}

// OnEnter writes a one-line banner so the player knows they've left
// the game prompt. The numbered options come out via Prompt afterwards.
func (m *Dialogue) OnEnter(s *telnet.Session) error {
	return s.WriteRaw([]byte(fmt.Sprintf("\r\nYou begin speaking with %s.\r\n", m.npcName)))
}

// OnExit emits a closing line so the transition back to game mode is
// visible. The dispatcher's prompt cache is cleared by PopMode for us.
func (m *Dialogue) OnExit(s *telnet.Session) error {
	return s.WriteRaw([]byte(fmt.Sprintf("You stop speaking with %s.\r\n", m.npcName)))
}

// Handle dispatches one line of player input against the current node.
//
// Input precedence:
//   - empty / "bye" / "quit" / "leave" — pop the mode.
//   - bare integer in [1, len(visible)] — pick that numbered response.
//   - free text — match against Response.Match[] (case-insensitive
//     substring on either side).
//   - unmatched — Prompt re-renders, no state change.
//
// Effects fire AFTER the Reply is written and BEFORE Next is followed
// so a `set_flag` mutation is visible the moment the next node's
// Prompt computes its visible list.
func (m *Dialogue) Handle(ctx context.Context, s *telnet.Session, line string) error {
	trimmed := strings.TrimSpace(line)
	switch strings.ToLower(trimmed) {
	case "", "bye", "quit", "leave":
		return s.PopMode()
	}

	// Refresh the visible-list cache against the current node + flag
	// bag before resolving the player's input. Prompt is a pure
	// renderer; the cache lives here so pick can resolve numbered
	// choices without recomputing inside the loop.
	if node, ok := m.tree.Nodes[m.currentID]; ok {
		m.visible = m.visibleResponses(node)
	} else {
		m.visible = nil
	}

	picked, ok := m.pick(trimmed)
	if !ok {
		// Unmatched input — let Prompt re-render the same node. Emit a
		// tiny hint so the player isn't confused about why nothing
		// changed; keeps it terse to match other modes.
		return s.WriteRaw([]byte("(That isn't one of your replies. Type a number, a keyword, or 'bye'.)\r\n"))
	}

	if picked.Reply != "" {
		if err := s.WriteRaw([]byte(picked.Reply + "\r\n")); err != nil {
			return err
		}
	}

	next := picked.Next
	popped := false
	for _, eff := range picked.Effects {
		if popped {
			break
		}
		switch eff.Kind {
		case dialogue.EffectSetFlag:
			if name := eff.Args["name"]; name != "" {
				m.flags[name] = true
			}
		case dialogue.EffectClearFlag:
			if name := eff.Args["name"]; name != "" {
				delete(m.flags, name)
			}
		case dialogue.EffectGoto:
			if dest := dialogue.NodeID(eff.Args["node"]); dest != "" {
				if _, ok := m.tree.Nodes[dest]; ok {
					next = dest
				}
			}
		case dialogue.EffectPushMode:
			if m.hooks.PushMode == nil {
				slog.Warn("dialogue push_mode unbound", "mode", eff.Args["mode"], "npc", m.npcName)
				continue
			}
			if err := m.hooks.PushMode(ctx, s, eff.Args["mode"], eff.Args); err != nil {
				return fmt.Errorf("dialogue push_mode %q: %w", eff.Args["mode"], err)
			}
			popped = true
		case dialogue.EffectAcceptQuest:
			if m.hooks.AcceptQuest == nil {
				slog.Warn("dialogue accept_quest unbound", "quest", eff.Args["quest_id"], "npc", m.npcName)
				continue
			}
			if err := m.hooks.AcceptQuest(ctx, s, eff.Args["quest_id"]); err != nil {
				slog.Warn("dialogue accept_quest failed",
					"quest", eff.Args["quest_id"], "npc", m.npcName, "error", err)
				// Don't surface to player — accept is idempotent and a
				// repo error shouldn't fail the response. Continue the
				// effect chain.
			}
		case dialogue.EffectAdvanceQuest:
			if m.hooks.AdvanceQuest == nil {
				slog.Warn("dialogue advance_quest unbound", "quest", eff.Args["quest_id"], "npc", m.npcName)
				continue
			}
			if err := m.hooks.AdvanceQuest(ctx, s, eff.Args["quest_id"], m.npcExternalID); err != nil {
				slog.Warn("dialogue advance_quest failed",
					"quest", eff.Args["quest_id"], "npc", m.npcName, "error", err)
			}
		case dialogue.EffectScript:
			if m.hooks.RunScript == nil {
				slog.Warn("dialogue script unbound", "script", eff.Args["script"], "npc", m.npcName)
				continue
			}
			if err := m.hooks.RunScript(ctx, s, eff.Args["script"]); err != nil {
				slog.Warn("dialogue script failed",
					"script", eff.Args["script"], "npc", m.npcName, "error", err)
			}
		case dialogue.EffectEnd:
			popped = true
		}
	}

	if popped {
		// Either push_mode replaced/pushed a sibling, or end fired.
		// In the end case we still need to pop ourselves.
		if _, ok := s.CurrentMode().(*Dialogue); ok {
			return s.PopMode()
		}
		return nil
	}

	if next == "" {
		// Empty Next = end-of-conversation, same as EffectEnd.
		return s.PopMode()
	}
	if _, ok := m.tree.Nodes[next]; !ok {
		// Validator caught dangling targets; this is defense in depth.
		slog.Warn("dialogue node missing at runtime", "node", next, "npc", m.npcName)
		return s.PopMode()
	}
	m.currentID = next
	return nil
}

// pick resolves the player's input against the current node's visible
// responses. Numbered choice wins outright; otherwise we substring-
// match each Response.Match[] entry case-insensitively on either side
// (so "tell me about quests" matches Match=["quest"]).
func (m *Dialogue) pick(input string) (dialogue.Response, bool) {
	if n, err := strconv.Atoi(input); err == nil {
		if n >= 1 && n <= len(m.visible) {
			return m.visible[n-1], true
		}
		return dialogue.Response{}, false
	}
	lower := strings.ToLower(input)
	for _, r := range m.visible {
		for _, kw := range r.Match {
			if kw == "" {
				continue
			}
			// Match the player input against the authored keyword as a
			// case-insensitive substring. The reverse direction (keyword
			// substring of input) is intentionally NOT checked: a single-
			// character input like "a" or "i" would otherwise match every
			// keyword that contains those letters.
			if strings.Contains(lower, strings.ToLower(kw)) {
				return r, true
			}
		}
	}
	return dialogue.Response{}, false
}

// visibleResponses filters out Show-gated responses against the
// session's flag bag. The slice ordering matches the YAML authoring
// order — numbered choices stay stable as long as visibility doesn't
// change.
func (m *Dialogue) visibleResponses(n dialogue.Node) []dialogue.Response {
	out := make([]dialogue.Response, 0, len(n.Responses))
	for _, r := range n.Responses {
		if r.Show.RequireFlag != "" && !m.flags[r.Show.RequireFlag] {
			continue
		}
		if r.Show.ForbidFlag != "" && m.flags[r.Show.ForbidFlag] {
			continue
		}
		out = append(out, r)
	}
	return out
}

// responseLabel produces the user-visible label for a numbered choice.
// Explicit Label wins; otherwise the first Match keyword; otherwise
// the first sentence of Reply (trimmed). Falls back to "(continue)"
// so an authoring oversight doesn't render a blank line.
func responseLabel(r dialogue.Response) string {
	if r.Label != "" {
		return r.Label
	}
	if len(r.Match) > 0 && r.Match[0] != "" {
		return r.Match[0]
	}
	if r.Reply != "" {
		first, _, _ := strings.Cut(r.Reply, ".")
		if first != "" {
			return first
		}
	}
	return "(continue)"
}
