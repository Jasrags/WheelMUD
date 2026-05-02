package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewWho builds the who command. Lists every bound session with its
// character name and idle time, marks the caller "(you)". Class /
// level / title columns are deferred until char-create populates
// the underlying Character fields (ROADMAP §13).
func NewWho(sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name: "who",
		Help: "List connected players",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			snap := sessions.Snapshot()
			rows := make([]string, 0, len(snap))
			now := time.Now().UTC()
			for _, peer := range snap {
				name := peer.CharacterName
				if name == "" {
					// Pre-character session — login or character-select.
					// Show as "(connecting)" rather than the remote address
					// so `who` can't be used to enumerate IPs.
					name = "(connecting)"
				}
				idle := ""
				if d := peer.IdleSince(now); d >= 30*time.Second {
					idle = " idle " + formatIdle(d)
				}
				marker := ""
				if peer == c.Session {
					marker = " (you)"
				}
				rows = append(rows, "- "+name+marker+idle)
			}
			sort.Strings(rows)
			header := fmt.Sprintf("%d player(s) online:\r\n", len(rows))
			return c.Session.WriteRaw([]byte(header + strings.Join(rows, "\r\n") + "\r\n"))
		},
	}
}

// formatIdle renders a duration as a short human-friendly string:
// "45s", "3m", "1h12m". Long enough idles round to whole hours.
func formatIdle(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	}
}
