package cmd

import (
	"log/slog"
	"time"

	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewParry builds the `parry` command — a #18 follow-up that puts the
// actor into a parrying stance for one round. The next incoming
// attack triggers an opposed roll; on a successful parry the attack
// is negated and the attacker becomes flat-footed for one round.
//
// Refused outside an active fight or when the actor has no melee
// weapon equipped — unarmed parry is deferred (combat_18_followups).
func NewParry(mgr *combat.Manager, characters repo.CharacterRepo, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name: "parry",
		Help: "Parry — ready a defensive stance against the next incoming blow",
		Long: "Usage: parry\n" +
			"       You must have a melee weapon wielded. The stance is\n" +
			"       consumed by the first attack against you and lasts only\n" +
			"       through the current round.\n",
		Auth: telnet.AuthPlayer,
		Lag:  1 * time.Second,
		Run: func(c *telnet.Context) error {
			s := c.Session
			if s.CurrentRoomID == 0 || !mgr.Active(s.CurrentRoomID) {
				return s.WriteString("{{You aren't fighting anyone.}}::yellow\r\n")
			}
			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("parry: char lookup failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{Your stance falters.}}::red\r\n")
			}
			weaponID := char.Equipment.Get(creature.SlotPrimaryWield)
			if weaponID == 0 {
				return s.WriteString("{{You need a weapon to parry.}}::yellow\r\n")
			}
			actor := ActorRefForCharacter(s.CharacterID)
			if err := mgr.EnqueueAction(s.CurrentRoomID, actor, combat.Action{
				Kind:     combat.ActionParry,
				WeaponID: weaponID,
			}); err != nil {
				return s.WriteString("{{You can't parry right now.}}::red\r\n")
			}
			actorName := safeActor(s)
			broadcastRoom(sessions, s.CurrentRoomID, s,
				"{{"+actorName+" raises a weapon to parry.}}::yellow\r\n")
			return s.WriteString("{{You raise your weapon to parry the next blow.}}::yellow\r\n")
		},
	}
}
