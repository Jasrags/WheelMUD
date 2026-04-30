package cmd

import "github.com/Jasrags/WheelMUD/telnet"

// Who is a placeholder until session rosters land. Lists only the caller.
var Who = &telnet.Command{
	Name: "who",
	Help: "List connected players",
	Run: func(c *telnet.Context) error {
		return c.Session.WriteRaw([]byte("- " + c.Session.RemoteAddress + " (you)\r\n"))
	},
}
