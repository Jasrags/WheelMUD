package cmd

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewSidestep builds the `sidestep <attacker>` command — a Phase L #64
// positional verb. Stamps FlatFootedUntil on the named attacker for
// one round (loses Dex bonus to Defense on the next swing landed
// against them). No damage, no opposed roll — cheap setup play.
func NewSidestep(
	mgr *combat.Manager,
	mobs repo.MobInstanceRepo,
	sessions *session.Registry,
) *telnet.Command {
	return &telnet.Command{
		Name: "sidestep",
		Help: "Sidestep <attacker> — flat-foot a named foe for one round",
		Long: "Usage: sidestep <attacker>\n" +
			"       Reads the named attacker out of the active fight and\n" +
			"       drops their Dex bonus to Defense on the next swing\n" +
			"       landed against them.\n",
		MinArgs: 1,
		Auth:    telnet.AuthPlayer,
		Lag:     500 * time.Millisecond,
		Run: func(c *telnet.Context) error {
			s := c.Session
			if s.CurrentRoomID == 0 || !mgr.Active(s.CurrentRoomID) {
				return s.WriteString("{{You aren't fighting anyone.}}::yellow\r\n")
			}
			target := strings.ToLower(strings.TrimSpace(strings.Join(c.Args, " ")))
			if target == "" {
				return s.WriteString("{{Sidestep whom?}}::yellow\r\n")
			}

			roomMobs, err := mobs.ListInRoom(c.Ctx, s.CurrentRoomID)
			if err != nil {
				slog.Error("sidestep: list mobs failed",
					"room", s.CurrentRoomID, "error", err)
				return s.WriteString("{{Your focus slips.}}::red\r\n")
			}
			var (
				targetRef  combat.ActorRef
				targetName string
				found      bool
			)
			if mob, ok := MatchMob(target, roomMobs); ok {
				targetRef = ActorRefForMob(mob.ID)
				targetName = mob.Core.Name
				found = true
			} else if peer, _ := MatchPlayer(target, sessions, s); peer != nil {
				targetRef = ActorRefForCharacter(peer.CharacterID)
				targetName = peer.CharacterName
				found = true
			}
			if !found {
				return s.WriteString("{{You don't see them here.}}::yellow\r\n")
			}

			actor := ActorRefForCharacter(s.CharacterID)
			if err := mgr.EnqueueAction(s.CurrentRoomID, actor, combat.Action{
				Kind:   combat.ActionSidestep,
				Target: targetRef,
			}); err != nil {
				if errors.Is(err, combat.ErrInsufficientStamina) {
					return s.WriteString("{{You're too winded to sidestep.}}::yellow\r\n")
				}
				return s.WriteString("{{You can't sidestep right now.}}::red\r\n")
			}

			actorName := safeActor(s)
			broadcastRoom(sessions, s.CurrentRoomID, s,
				"{{"+actorName+" eyes "+targetName+"'s footing.}}::yellow\r\n")
			return s.WriteString("{{You slip aside, watching " + targetName + "'s next swing.}}::yellow\r\n")
		},
	}
}
