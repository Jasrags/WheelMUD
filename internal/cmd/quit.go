package cmd

import "github.com/Jasrags/WheelMUD/telnet"

var Quit = &telnet.Command{
	Name:    "quit",
	Aliases: []string{"q", "exit"},
	Help:    "Disconnect from the server",
	Run: func(c *telnet.Context) error {
		// Best-effort goodbye; any write error is moot because we are
		// about to close the connection.
		_ = c.Session.WriteRaw([]byte("Goodbye.\r\n"))
		_ = c.Session.Conn.Close()
		return telnet.ErrSessionEnded
	},
}
