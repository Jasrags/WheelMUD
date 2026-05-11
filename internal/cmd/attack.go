package cmd

import (
	"context"
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

// attackDeps bundles the repos / registries the attack family of
// verbs reads. Pinned in one struct so NewAttack, NewPower, and
// NewJab share a single runAttack helper without re-declaring six
// constructor parameters apiece.
type attackDeps struct {
	mgr        *combat.Manager
	rooms      repo.RoomRepo
	mobs       repo.MobInstanceRepo
	characters repo.CharacterRepo
	sessions   *session.Registry
	groups     *group.Manager
}

// NewAttack builds the `attack <target> [power|quick]` command.
// Slice 1 of Phase D #18 started the fight pipeline; Phase L #61
// extends the verb with optional Power/Quick variants. Re-issuing
// during a fight is allowed and overwrites the previous queued
// action — variant included.
func NewAttack(
	mgr *combat.Manager,
	rooms repo.RoomRepo,
	mobs repo.MobInstanceRepo,
	characters repo.CharacterRepo,
	sessions *session.Registry,
	groups *group.Manager,
) *telnet.Command {
	deps := attackDeps{mgr: mgr, rooms: rooms, mobs: mobs, characters: characters, sessions: sessions, groups: groups}
	return &telnet.Command{
		Name:    "attack",
		Aliases: []string{"kill"},
		Help:    "Attack <target> [power|quick] — engage a foe in melee",
		Long: "Usage: attack <target> [power|quick]\n" +
			"       Re-issuing while a fight is in progress switches your\n" +
			"       queued target/variant without restarting the fight.\n" +
			"       `power` trades speed for damage; `quick` trades damage for speed.\n",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Lag:       500 * time.Millisecond,
		Completer: completeAttackTargets(mobs),
		Run: func(c *telnet.Context) error {
			target, variant, parsed := parseAttackArgs(c.Args)
			if !parsed {
				// Trailing variant token present with no target: refuse
				// with usage. Bare "attack" (no args) keeps the original
				// short refusal so the empty-args test still reads.
				if len(c.Args) == 0 {
					return c.Session.WriteString("{{Attack what?}}::yellow\r\n")
				}
				return c.Session.WriteString("{{Usage: attack <target> [power|quick]}}::yellow\r\n")
			}
			return runAttack(c, deps, target, variant)
		},
	}
}

// NewPower builds the `power <target>` command — alias for
// `attack <target> power`. Slower swing, ×1.5 damage, -2 attack.
func NewPower(
	mgr *combat.Manager,
	rooms repo.RoomRepo,
	mobs repo.MobInstanceRepo,
	characters repo.CharacterRepo,
	sessions *session.Registry,
	groups *group.Manager,
) *telnet.Command {
	deps := attackDeps{mgr: mgr, rooms: rooms, mobs: mobs, characters: characters, sessions: sessions, groups: groups}
	return &telnet.Command{
		Name:      "power",
		Help:      "Power <target> — slower, heavier melee strike",
		Long:      "Usage: power <target>\n       Trades cadence for damage (×1.5, -2 to hit).\n",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Lag:       500 * time.Millisecond,
		Completer: completeAttackTargets(mobs),
		Run: func(c *telnet.Context) error {
			target := strings.ToLower(strings.TrimSpace(strings.Join(c.Args, " ")))
			if target == "" {
				return c.Session.WriteString("{{Power-strike what?}}::yellow\r\n")
			}
			return runAttack(c, deps, target, combat.VariantPower)
		},
	}
}

// NewJab builds the `jab <target>` command — alias for
// `attack <target> quick`. Faster swing, ×0.6 damage (floor 1), +1 attack.
func NewJab(
	mgr *combat.Manager,
	rooms repo.RoomRepo,
	mobs repo.MobInstanceRepo,
	characters repo.CharacterRepo,
	sessions *session.Registry,
	groups *group.Manager,
) *telnet.Command {
	deps := attackDeps{mgr: mgr, rooms: rooms, mobs: mobs, characters: characters, sessions: sessions, groups: groups}
	return &telnet.Command{
		Name:      "jab",
		Help:      "Jab <target> — quick, lighter melee strike",
		Long:      "Usage: jab <target>\n       Trades damage for cadence (×0.6, +1 to hit).\n",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Lag:       500 * time.Millisecond,
		Completer: completeAttackTargets(mobs),
		Run: func(c *telnet.Context) error {
			target := strings.ToLower(strings.TrimSpace(strings.Join(c.Args, " ")))
			if target == "" {
				return c.Session.WriteString("{{Jab what?}}::yellow\r\n")
			}
			return runAttack(c, deps, target, combat.VariantQuick)
		},
	}
}

// parseAttackArgs splits `attack <target...> [power|quick]` into the
// lowercase target string and a variant. Returns ok=false when the
// trailing token is present but isn't a known variant — refusing
// rather than silently treating "attack x sideways" as Normal.
// A target with embedded spaces is supported (`attack iron golem
// power`).
func parseAttackArgs(args []string) (target string, variant combat.AttackVariant, ok bool) {
	if len(args) == 0 {
		return "", combat.VariantNormal, false
	}
	tail := strings.ToLower(strings.TrimSpace(args[len(args)-1]))
	switch tail {
	case "power":
		variant = combat.VariantPower
		args = args[:len(args)-1]
	case "quick":
		variant = combat.VariantQuick
		args = args[:len(args)-1]
	case "normal":
		args = args[:len(args)-1]
	}
	// Trailing token consumed; if zero args remain we have no target.
	if len(args) == 0 {
		return "", variant, false
	}
	target = strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
	if target == "" {
		return "", variant, false
	}
	return target, variant, true
}

// runAttack is the shared body of the attack-family verbs. variant
// selects Normal / Power / Quick; everything else is identical.
func runAttack(c *telnet.Context, deps attackDeps, target string, variant combat.AttackVariant) error {
	s := c.Session
	if s.CurrentRoomID == 0 {
		return s.WriteString("{{You can't fight here.}}::yellow\r\n")
	}

	room, roomErr := deps.rooms.FindByID(c.Ctx, s.CurrentRoomID)
	if roomErr == nil && room.Flags.Peaceful {
		return s.WriteString("{{A profound peace settles here. You can't bring yourself to attack.}}::yellow\r\n")
	}

	roomMobs, err := deps.mobs.ListInRoom(c.Ctx, s.CurrentRoomID)
	if err != nil {
		slog.Error("attack: list mobs failed", "room", s.CurrentRoomID, "error", err)
		return s.WriteString("{{Your focus slips.}}::red\r\n")
	}
	mob, mobOK := MatchMob(target, roomMobs)

	char, err := deps.characters.FindByName(c.Ctx, s.CharacterName)
	if err != nil {
		slog.Error("attack: char lookup failed", "char", s.CharacterID, "error", err)
		return s.WriteString("{{You feel disoriented.}}::red\r\n")
	}

	actor := ActorRefForCharacter(s.CharacterID)
	var (
		defender   combat.ActorRef
		targetName string
		targetPeer *telnet.Session // non-nil for player targets only
	)

	if mobOK {
		defender = ActorRefForMob(mob.ID)
		targetName = mob.Core.Name
	} else {
		peer, _ := MatchPlayer(target, deps.sessions, s)
		if peer == nil {
			return s.WriteString("{{You don't see them here.}}::yellow\r\n")
		}
		targetPeer = peer
		targetChar, err := deps.characters.GetByID(c.Ctx, peer.CharacterID)
		if err != nil {
			slog.Error("attack: target char lookup failed",
				"char", peer.CharacterID, "error", err)
			return s.WriteString("{{They slip from your focus.}}::red\r\n")
		}
		sameGroup := deps.groups != nil && deps.groups.SameGroup(s.CharacterID, targetChar.ID)
		if msg, ok := pvpRefusalReason(roomErr == nil, room, char, targetChar, sameGroup); !ok {
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
	if _, err := deps.mgr.Start(c.Ctx, s.CurrentRoomID,
		[]combat.ActorRef{actor, defender}); err != nil && !errors.Is(err, combat.ErrFightExists) {
		slog.Warn("attack: start fight failed", "room", s.CurrentRoomID, "error", err)
		return s.WriteString("{{You can't engage right now.}}::red\r\n")
	}

	if err := deps.mgr.EnqueueAction(s.CurrentRoomID, actor, combat.Action{
		Kind:     combat.ActionAttack,
		Variant:  variant,
		Target:   defender,
		WeaponID: weaponID,
	}); err != nil {
		slog.Warn("attack: enqueue failed", "room", s.CurrentRoomID, "error", err)
		return s.WriteString("{{You can't engage right now.}}::red\r\n")
	}

	actorName := safeActor(s)
	defenderLine, roomVerb, selfLine := attackPrepLines(variant, targetName)
	if targetPeer != nil {
		if err := targetPeer.WriteAsync(
			"{{" + actorName + " " + defenderLine + "}}::red\r\n"); err != nil {
			slog.Debug("attack: defender notify failed",
				"to", targetName, "error", err)
		}
	}
	broadcastRoomExcept2(deps.sessions, s.CurrentRoomID, s, targetPeer,
		"{{"+actorName+" "+roomVerb+" "+targetName+".}}::red\r\n")
	return s.WriteString(selfLine)
}

// attackPrepLines returns the variant-flavored phrasing used at
// queue time: the defender-facing tail ("readies an attack against
// you!"), the room broadcast verb ("moves to attack" / "winds up a
// power strike at" / "snaps off a quick jab at"), and the
// first-person self echo. Normal-variant copy is bit-identical to
// the pre-#61 lines so existing tests stay green.
func attackPrepLines(v combat.AttackVariant, target string) (defenderTail, roomVerb, selfLine string) {
	switch v {
	case combat.VariantPower:
		return "winds up a power strike against you!",
			"winds up a power strike at",
			"{{You wind up a power strike against " + target + ".}}::red\r\n"
	case combat.VariantQuick:
		return "snaps off a quick jab at you!",
			"snaps off a quick jab at",
			"{{You snap off a quick jab at " + target + ".}}::red\r\n"
	default:
		return "readies an attack against you!",
			"moves to attack",
			"{{You ready an attack against " + target + ".}}::red\r\n"
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

// pvpRefusalReason runs the PvP guard for an attacker→target pair.
// Returns ("", true) when the attack is allowed, otherwise a refusal
// line (cfmt-tagged, CRLF-terminated) and false. Refusals are checked
// in priority order so the message matches the first failing gate.
//
//	1. nopvp room flag (room.Flags.NoPVP) — also refused when
//	   roomKnown=false; we fail closed on the safety gate rather
//	   than let a transient rooms.FindByID error open a PvP path
//	   the room flag would have closed.
//	2. same-group comrade — Phase D #22 slice 2; precedes the
//	   newbie cap so a refusal cites the group bond rather than
//	   the level gate.
//	3. attacker below newbie cap
//	4. target below newbie cap
//	5. attacker has not opted in
//	6. target has not opted in
func pvpRefusalReason(roomKnown bool, room repo.Room, attacker, defender repo.Character, sameGroup bool) (string, bool) {
	if !roomKnown || room.Flags.NoPVP {
		return "{{These grounds are sanctified — no violence between travelers here.}}::yellow\r\n", false
	}
	if sameGroup {
		return "{{" + defender.Name + " is a comrade — you won't strike them.}}::yellow\r\n", false
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
