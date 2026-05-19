package cmd

import (
	"github.com/Jasrags/WheelMUD/telnet"
)

// PushFlowFn is the boot-time closure that pushes a flow.Mode onto
// the session's mode stack. Defined here so `internal/cmd` doesn't
// import `internal/mode` directly; cmd/server/main.go wires the
// closure with the actual mode.NewFlow + Session.PushMode call.
// Mirrors PushREditFn (§G #34, internal/cmd/redit.go).
type PushFlowFn func(s *telnet.Session, flowID string) error

// NewWizdemo builds the player-facing test verb that walks the
// bundled `wizdemo` flow. AuthPlayer so a normal player can smoke-
// test the engine; the flow is harmless (no DB writes, only an
// slog call from its commit action).
func NewWizdemo(push PushFlowFn) *telnet.Command {
	return &telnet.Command{
		Name: "wizdemo",
		Help: "Walk the flow-engine demo wizard (§O.1 test verb)",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			return push(c.Session, "wizdemo")
		},
	}
}
