package mode

import (
	"time"

	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/telnet"
)

// handleNewsEnter renders the news/MOTD block on demand. The hook is
// the same one postAuth fires once-per-login. The lastSeen watermark
// is preserved so re-entries show the same unread count the post-login
// block did; MOTDAlways flattens that to zero so the full block
// re-renders every time. After the block, we hold the user at this
// substep until they press Enter (or anything else) — that returns to
// root.
func (m *AccountMenu) handleNewsEnter(s *telnet.Session) error {
	if m.motd == nil {
		if werr := writeError(s, "News is not configured on this server."); werr != nil {
			return werr
		}
		return m.returnToRoot(s)
	}
	watermark := m.lastSeen
	if m.settings.MOTDAlways {
		watermark = time.Time{}
	}
	if err := m.motd(s, watermark); err != nil {
		return err
	}
	if err := display.Rule(s); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte("  Press [Enter] to return…\r\n")); err != nil {
		return err
	}
	m.step = accountStepNews
	return nil
}

// handleNewsLine eats one input line and returns to root regardless
// of content. The "press Enter to return" affordance is intentional:
// long news blocks paginate naturally, and the next keystroke just
// gets the user back to the menu.
func (m *AccountMenu) handleNewsLine(s *telnet.Session, _ string) error {
	return m.returnToRoot(s)
}
