package repo

import (
	"context"
	"errors"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
)

// Character is a play-able persona owned by an Account. Names are
// globally unique (case-insensitive); login enforces single-active-
// character once multi-session policy lands.
//
// The Core stat block (abilities, HP, defense, saves, speed,
// conditions, position flags, DR/resists) is shared with mobs via
// creature.Core. Player-only fields (race, class levels, xp,
// reputation, wealth, idle/fatigue timers, bound room) live as
// peers next to Core.
//
// Note: CurrentRoomID stays top-level rather than folding into
// Core.CurrentRoomID. The character's room column predates Core
// and is consumed by mode/postauth + Session bootstrap; keeping the
// repo bind for it stable avoids touching every caller.
type Character struct {
	ID            int64
	AccountID     int64
	Name          string
	NameLower     string
	CreatedAt     time.Time
	LastPlayedAt  *time.Time
	CurrentRoomID int64

	// Shared stat block. Core.ID / Core.CurrentRoomID are not used
	// for characters — the top-level fields are the source of truth.
	Core creature.Core

	// Player-only fields (migration 0009).
	Race        creature.Race
	Background  creature.Background
	ClassLevels map[creature.Class]int8

	XP             int64
	PracticePoints int16

	HeightCm   int16
	WeightKg   int16
	Age        int16
	Handedness creature.Hand

	Fame        int32
	Infamy      int32
	InfamyShare float32

	Coin        currency.Amount
	BankBalance currency.Amount

	// CoinVersion is the optimistic-concurrency token for
	// (Coin, BankBalance). Every successful RecordCoin bumps it by 1.
	// Coin-mutating verbs read this on the snapshot they computed
	// against and pass it as the expected version to RecordCoin; the
	// repo refuses the UPDATE when the row's version has moved on
	// (ErrCoinConflict). Same shape as ErrItemMoved on items.
	CoinVersion int64

	Encumbrance  creature.Load
	FatigueUntil time.Time
	Position     creature.Stance // standing/sitting/sleeping/fighting
	IdleSince    time.Time

	BoundRoomID   int64
	PlayedSeconds int64
	LastLogin     time.Time

	// PromptTemplate overrides the server default prompt template
	// (cmd/server/main.go::defaultPromptTemplate). Empty means "use
	// the server default". The template grammar is internal/prompt's
	// (`%h/%H/%r/%g/%%/...`) and may include cfmt color tags
	// (`{{...}}::red`) which Game.Prompt renders before write.
	PromptTemplate string

	// LastNewsSeen is the date of the most recent §18 news entry
	// the character has read. Persisted as unix seconds; the zero
	// value means "never seen", so every seeded entry shows as
	// unread on first game-entry. The `news <id>` command bumps
	// this via CharacterRepo.MarkNewsSeen, which clamps so a stale
	// entry can never lower the watermark.
	LastNewsSeen time.Time

	// JSON-encoded catalogs and bag-of-things. Plumbed end-to-end
	// so the round-trip is verified, but typed and consumed by
	// later roadmap items (§12 feats/skills, §14 equipment/
	// inventory, §15 quests/dialogue).
	Feats         []int32
	Skills        map[int32]creature.SkillRanks
	ClassFeatures []int32
	QuestLog      []creature.QuestProgress
	DialogueState map[int64]creature.DialogueCursor
	Equipment     creature.Equipment
	Inventory     []int64

	// ChannelSettings holds per-channel mute state keyed by channel
	// name (lowercase). `true` means the player has the channel
	// turned off and won't receive broadcasts; absent / `false`
	// means they're listening on the default. Kept sparse so the
	// JSON column stays small for the common (all-defaults) case.
	ChannelSettings map[string]bool

	// Channeling is the optional channeler sub-record. nil for
	// non-channeler classes (Armsman, Woodsman, etc.); non-nil for
	// Initiate / Wilder. Populated by chargen at commit time and
	// persisted via channeling_json (migration 0033). The pointer
	// is reconstructed on every load — it is the source of truth
	// for the character's One Power affiliation.
	Channeling *creature.Channeling

	// PvP is the player's opt-in for character-vs-character combat.
	// Default false — `attack <player>` is refused unless both the
	// attacker and the defender have toggled this on (via the `pvp`
	// verb). Persisted as INTEGER 0/1 (migration 0037). Room-side
	// `nopvp` and the newbie level cap are independent gates checked
	// at the verb layer; this flag is only the consent half.
	PvP bool

	// Pending* are level-up gain pools deposited by `train` and
	// decremented by future spend verbs (`learn`, `pick`, `bump`).
	// All four default 0 (migration 0039) and accumulate across
	// level-ups; the V1 cadence is encoded in
	// progression.ComputeLevelUp:
	//   PendingFeats         — +1 at every level divisible by 3
	//   PendingSkillPoints   — +max(1, class.SkillPoints + IntMod) per level
	//   PendingAbilityBumps  — +1 at L4/8/12/16/20
	//   PendingWeaves        — +1 per channeler-class level (else 0)
	PendingFeats        int32
	PendingSkillPoints  int32
	PendingAbilityBumps int32
	PendingWeaves       int32

	// AuthLevel mirrors telnet.AuthLevel as a plain uint8 to avoid
	// coupling the repo package to telnet. Use the AuthLevel*
	// constants below. postauth.promoteToGame copies this onto
	// session.AuthLevel when the character is selected, so a single
	// account can own admin and player characters side-by-side.
	AuthLevel uint8
}

