package repo

import (
	"context"
	"errors"
	"time"
)

// Character is a play-able persona owned by an Account. Names are
// globally unique (case-insensitive); login enforces single-active-
// character once multi-session policy lands.
type Character struct {
	ID            int64
	AccountID     int64
	Name          string
	NameLower     string
	CreatedAt     time.Time
	LastPlayedAt  *time.Time
	CurrentRoomID int64
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
}

var (
	ErrCharacterNotFound      = errors.New("repo: character not found")
	ErrDuplicateCharacterName = errors.New("repo: character name already taken")
)
