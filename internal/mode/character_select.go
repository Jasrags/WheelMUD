package mode

import (
	"context"
	"errors"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// CharacterSelect is the legacy multi-character picker. Post-§6
// AccountMenu replaces it as the post-login routing target; this mode
// is preserved only for any direct callers / tests that still push it
// explicitly. New code should prefer AccountMenu.
type CharacterSelect struct {
	chars     []repo.Character
	repo      repo.CharacterRepo
	game      telnet.Mode
	catalog   *chargen.Catalog
	builders  repo.BuilderZoneRepo // optional; forwarded to promoteToGame
	listShown bool
}

func NewCharacterSelect(chars []repo.Character, characters repo.CharacterRepo, game telnet.Mode) *CharacterSelect {
	return &CharacterSelect{chars: chars, repo: characters, game: game}
}

// SetCatalog forwards the chargen catalog to a CharacterCreate
// spawned by the user typing 'create'.
func (m *CharacterSelect) SetCatalog(c *chargen.Catalog) { m.catalog = c }

// SetBuilders wires the per-zone builder-grant repo so promoteToGame
// can cache the character's grants on the session. Optional; nil
// disables the load (Phase G #33).
func (m *CharacterSelect) SetBuilders(r repo.BuilderZoneRepo) { m.builders = r }

func (m *CharacterSelect) Prompt(_ context.Context, _ *telnet.Session) string {
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
		create := NewCharacterCreate(m.repo, m.game)
		create.SetCatalog(m.catalog)
		return s.ReplaceMode(create)
	}

	// Find the requested character within the *account's* list — a user
	// must not be able to play a character they don't own even if they
	// know its name.
	for _, c := range m.chars {
		if strings.EqualFold(c.Name, choice) {
			return promoteToGame(ctx, s, c, m.repo, m.builders, m.game)
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
