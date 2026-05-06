package mode

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// handlePlayEnter handles "1) Play" from the root menu. With one
// character this auto-promotes (the "one keystroke into the world"
// affordance preserved from the legacy verb flow). With more than one
// it pushes the picker.
func (m *AccountMenu) handlePlayEnter(ctx context.Context, s *telnet.Session) error {
	if len(m.chars) == 0 {
		// Defensive: rootMenu hides this option when chars is empty.
		return m.returnToRoot(s)
	}
	if len(m.chars) == 1 {
		return m.promoteIntoCharacter(ctx, s, &m.chars[0])
	}
	m.step = accountStepPlayPicker
	return m.writePlayPicker(s)
}

func (m *AccountMenu) writePlayPicker(s *telnet.Session) error {
	if err := display.SectionHeader(s, "Play"); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte("  Pick a character:\r\n")); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte(m.charListBlock())); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte("\r\n  [B]ack\r\n")); err != nil {
		return err
	}
	return s.WriteRaw([]byte(fmt.Sprintf("[1-%d] >\r\n", len(m.chars))))
}

func (m *AccountMenu) handlePlayPickerLine(ctx context.Context, s *telnet.Session, line string) error {
	if isBack(line) {
		return m.returnToRoot(s)
	}
	idx, err := parsePositiveIndex(strings.TrimSpace(line), len(m.chars))
	if err != nil {
		if werr := writeError(s,
			fmt.Sprintf("Invalid choice. [1-%d] or [B]ack.", len(m.chars))); werr != nil {
			return werr
		}
		return nil
	}
	return m.promoteIntoCharacter(ctx, s, &m.chars[idx])
}

// promoteIntoCharacter is the shared post-pick path. It applies the
// session-scoped account settings (color/width) before promoteToGame
// so the next render lands with the chosen knobs in effect.
func (m *AccountMenu) promoteIntoCharacter(ctx context.Context, s *telnet.Session, c *repo.Character) error {
	applyAccountSettings(s, m.settings)
	return promoteToGame(ctx, s, *c, m.repo, m.game)
}
