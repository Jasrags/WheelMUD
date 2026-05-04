package news

import (
	"fmt"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/telnet"
)

// WriteSplash renders the connect-time banner. Called from
// cmd/server/main.go before the login mode pushes its prompt; runs
// on the accept goroutine and is safe under writeMu.
//
// The splash is operator-curated embedded content. See the trust
// boundary note on Entry.Body — these writes interpret cfmt tags,
// so the assets must never include player-derived strings.
func (c *Catalog) WriteSplash(s *telnet.Session) error {
	if c == nil || c.splash == "" {
		return nil
	}
	return s.WriteWrapped(c.splash)
}

// WriteMOTDBlock renders the MOTD body and an unread-news summary
// for `lastSeen`. Called from mode/postauth.promoteToGame after the
// "Playing as ..." line and before the game mode replaces. The
// dispatcher hasn't started yet, so synchronous WriteWrapped is
// fine — no need for WriteAsync.
func (c *Catalog) WriteMOTDBlock(s *telnet.Session, lastSeen time.Time) error {
	if c == nil {
		return nil
	}
	if c.motd != "" {
		body := strings.TrimRight(c.motd, "\r\n")
		if err := s.WriteWrapped(body + "\r\n"); err != nil {
			return err
		}
	}
	unread := c.UnreadCount(lastSeen)
	if unread == 0 {
		return nil
	}
	noun := "entries"
	if unread == 1 {
		noun = "entry"
	}
	line := fmt.Sprintf(
		"{{[news]}}::yellow|bold %d unread %s. Type {{news}}::yellow to read.\r\n",
		unread, noun,
	)
	return s.WriteString(line)
}
