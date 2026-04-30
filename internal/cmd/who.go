package cmd

import "github.com/Jasrags/WheelMUD/telnet"

// Who is a placeholder until the session registry lands. Lists only the
// caller and falls back to the remote address when the session has not
// yet picked a character.
var Who = &telnet.Command{
	Name: "who",
	Help: "List connected players",
	Run: func(c *telnet.Context) error {
		who := c.Session.CharacterName
		if who == "" {
			who = c.Session.RemoteAddress
		}
		return c.Session.WriteRaw([]byte("- " + who + " (you)\r\n"))
	},
}
