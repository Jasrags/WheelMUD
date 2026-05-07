package cmd

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewAttack builds the `attack <target>` command. Slice 1 of Phase D
// #18: starts (or reuses) a per-room Fight, queues an ActionAttack
// against the target for this combatant's slot in the round, and lets
// the combat tick resolve hit/miss/damage.
//
// Refused when the room is Peaceful, when the target is missing, or
// when the actor types only "attack". Re-targeting an in-progress
// fight is allowed and overwrites the previous queued action.
func NewAttack(
	mgr *combat.Manager,
	rooms repo.RoomRepo,
	mobs repo.MobInstanceRepo,
	characters repo.CharacterRepo,
	sessions *session.Registry,
) *telnet.Command {
	return &telnet.Command{
		Name:    "attack",
		Aliases: []string{"kill"},
		Help:    "Attack <target> — engage a foe in melee",
		Long: "Usage: attack <target>\n" +
			"       Re-issuing while a fight is in progress switches your\n" +
			"       queued target without restarting the fight.\n",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Completer: completeAttackTargets(mobs),
		Run: func(c *telnet.Context) error {
			s := c.Session
			if s.CurrentRoomID == 0 {
				return s.WriteString("{{You can't fight here.}}::yellow\r\n")
			}
			target := strings.ToLower(strings.TrimSpace(strings.Join(c.Args, " ")))
			if target == "" {
				return s.WriteString("{{Attack what?}}::yellow\r\n")
			}

			room, err := rooms.FindByID(c.Ctx, s.CurrentRoomID)
			if err == nil && room.Flags.Peaceful {
				return s.WriteString("{{A profound peace settles here. You can't bring yourself to attack.}}::yellow\r\n")
			}

			roomMobs, err := mobs.ListInRoom(c.Ctx, s.CurrentRoomID)
			if err != nil {
				slog.Error("attack: list mobs failed", "room", s.CurrentRoomID, "error", err)
				return s.WriteString("{{Your focus slips.}}::red\r\n")
			}
			mob, ok := MatchMob(target, roomMobs)
			if !ok {
				return s.WriteString("{{You don't see them here.}}::yellow\r\n")
			}

			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("attack: char lookup failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You feel disoriented.}}::red\r\n")
			}
			weaponID := char.Equipment.Get(creature.SlotPrimaryWield)

			actor := ActorRefForCharacter(s.CharacterID)
			defender := ActorRefForMob(mob.ID)

			// Start atomically rejects with ErrFightExists when a fight
			// is already in progress; treating that as success removes
			// the racy Active-then-Start TOCTOU window between two
			// callers in the same room.
			if _, err := mgr.Start(c.Ctx, s.CurrentRoomID,
				[]combat.ActorRef{actor, defender}); err != nil && !errors.Is(err, combat.ErrFightExists) {
				slog.Warn("attack: start fight failed", "room", s.CurrentRoomID, "error", err)
				return s.WriteString("{{You can't engage right now.}}::red\r\n")
			}

			if err := mgr.EnqueueAction(s.CurrentRoomID, actor, combat.Action{
				Kind:     combat.ActionAttack,
				Target:   defender,
				WeaponID: weaponID,
			}); err != nil {
				slog.Warn("attack: enqueue failed", "room", s.CurrentRoomID, "error", err)
				return s.WriteString("{{You can't engage right now.}}::red\r\n")
			}

			actorName := safeActor(s)
			broadcastRoom(sessions, s.CurrentRoomID, s,
				"{{"+actorName+" moves to attack "+mob.Core.Name+".}}::red\r\n")
			return s.WriteString("{{You ready an attack against " + mob.Core.Name + ".}}::red\r\n")
		},
	}
}

// ActorRefForCharacter wraps a character id as a combat ActorRef.
// Exported because the cmd tests build refs directly.
func ActorRefForCharacter(id int64) combat.ActorRef {
	return combat.ActorRef{Kind: combat.ActorKindCharacter, ID: id}
}

// ActorRefForMob wraps a mob instance id as a combat ActorRef.
func ActorRefForMob(id int64) combat.ActorRef {
	return combat.ActorRef{Kind: combat.ActorKindMob, ID: id}
}

// completeAttackTargets returns the mob keyword candidates in the
// actor's room. Slot 0 only — `attack` takes one argument.
func completeAttackTargets(mobs repo.MobInstanceRepo) func(s *telnet.Session, args string) []telnet.Candidate {
	return func(s *telnet.Session, args string) []telnet.Candidate {
		slot, partial := completerSlot(args)
		if slot != 0 || s.CurrentRoomID == 0 {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		list, err := mobs.ListInRoom(ctx, s.CurrentRoomID)
		if err != nil {
			return nil
		}
		return mobKeywordCandidates(list, partial)
	}
}