// AuthLevel constants mirror telnet.AuthLevel. Defined on the repo
// package so SQL plumbing and bootstrap logic can reference the enum
// without importing telnet (which would invite a cycle once telnet
// grows repo-aware helpers).
const (
	AuthLevelGuest  uint8 = 0
	AuthLevelPlayer uint8 = 1
	AuthLevelAdmin  uint8 = 2
	AuthLevelMax          = AuthLevelAdmin
)

// CharacterRepo is the persistence boundary character-select / character-
// create modes talk to.
type CharacterRepo interface {
	// Create inserts a new character. Returns ErrDuplicateCharacterName
	// when NameLower already exists.
	//
	// Bootstrap: if the characters table is empty when Create runs,
	// the new row is forced to AuthLevelAdmin atomically (the count
	// and insert run in a single transaction) so a fresh deploy has
	// a working operator without manual SQL. The caller's AuthLevel
	// is overridden in that single case; subsequent calls honor it.
	Create(ctx context.Context, c Character) (Character, error)
	// FindByName resolves a character by case-insensitive name.
	// Returns ErrCharacterNotFound when missing.
	FindByName(ctx context.Context, name string) (Character, error)
	// GetByID resolves a character by primary key. Returns
	// ErrCharacterNotFound when no row matches. Used by combat to
	// resolve participant Cores without going through name lookups.
	GetByID(ctx context.Context, id int64) (Character, error)
	// ListByAccount returns the account's characters, ordered by
	// last_played_at descending (nulls last) then by name.
	ListByAccount(ctx context.Context, accountID int64) ([]Character, error)
	// RecordPlay updates last_played_at for a character.
	RecordPlay(ctx context.Context, id int64, when time.Time) error
	// RecordRoom persists the character's current location. Movement
	// commands call this on every successful move so a reconnect drops
	// the character back where they were.
	RecordRoom(ctx context.Context, id, roomID int64) error
	// RecordCore persists the live mutable stat-block fields (HP,
	// subdual, conditions, position-flags, affects). Combat / regen
	// / weave-resolution paths call this; immutable fields like
	// abilities and class are untouched.
	RecordCore(ctx context.Context, id int64, hpCurrent, subdual int32, conditions creature.Condition, positionFlags creature.PositionFlags) error
	// RecordChannelSettings persists the per-channel mute map after
	// a toggle. The channel command writes through immediately so
	// the setting survives logout even if autosave hasn't fired.
	RecordChannelSettings(ctx context.Context, id int64, settings map[string]bool) error
	// RecordInventory persists the ordered item-id list after a
	// pickup / drop / give. The inventory_json column is the display
	// ordering; ownership truth lives on items.owner_character_id.
	RecordInventory(ctx context.Context, id int64, ids []int64) error
	// RecordEquipment persists the worn/wielded slot map after a
	// wear/wield/remove (or an auto-clear from drop/give/put). The
	// equipment_json column is a pointer overlay over inventory:
	// equipped items keep owner_character_id set, so location truth
	// still lives on items, not on this map. Returns
	// ErrCharacterNotFound when no row matches id.
	RecordEquipment(ctx context.Context, id int64, eq creature.Equipment) error
	// RecordCoin persists carried + bank wealth after a transfer or
	// shop transaction. expectedVersion must match the row's current
	// coin_version (typically the CoinVersion off the Character
	// snapshot the caller computed against). The repo bumps the
	// version on success and returns ErrCoinConflict on mismatch —
	// the caller should refuse the verb with "your balance changed".
	// ErrCharacterNotFound is returned when the row is missing.
	RecordCoin(ctx context.Context, id int64, coin, bank currency.Amount, expectedVersion int64) error
	// RecordXP persists the character's total XP after an award.
	// Combat / quest reward paths call this; the value passed is the
	// new absolute total, not a delta. Returns ErrCharacterNotFound
	// when no row matches id.
	RecordXP(ctx context.Context, id int64, xp int64) error
	// RecordPromptTemplate persists the per-character prompt override.
	// Empty tmpl means "fall back to the server default". Returns
	// ErrCharacterNotFound when no row matches id.
	RecordPromptTemplate(ctx context.Context, id int64, tmpl string) error
	// RecordPvP toggles the character's PvP opt-in flag. Persists to
	// characters.pvp (migration 0037). Returns ErrCharacterNotFound
	// when no row matches id.
	RecordPvP(ctx context.Context, id int64, on bool) error
	// RecordLevelUp atomically writes the new ClassLevels map plus
	// the recomputed Core fields (HP/MaxHP/BAB/Saves) and increments
	// the four pending-pool counters by the per-level deltas. Phase E
	// #23 slices 3 (commit) + 4 (pending pools). The caller computes
	// the new totals via progression.ComputeLevelUp and copies the
	// shape into LevelUpFields; the repo persists everything in one
	// UPDATE. Returns ErrCharacterNotFound when no row matches id.
	RecordLevelUp(ctx context.Context, id int64, f LevelUpFields) error
	// RecordSkillRank atomically writes a single skill's rank entry
	// and stamps the new pending_skill_points balance. Phase E #24
	// (`learn` verb). The caller computes the new absolute pending
	// value (mirrors RecordCoin / RecordXP — no read-modify-write
	// race). The skills_json column is fully rewritten with the
	// upserted entry; new keys add, existing keys overwrite ranks +
	// IsClassSkill. newRanks must be ≥ 0; the verb layer enforces
	// per-skill caps (level+3) and budget refusals before this call.
	// Returns ErrCharacterNotFound when no row matches id.
	RecordSkillRank(ctx context.Context, id int64,
		skillID int32, newRanks int8, isClassSkill bool,
		newPendingSkillPoints int32) error
	// RecordFeatPick appends a single feat hash to feats_json and
	// stamps the new pending_feats balance. Phase E #25 (`pick feat`).
	// Caller computes the absolute new pending value. The feats_json
	// column is rewritten with the appended entry. Verb layer
	// enforces duplicate-pick + empty-pool refusals before this call.
	// Returns ErrCharacterNotFound when no row matches id.
	RecordFeatPick(ctx context.Context, id int64,
		featID int32, newPendingFeats int32) error
	// RecordAbilityBump increments one ability's Current score by 1
	// and stamps the new pending_ability_bumps balance. Phase E #25
	// (`bump <abil>`). The ability column written is selected by the
	// AbilityKey enum. Caller passes the absolute new score so the
	// repo doesn't need to read-modify-write. Verb layer enforces
	// the cap-at-20 + empty-pool refusals before this call.
	// Returns ErrCharacterNotFound when no row matches id.
	RecordAbilityBump(ctx context.Context, id int64,
		ability AbilityKey, newScore int8,
		newPendingAbilityBumps int32) error
	// RecordWeavePick appends a weave id to channeling_json's
	// WeavesKnownIDs slice and stamps the new pending_weaves balance.
	// Phase E #25 (`learn weave`). Caller computes the absolute new
	// pending value. The channeling_json column is rewritten with
	// the appended entry; the row's existing Channeling sub-record
	// must be non-nil (verb layer refuses non-channelers). Verb
	// layer enforces affinity + duplicate-pick + empty-pool refusals
	// before this call. Returns ErrCharacterNotFound when no row
	// matches id; ErrNotChanneler when the row's channeling_json is
	// nil (defense-in-depth — should be unreachable from verb).
	RecordWeavePick(ctx context.Context, id int64,
		weaveID string, newPendingWeaves int32) error
	// MarkNewsSeen advances last_news_seen to `when` if it strictly
	// advances the watermark; older or equal values are silently
	// ignored so reading an old entry can't unread newer ones.
	// Returns ErrCharacterNotFound when no row matches id.
	MarkNewsSeen(ctx context.Context, id int64, when time.Time) error
	// Delete removes the character row identified by id. Caller is
	// responsible for cleaning up soft-FK references first
	// (items.owner_character_id in particular — items belonging to
	// the deleted character must be deleted or relocated before this
	// call so the location invariant on items.owner_character_id is
	// not left dangling). Columns that live on the character row
	// (equipment_json, inventory_json, coin, bank_balance,
	// channeling_json, last_news_seen, …) are removed with the row.
	// Returns ErrCharacterNotFound when no row matches id.
	Delete(ctx context.Context, id int64) error
}

