package cmd

import (
	"fmt"

	"github.com/Jasrags/WheelMUD/telnet"
)

var TogglePassword = &telnet.Command{
	Name:    "togglepassword",
	Aliases: []string{"tp"},
	Help:    "Toggle password-style input echo (debug)",
	Run: func(c *telnet.Context) error {
		c.Session.InPasswordMode = !c.Session.InPasswordMode
		status := "off"
		if c.Session.InPasswordMode {
			status = "on"
		}
		return c.Session.WriteRaw([]byte(fmt.Sprintf("Password mode %s\r\n", status)))
	},
}
