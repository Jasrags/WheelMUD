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
	// RecordPromptTemplate persists the per-character prompt override.
	// Empty tmpl means "fall back to the server default". Returns
	// ErrCharacterNotFound when no row matches id.
	RecordPromptTemplate(ctx context.Context, id int64, tmpl string) error
	// MarkNewsSeen advances last_news_seen to `when` if it strictly
	// advances the watermark; older or equal values are silently
	// ignored so reading an old entry can't unread newer ones.
	// Returns ErrCharacterNotFound when no row matches id.
	MarkNewsSeen(ctx context.Context, id int64, when time.Time) error
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
)
