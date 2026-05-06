package mode

import (
	"strings"

	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/telnet"
)

// handleQuitEnter pivots to the Y/N quit confirm. Picking Y closes
// the connection cleanly; N (or [B]ack) returns to the root menu.
// Empty input on the confirm step is treated as N — quitting should
// require an explicit affirmative.
func (m *AccountMenu) handleQuitEnter(s *telnet.Session) error {
	m.step = accountStepQuitConfirm
	if err := display.SectionHeader(s, "Quit"); err != nil {
		return err
	}
	return s.WriteRaw([]byte("  Disconnect now? [Y/N] >\r\n"))
}

func (m *AccountMenu) handleQuitConfirmLine(s *telnet.Session, line string) error {
	in := strings.TrimSpace(strings.ToLower(line))
	switch in {
	case "y", "yes":
		_ = s.WriteRaw([]byte("Goodbye.\r\n"))
		_ = s.Conn.Close()
		return telnet.ErrSessionEnded
	case "", "n", "no", "b", "back", "cancel":
		return m.returnToRoot(s)
	}
	if werr := writeError(s, "Please answer Y or N."); werr != nil {
		return werr
	}
	return nil
}
