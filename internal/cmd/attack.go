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

			room, roomErr := rooms.FindByID(c.Ctx, s.CurrentRoomID)
			if roomErr == nil && room.Flags.Peaceful {
				return s.WriteString("{{A profound peace settles here. You can't bring yourself to attack.}}::yellow\r\n")
			}

			roomMobs, err := mobs.ListInRoom(c.Ctx, s.CurrentRoomID)
			if err != nil {
				slog.Error("attack: list mobs failed", "room", s.CurrentRoomID, "error", err)
				return s.WriteString("{{Your focus slips.}}::red\r\n")
			}
			mob, mobOK := MatchMob(target, roomMobs)

			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("attack: char lookup failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You feel disoriented.}}::red\r\n")
			}

			actor := ActorRefForCharacter(s.CharacterID)
			var (
				defender    combat.ActorRef
				targetName  string
				targetPeer  *telnet.Session // non-nil for player targets only
			)

			if mobOK {
				defender = ActorRefForMob(mob.ID)
				targetName = mob.Core.Name
			} else {
				peer := matchPlayerInRoom(target, s, sessions)
				if peer == nil {
					return s.WriteString("{{You don't see them here.}}::yellow\r\n")
				}
				targetPeer = peer
				targetChar, err := characters.GetByID(c.Ctx, peer.CharacterID)
				if err != nil {
					slog.Error("attack: target char lookup failed",
						"char", peer.CharacterID, "error", err)
					return s.WriteString("{{They slip from your focus.}}::red\r\n")
				}
				if msg, ok := pvpRefusalReason(roomErr == nil, room, char, targetChar); !ok {
					return s.WriteString(msg)
				}
				defender = ActorRefForCharacter(targetChar.ID)
				targetName = targetChar.Name
			}

			weaponID := char.Equipment.Get(creature.SlotPrimaryWield)

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
			// Defender-specific second-person line lets a player notice
			// they're being attacked without having to parse the room's
			// third-person narration. Mobs (targetPeer == nil) keep the
			// single broadcastRoom call.
			if targetPeer != nil {
				if err := targetPeer.WriteAsync(
					"{{" + actorName + " readies an attack against you!}}::red\r\n"); err != nil {
					slog.Debug("attack: defender notify failed",
						"to", targetName, "error", err)
				}
			}
			broadcastRoomExcept2(sessions, s.CurrentRoomID, s, targetPeer,
				"{{"+actorName+" moves to attack "+targetName+".}}::red\r\n")
			return s.WriteString("{{You ready an attack against " + targetName + ".}}::red\r\n")
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

// matchPlayerInRoom returns the first peer session whose CharacterName
// has a token-prefix match against target (case-insensitive) and who
// shares the actor's CurrentRoomID. Returns nil on miss. Hidden peers
// (wizinvis) are skipped so a non-admin actor can't probe them; the
// actor's own session is filtered out so `attack <self>` falls
// through to "don't see them here".
//
// Cross-goroutine field reads (peer.CurrentRoomID, peer.CharacterName)
// are unsynchronized — same pattern as session.Registry.FindByCharacterName
// and cmd/comm.go::onlineNameCandidates. CLAUDE.md treats these
// snapshot reads as tolerated stale-but-coherent values; the verb-
// layer guard (pvpRefusalReason) re-fetches the canonical
// repo.Character before any state change, so a racing room move
// only widens the matching window by one tick.
func matchPlayerInRoom(target string, self *telnet.Session, sessions *session.Registry) *telnet.Session {
	if sessions == nil || self == nil || self.CurrentRoomID == 0 {
		return nil
	}
	lower := strings.ToLower(strings.TrimSpace(target))
	if lower == "" {
		return nil
	}
	for _, peer := range sessions.Snapshot() {
		if peer == self {
			continue
		}
		if peer.CurrentRoomID != self.CurrentRoomID {
			continue
		}
		if peer.IsHidden() && self.AuthLevel < telnet.AuthAdmin {
			continue
		}
		name := strings.ToLower(peer.CharacterName)
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, lower) {
			return peer
		}
	}
	return nil
}

// pvpRefusalReason runs the PvP guard for an attacker→target pair.
// Returns ("", true) when the attack is allowed, otherwise a refusal
// line (cfmt-tagged, CRLF-terminated) and false. Refusals are checked
// in priority order so the message matches the first failing gate.
//
//	1. nopvp room flag (room.Flags.NoPVP) — also refused when
//	   roomKnown=false; we fail closed on the safety gate rather
//	   than let a transient rooms.FindByID error open a PvP path
//	   the room flag would have closed.
//	2. attacker below newbie cap
//	3. target below newbie cap
//	4. attacker has not opted in
//	5. target has not opted in
func pvpRefusalReason(roomKnown bool, room repo.Room, attacker, defender repo.Character) (string, bool) {
	if !roomKnown || room.Flags.NoPVP {
		return "{{These grounds are sanctified — no violence between travelers here.}}::yellow\r\n", false
	}
	if characterLevel(attacker) < NewbiePvPLevelCap {
		return "{{You are still too green for the killing fields.}}::yellow\r\n", false
	}
	if characterLevel(defender) < NewbiePvPLevelCap {
		return "{{" + defender.Name + " is too green to be a fair target.}}::yellow\r\n", false
	}
	if !attacker.PvP {
		return "{{You haven't enabled PvP — see `help pvp`.}}::yellow\r\n", false
	}
	if !defender.PvP {
		return "{{" + defender.Name + " hasn't enabled PvP.}}::yellow\r\n", false
	}
	return "", true
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
