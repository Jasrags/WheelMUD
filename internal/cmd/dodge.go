package cmd

import (
	"errors"
	"time"

	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewDodge builds the `dodge` command — a Phase L #64 defensive verb.
// Dex-flavored mirror of `parry`: enters a one-round dodge stance
// that grants +4 Defense and flat-foot immunity against the next
// incoming swing, then is consumed. Unlike parry there is no
// weapon requirement and no opposed roll — the bonus applies
// directly when the resolver computes effective Defense.
func NewDodge(mgr *combat.Manager, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name: "dodge",
		Help: "Dodge — ready a Dex-flavored evasion against the next blow",
		Long: "Usage: dodge\n" +
			"       Grants +4 Defense and flat-foot immunity against the\n" +
			"       next incoming attack. Consumed on the first swing.\n",
		Auth: telnet.AuthPlayer,
		Lag:  500 * time.Millisecond,
		Run: func(c *telnet.Context) error {
			s := c.Session
			if s.CurrentRoomID == 0 || !mgr.Active(s.CurrentRoomID) {
				return s.WriteString("{{You aren't fighting anyone.}}::yellow\r\n")
			}
			actor := ActorRefForCharacter(s.CharacterID)
			if err := mgr.EnqueueAction(s.CurrentRoomID, actor, combat.Action{
				Kind: combat.ActionDodge,
			}); err != nil {
				if errors.Is(err, combat.ErrInsufficientStamina) {
					return s.WriteString("{{You're too winded to dodge.}}::yellow\r\n")
				}
				return s.WriteString("{{You can't dodge right now.}}::red\r\n")
			}
			actorName := safeActor(s)
			broadcastRoom(sessions, s.CurrentRoomID, s,
				"{{"+actorName+" drops into a wary crouch.}}::yellow\r\n")
			return s.WriteString("{{You brace and watch for an opening.}}::yellow\r\n")
		},
	}
}
