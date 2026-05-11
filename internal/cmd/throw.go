package cmd

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/group"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewThrow builds the `throw <weapon> <target>` command — Phase L #64.
// Consumes the wielded thrown weapon (WeaponStats.Range == "thrown")
// from SlotPrimaryWield, resolves a Normal-variant attack against the
// target, and drops the weapon at the target's feet (or into the
// corpse on a kill).
func NewThrow(
	mgr *combat.Manager,
	rooms repo.RoomRepo,
	mobs repo.MobInstanceRepo,
	characters repo.CharacterRepo,
	items repo.ItemRepo,
	sessions *session.Registry,
	groups *group.Manager,
) *telnet.Command {
	return &telnet.Command{
		Name: "throw",
		Help: "Throw <weapon> <target> — fling a thrown weapon at a foe",
		Long: "Usage: throw <weapon> <target>\n" +
			"       The weapon must be currently wielded and tagged as a\n" +
			"       throwable (WeaponStats.Range == \"thrown\"). It is\n" +
			"       consumed from your hand and lands at the target's\n" +
			"       feet — or in the corpse if your throw kills them.\n",
		MinArgs: 2,
		Auth:    telnet.AuthPlayer,
		Lag:     500 * time.Millisecond,
		Run: func(c *telnet.Context) error {
			s := c.Session
			if s.CurrentRoomID == 0 {
				return s.WriteString("{{You can't fight here.}}::yellow\r\n")
			}
			weaponArg := strings.ToLower(strings.TrimSpace(c.Args[0]))
			targetArg := strings.ToLower(strings.TrimSpace(strings.Join(c.Args[1:], " ")))
			if weaponArg == "" || targetArg == "" {
				return s.WriteString("{{Usage: throw <weapon> <target>}}::yellow\r\n")
			}

			room, roomErr := rooms.FindByID(c.Ctx, s.CurrentRoomID)
			if roomErr == nil && room.Flags.Peaceful {
				return s.WriteString("{{A profound peace settles here. You can't bring yourself to throw.}}::yellow\r\n")
			}

			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("throw: char lookup failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You feel disoriented.}}::red\r\n")
			}
			wieldedID := char.Equipment.Get(creature.SlotPrimaryWield)
			if wieldedID == 0 {
				return s.WriteString("{{You're not wielding anything.}}::yellow\r\n")
			}
			weapon, err := items.GetByID(c.Ctx, wieldedID)
			if err != nil {
				slog.Error("throw: weapon lookup failed",
					"item", wieldedID, "error", err)
				return s.WriteString("{{You can't seem to find your weapon.}}::red\r\n")
			}
			// Match the weapon-keyword argument against the wielded item.
			// Throw V1 only supports the primary wield slot, so we don't
			// need to walk inventory — but the arg must still match the
			// wielded item so a typo doesn't silently consume the wrong
			// thing.
			if !itemMatchesKeyword(weapon, weaponArg) {
				return s.WriteString("{{You're not wielding that.}}::yellow\r\n")
			}
			ws, ok := weapon.Stats.(*repo.WeaponStats)
			if !ok || ws == nil || !strings.EqualFold(ws.Range, "thrown") {
				return s.WriteString("{{That won't fly true — it isn't a throwing weapon.}}::yellow\r\n")
			}

			roomMobs, err := mobs.ListInRoom(c.Ctx, s.CurrentRoomID)
			if err != nil {
				slog.Error("throw: list mobs failed", "room", s.CurrentRoomID, "error", err)
				return s.WriteString("{{Your aim slips.}}::red\r\n")
			}
			actor := ActorRefForCharacter(s.CharacterID)
			var (
				defender   combat.ActorRef
				targetName string
				targetPeer *telnet.Session
			)
			if mob, mobOK := MatchMob(targetArg, roomMobs); mobOK {
				defender = ActorRefForMob(mob.ID)
				targetName = mob.Core.Name
			} else if peer, _ := MatchPlayer(targetArg, sessions, s); peer != nil {
				targetPeer = peer
				targetChar, err := characters.GetByID(c.Ctx, peer.CharacterID)
				if err != nil {
					slog.Error("throw: target char lookup failed",
						"char", peer.CharacterID, "error", err)
					return s.WriteString("{{They slip from your focus.}}::red\r\n")
				}
				sameGroup := groups != nil && groups.SameGroup(s.CharacterID, targetChar.ID)
				if msg, ok := pvpRefusalReason(roomErr == nil, room, char, targetChar, sameGroup); !ok {
					return s.WriteString(msg)
				}
				defender = ActorRefForCharacter(targetChar.ID)
				targetName = targetChar.Name
			} else {
				return s.WriteString("{{You don't see them here.}}::yellow\r\n")
			}

			if _, err := mgr.Start(c.Ctx, s.CurrentRoomID,
				[]combat.ActorRef{actor, defender}); err != nil && !errors.Is(err, combat.ErrFightExists) {
				slog.Warn("throw: start fight failed", "room", s.CurrentRoomID, "error", err)
				return s.WriteString("{{You can't engage right now.}}::red\r\n")
			}

			if err := mgr.EnqueueAction(s.CurrentRoomID, actor, combat.Action{
				Kind:     combat.ActionThrow,
				Target:   defender,
				WeaponID: wieldedID,
			}); err != nil {
				if errors.Is(err, combat.ErrInsufficientStamina) {
					return s.WriteString("{{You're too winded to throw.}}::yellow\r\n")
				}
				slog.Warn("throw: enqueue failed", "room", s.CurrentRoomID, "error", err)
				return s.WriteString("{{You can't engage right now.}}::red\r\n")
			}

			actorName := safeActor(s)
			itemName := weapon.Name
			if itemName == "" {
				itemName = "a weapon"
			}
			if targetPeer != nil {
				if err := targetPeer.WriteAsync(
					"{{" + actorName + " takes aim and hurls " + itemName + " at you!}}::red\r\n"); err != nil {
					slog.Debug("throw: defender notify failed",
						"to", targetName, "error", err)
				}
			}
			broadcastRoomExcept2(sessions, s.CurrentRoomID, s, targetPeer,
				"{{"+actorName+" hurls "+itemName+" at "+targetName+".}}::red\r\n")
			return s.WriteString("{{You take aim and throw " + itemName + " at " + targetName + ".}}::red\r\n")
		},
	}
}

// itemMatchesKeyword tests a single item against a keyword using the
// same token-prefix rules as MatchItem. Strips a leading "<n>." ordinal
// prefix so a player typing `throw 2.knife <target>` still matches the
// one wielded knife (V1 throw only resolves the primary wield slot, so
// the ordinal can't disambiguate beyond that — we just don't refuse on
// a non-fatal typing habit).
func itemMatchesKeyword(it repo.Item, keyword string) bool {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return false
	}
	_, kw := parseOrdinal(keyword)
	if kw == "" {
		return false
	}
	return nameMatches(it.Name, kw)
}
