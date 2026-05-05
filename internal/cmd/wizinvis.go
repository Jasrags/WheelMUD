package cmd

import "github.com/Jasrags/WheelMUD/telnet"

// NewWizinvis builds the wizinvis admin toggle. Flips the session's
// hidden bit (Session.ToggleHidden); other commands query
// Session.IsHidden to decide whether to surface this session to
// non-admin viewers (`who`, tell-name completion, future
// look-shows-occupants).
//
// Session-scoped: the bit is dropped on disconnect by design. No
// schema change. Persisting wizinvis across reconnect is tracked as
// a follow-up.
func NewWizinvis() *telnet.Command {
	return &telnet.Command{
		Name: "wizinvis",
		Help: "wizinvis — toggle admin invisibility (per session)",
		Auth: telnet.AuthAdmin,
		Run: func(c *telnet.Context) error {
			if c.Session.ToggleHidden() {
				return c.Session.WriteString("{{You fade from sight.}}::magenta\r\n")
			}
			return c.Session.WriteString("{{You return to view.}}::cyan\r\n")
		},
	}
}
