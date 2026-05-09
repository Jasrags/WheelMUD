// Package quest is the Phase F #31 quest engine: pure-data catalog,
// per-character state machine, and event subscribers.
//
// V1 ships three step kinds — `talk_to`, `kill_n`, `reach_room` —
// authored in YAML under `internal/quest/default/<id>.yaml` (or a
// `QUEST_DIR` override) and loaded once at boot via Load. The engine
// subscribes to `combat.CombatDeath` (kill_n), `world.PlayerEntered`
// (reach_room), and is driven for talk_to via a dialogue Effect
// (`accept_quest` / `advance_quest`) injected by the cmd-layer.
//
// `fetch` / `deliver` defer to a follow-up slice once we publish
// `world.ItemPickedUp` / `world.ItemGivenToMob`; `script` defers to
// the Lua work in #32. Both are intentionally absent from the
// StepKind allow-list so a YAML typo fails the boot.
package quest

// StepKind enumerates the V1 step vocabulary. Adding a new kind
// requires (a) extending the Validate switch in validator.go, (b)
// teaching the engine how to drive it (engine.go for event-driven
// kinds; effects.go for dialogue-driven kinds), and (c) updating
// the per-character StateJSON shape if the kind tracks counters.
type StepKind string

const (
	// StepTalkTo advances when the player executes an `advance_quest`
	// dialogue effect targeting this step's MobExternalID. One-shot;
	// no per-character counter state.
	StepTalkTo StepKind = "talk_to"

	// StepKillN advances each time the player kills a mob whose
	// template ExternalID matches MobExternalID. Per-character state
	// is `{"remaining": N}` decremented on every CombatDeath; the
	// step completes when remaining hits zero.
	StepKillN StepKind = "kill_n"

	// StepReachRoom advances when the player enters a room whose
	// ExternalID matches RoomExternalID. One-shot.
	StepReachRoom StepKind = "reach_room"
)

// Step is one entry in a Quest's ordered step list. Fields are
// kind-specific; Validate ensures the right ones are populated.
type Step struct {
	Kind   StepKind `yaml:"kind"`
	Prompt string   `yaml:"prompt"`

	// Mob is the target NPC's ExternalID. Used by StepTalkTo
	// (the NPC the player must speak with) and StepKillN (the
	// template ExternalID whose deaths count toward the quota).
	Mob string `yaml:"mob,omitempty"`

	// Room is the destination room's ExternalID for StepReachRoom.
	Room string `yaml:"room,omitempty"`

	// Count is the number of kills required for StepKillN. Must be
	// > 0; the validator rejects zero or negative counts.
	Count int `yaml:"count,omitempty"`
}

// Reward is the bundle granted on quest completion (final step's
// transition). XP and Copper are absolute integers; the engine adds
// XP via CharacterRepo.RecordXP and coin via RecordCoin.
//
// Item rewards are deferred — see followups memory. Coin grant is
// optimistic-lock-aware (mirrors the shop verbs); XP grant is not
// (no concurrency surface).
type Reward struct {
	XP     int64 `yaml:"xp,omitempty"`
	Copper int64 `yaml:"copper,omitempty"`
}

// Quest is the authored definition.
type Quest struct {
	ID        string `yaml:"id"`
	Name      string `yaml:"name"`
	Summary   string `yaml:"summary,omitempty"`
	GiverMob  string `yaml:"giver_mob,omitempty"` // optional: the NPC who hands it out (informational only)
	Steps     []Step `yaml:"steps"`
	Rewards   Reward `yaml:"rewards,omitempty"`
}

// Catalog is the immutable boot-time set of authored quests, keyed
// by Quest.ID. Lookups are done by string id; the engine never
// constructs a Catalog at runtime.
type Catalog struct {
	ByID map[string]*Quest
}

// Get returns the catalog entry for id (or nil + false on miss).
// Engine and dialogue effects use this to refuse unknown quest ids
// without crashing.
func (c *Catalog) Get(id string) (*Quest, bool) {
	if c == nil {
		return nil, false
	}
	q, ok := c.ByID[id]
	return q, ok
}
