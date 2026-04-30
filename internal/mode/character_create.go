package mode

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

const (
	minCharacterNameLen = 3
	maxCharacterNameLen = 24
)

// reservedCharacterNames are forbidden case-insensitively because they
// collide with character-select keywords; an unrouteable character name
// is a permanent footgun (mirrors the "new" reservation in account
// create).
var reservedCharacterNames = map[string]bool{
	"create": true,
	"quit":   true,
}

// CharacterCreate prompts for a new character name and inserts it for
// the authenticated account. On success it auto-promotes the session
// into the game mode with the new character.
type CharacterCreate struct {
	repo  repo.CharacterRepo
	game  telnet.Mode
	shown bool
}

func NewCharacterCreate(characters repo.CharacterRepo, game telnet.Mode) *CharacterCreate {
	return &CharacterCreate{repo: characters, game: game}
}

func (m *CharacterCreate) Prompt(_ *telnet.Session) string {
	return "Choose a character name: "
}

func (m *CharacterCreate) OnEnter(s *telnet.Session) error {
	if m.shown {
		return nil
	}
	m.shown = true
	return s.WriteRaw([]byte("\r\nCreate a character.\r\n"))
}

func (m *CharacterCreate) OnExit(_ *telnet.Session) error { return nil }

func (m *CharacterCreate) Handle(ctx context.Context, s *telnet.Session, line string) error {
	name := strings.TrimSpace(line)
	if err := validateCharacterName(name); err != nil {
		return s.WriteRaw([]byte(err.Error() + "\r\n"))
	}

	c, err := m.repo.Create(ctx, repo.Character{
		AccountID: s.AccountID,
		Name:      name,
	})
	switch {
	case errors.Is(err, repo.ErrDuplicateCharacterName):
		return s.WriteRaw([]byte("Character name already taken. Choose another.\r\n"))
	case err != nil:
		return s.WriteRaw([]byte("Character creation failed. Try again later.\r\n"))
	}
	return promoteToGame(ctx, s, c, m.repo, m.game)
}

// validateCharacterName mirrors validateUsername — same charset and
// length rules, distinct reserved set. Kept separate from
// validateUsername so the two policies can drift independently as
// account vs. character UX evolves.
func validateCharacterName(name string) error {
	if name == "" {
		return errors.New("Character name cannot be empty")
	}
	n := len(name)
	if n < minCharacterNameLen {
		return errors.New("Character name too short (minimum 3 characters)")
	}
	if n > maxCharacterNameLen {
		return errors.New("Character name too long (maximum 24 characters)")
	}
	for _, r := range name {
		if r > unicode.MaxASCII {
			return errors.New("Character name may only contain ASCII letters, digits, _ or -")
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return errors.New("Character name may only contain letters, digits, _ or -")
		}
	}
	if reservedCharacterNames[strings.ToLower(name)] {
		return errors.New("Character name is reserved. Choose another")
	}
	return nil
}
