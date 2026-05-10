package cmd

import (
	"time"

	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewFlee builds the `flee` (alias `run`) command — a #18 follow-up
// peeled off combat_18_followups. Refused outside an active fight;
// inside one, queues an ActionFlee for the actor's slot. The Manager
// hands the actual move + roll off to its FleeMover at resolution
// time so combat stays free of session/world coupling.
func NewFlee(mgr *combat.Manager) *telnet.Command {
	return &telnet.Command{
		Name:    "flee",
		Aliases: []string{"run"},
		Help:    "Flee — try to retreat from the current fight",
		Long: "Usage: flee\n" +
			"       Resolves on your next combat tick. Failure leaves you\n" +
			"       in the fight; success drops you in a neighbouring room.\n",
		Auth: telnet.AuthPlayer,
		Lag:  2 * time.Second,
		Run: func(c *telnet.Context) error {
			s := c.Session
			if s.CurrentRoomID == 0 || !mgr.Active(s.CurrentRoomID) {
				return s.WriteString("{{You aren't fighting anyone.}}::yellow\r\n")
			}
			actor := ActorRefForCharacter(s.CharacterID)
			if err := mgr.EnqueueAction(s.CurrentRoomID, actor, combat.Action{
				Kind: combat.ActionFlee,
			}); err != nil {
				return s.WriteString("{{You can't flee right now.}}::red\r\n")
			}
			return s.WriteString("{{You look for an opening to flee.}}::yellow\r\n")
		},
	}
}
