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

	Encumbrance  creature.Load
	FatigueUntil time.Time
	Position     creature.Stance // standing/sitting/sleeping/fighting
	IdleSince    time.Time

	BoundRoomID   int64
	PlayedSeconds int64
	LastLogin     time.Time

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
}

// CharacterRepo is the persistence boundary character-select / character-
// create modes talk to.
type CharacterRepo interface {
	// Create inserts a new character. Returns ErrDuplicateCharacterName
	// when NameLower already exists.
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
}

var (
	ErrCharacterNotFound      = errors.New("repo: character not found")
	ErrDuplicateCharacterName = errors.New("repo: character name already taken")
)
