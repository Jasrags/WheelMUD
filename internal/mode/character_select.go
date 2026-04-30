package mode

import (
	"context"
	"errors"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// CharacterSelect lets a multi-character account pick which character
// to play. Single-character and zero-character accounts skip this mode
// entirely (postAuth promotes them straight to game or character-
// create). The mode therefore always has 2+ entries to render.
type CharacterSelect struct {
	chars     []repo.Character
	repo      repo.CharacterRepo
	game      telnet.Mode
	listShown bool
}

func NewCharacterSelect(chars []repo.Character, characters repo.CharacterRepo, game telnet.Mode) *CharacterSelect {
	return &CharacterSelect{chars: chars, repo: characters, game: game}
}

func (m *CharacterSelect) Prompt(_ *telnet.Session) string {
	return "Pick a character (or 'create' / 'quit'): "
}

func (m *CharacterSelect) OnEnter(s *telnet.Session) error {
	// Render the list once on entry. Subsequent re-prompts (e.g. after
	// an invalid input) just show the prompt — the list is verbose and
	// scrolls on a small terminal otherwise.
	if m.listShown {
		return nil
	}
	m.listShown = true
	var b strings.Builder
	b.WriteString("\r\nYour characters:\r\n")
	for _, c := range m.chars {
		b.WriteString("  ")
		b.WriteString(c.Name)
		b.WriteString("\r\n")
	}
	return s.WriteRaw([]byte(b.String()))
}

func (m *CharacterSelect) OnExit(_ *telnet.Session) error { return nil }

func (m *CharacterSelect) Handle(ctx context.Context, s *telnet.Session, line string) error {
	choice := strings.TrimSpace(line)
	switch {
	case choice == "":
		return nil
	case strings.EqualFold(choice, "quit"):
		_ = s.WriteRaw([]byte("Goodbye.\r\n"))
		_ = s.Conn.Close()
		return telnet.ErrSessionEnded
	case strings.EqualFold(choice, "create"):
		return s.ReplaceMode(NewCharacterCreate(m.repo, m.game))
	}

	// Find the requested character within the *account's* list — a user
	// must not be able to play a character they don't own even if they
	// know its name.
	for _, c := range m.chars {
		if strings.EqualFold(c.Name, choice) {
			return promoteToGame(ctx, s, c, m.repo, m.game)
		}
	}

	// Re-resolve via repo just so the error message can distinguish
	// "no such character anywhere" from "not yours" without leaking
	// either piece of information.
	if _, err := m.repo.FindByName(ctx, choice); err != nil && !errors.Is(err, repo.ErrCharacterNotFound) {
		return s.WriteRaw([]byte("Could not load that character. Try again.\r\n"))
	}
	return s.WriteRaw([]byte("No such character on this account.\r\n"))
}
