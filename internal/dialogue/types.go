// Package dialogue carries the typed model for NPC dialogue trees
// (§15 / Phase F #30). The package is pure data — no DB, no session,
// no eventbus. Authoring lives on mob YAML; persistence is a JSON
// blob on `mob_templates.dialogue_json` (migration 0045); the cmd-
// layer `talk` verb decodes a tree on demand and pushes the runtime
// `Dialogue` mode against it.
//
// V1 vocabulary:
//   - a Tree owns Nodes keyed by NodeID and names a Root.
//   - each Node renders a Prompt and offers Responses.
//   - each Response matches free-text keywords (case-insensitive
//     substring) or a numbered choice, optionally fires Effects, and
//     names a Next NodeID (or empty = end-of-conversation).
//   - Effects are interpreted by the runtime (mode/dialogue.go); the
//     vocabulary here is a typed enum + free-form Args bag.
//
// The package owns Validate so YAML load and repo Get can both refuse
// malformed trees without pulling the runtime mode in as a dependency.
package dialogue

// NodeID identifies one node within a Tree. Authoring uses short
// snake_case strings (`root`, `ask_quest`, `farewell`); the validator
// rejects empty IDs and dangling Next references.
type NodeID string

// EffectKind enumerates the V1 dialogue effect vocabulary. Adding a
// new kind requires a runtime handler in internal/mode/dialogue.go
// (or a closure injected by internal/cmd/talk.go for cross-package
// effects like push_mode).
type EffectKind string

const (
	// EffectSetFlag flips a session-local boolean flag on. Args["name"]
	// names the flag; Show conditions can gate later responses on it.
	EffectSetFlag EffectKind = "set_flag"

	// EffectClearFlag flips a session-local flag off. Symmetric to
	// EffectSetFlag.
	EffectClearFlag EffectKind = "clear_flag"

	// EffectGoto overrides the Response's Next field after the Effects
	// run. Args["node"] names the destination NodeID. Useful when one
	// Response branches on a flag.
	EffectGoto EffectKind = "goto"

	// EffectPushMode hands off to another telnet mode and ends the
	// dialogue mode. Args["mode"] names the target ("shop", "banker",
	// future "quest"). The runtime handler is registered by the cmd-
	// layer so internal/dialogue stays free of cmd imports.
	EffectPushMode EffectKind = "push_mode"

	// EffectEnd pops the Dialogue mode immediately. Equivalent to a
	// Response with empty Next, but explicit.
	EffectEnd EffectKind = "end"

	// EffectAcceptQuest enrolls the character in a quest at step 0.
	// Args["quest_id"] names the quest catalog entry. The runtime
	// handler is closure-injected by the cmd-layer (no internal/quest
	// import here). No-op if the character already has the quest in
	// their log (active or completed) so re-clicking is safe.
	EffectAcceptQuest EffectKind = "accept_quest"

	// EffectAdvanceQuest advances the character's active step on a
	// quest IFF that step is StepTalkTo and the current NPC matches
	// the step's Mob ExternalID. Args["quest_id"] names the quest.
	// Same closure-injection pattern as EffectAcceptQuest. Logs +
	// no-ops on mismatch so authoring mistakes never lock players.
	EffectAdvanceQuest EffectKind = "advance_quest"

	// EffectScript runs a Lua catalog script. Args["script"] names
	// a `internal/scripts/default/<name>.lua` entry. The script
	// receives the V1 + V2 API surface (`say`, `emote`, `log`,
	// `ctx`, `quest.accept`, `quest.advance`); it does NOT have
	// access to a per-NPC speaker — dialogue scripts run on behalf
	// of the *acting character*, so any reply text should still come
	// from the dialogue tree's Reply field. The runtime closure is
	// injected by the cmd-layer (DialogueHooks.RunScript) so this
	// package stays free of internal/lua imports. Boot-time cross-
	// reference against the script catalog lives in the world
	// loader (loader.go) — package dialogue stays catalog-agnostic.
	// Phase F #32 slice 2.
	EffectScript EffectKind = "script"
)

// Effect is one atomic state mutation fired when a Response is taken.
// Args is a free-form string map; each EffectKind defines the keys
// it consumes (see EffectKind constants).
type Effect struct {
	Kind EffectKind        `json:"kind"`
	Args map[string]string `json:"args,omitempty"`
}

// Show is the V1 visibility expression for a Response. Empty means
// "always visible". RequireFlag (if non-empty) requires the named
// flag to be set; ForbidFlag (if non-empty) requires it unset. Both
// can be set on the same Show (AND).
//
// Defer richer expressions (OR, parens, comparisons) to followups —
// flag checks cover the V1 quest-state surface needed by #31.
type Show struct {
	RequireFlag string `json:"require_flag,omitempty"`
	ForbidFlag  string `json:"forbid_flag,omitempty"`
}

// Response is one branch under a Node.
//
//   - Match[] is a list of case-insensitive substring keywords the
//     player's free-text input is tested against. Empty = number-only.
//   - Reply, if non-empty, is rendered to the player after the choice
//     is taken and before Effects run.
//   - Next is the destination NodeID. Empty Next pops the Dialogue
//     mode (equivalent to EffectEnd).
//   - Effects fire in order before Next is followed. EffectGoto
//     overrides Next if present.
//   - Show gates whether this Response renders in the numbered list.
type Response struct {
	Match   []string `json:"match,omitempty"`
	Reply   string   `json:"reply,omitempty"`
	Label   string   `json:"label,omitempty"` // optional numbered-choice label; falls back to Match[0] if empty
	Next    NodeID   `json:"next,omitempty"`
	Effects []Effect `json:"effects,omitempty"`
	Show    Show     `json:"show,omitempty"`
}

// Node is one conversation turn from the NPC.
type Node struct {
	ID        NodeID     `json:"id"`
	Prompt    string     `json:"prompt"`
	Responses []Response `json:"responses,omitempty"`
}

// Tree is the full conversation graph for one NPC.
type Tree struct {
	Root  NodeID          `json:"root"`
	Nodes map[NodeID]Node `json:"nodes"`
}