// LevelUpFields is the persistence shape for a single class-level
// commit. Mirrors progression.LevelGains without creating a package
// cycle (progression imports repo). The cmd-layer caller copies the
// fields across at the call site.
//
// HPCurrent/HPMax/BAB/Saves/ClassLevels are absolute new values
// (overwrite the row). Pending*Delta are increments applied with
// `pending_x = pending_x + delta` so deposits accumulate across
// level-ups; pass 0 for any pool that doesn't grow this level.
type LevelUpFields struct {
	ClassLevels              map[creature.Class]int8
	HPCurrent                int32
	HPMax                    int32
	BAB                      int16
	Saves                    creature.Saves
	PendingFeatsDelta        int32
	PendingSkillPointsDelta  int32
	PendingAbilityBumpsDelta int32
	PendingWeavesDelta       int32
}

var (
	ErrCharacterNotFound      = errors.New("repo: character not found")
	ErrDuplicateCharacterName = errors.New("repo: character name already taken")

	// ErrCoinConflict is returned by RecordCoin when the row's
	// coin_version no longer matches the caller's expected version.
	// This means another path (another session, a parallel verb, a
	// background tick) bumped the row between the caller's read and
	// write. Verbs surface this as "your balance changed, try again"
	// and let the player re-issue the command.
	ErrCoinConflict = errors.New("repo: coin version conflict")

	// ErrNotChanneler is returned by RecordWeavePick when the row's
	// channeling_json is nil. Defense-in-depth — the verb layer
	// should already have refused non-channelers before the repo
	// call.
	ErrNotChanneler = errors.New("repo: character is not a channeler")
)

// AbilityKey selects one of the six ability scores for
// RecordAbilityBump. Values are stable (do not reorder) — the SQLite
// implementation switches on these to pick the column to update.
type AbilityKey uint8

const (
	AbilityStr AbilityKey = iota
	AbilityDex
	AbilityCon
	AbilityInt
	AbilityWis
	AbilityCha
)

// String returns the lower-case three-letter ability token, useful for
// audit args and log output.
func (a AbilityKey) String() string {
	switch a {
	case AbilityStr:
		return "str"
	case AbilityDex:
		return "dex"
	case AbilityCon:
		return "con"
	case AbilityInt:
		return "int"
	case AbilityWis:
		return "wis"
	case AbilityCha:
		return "cha"
	}
	return "?"
}
