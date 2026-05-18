package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/affects"
	"github.com/Jasrags/WheelMUD/internal/cmd"
	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// combatSubscriberDeps bundles the repos + services every combat-side
// event subscriber needs. Built once in main() and passed straight to
// setupCombatSubscribers — the broadcast closures and per-event
// handlers all capture from this struct so the wiring stays in one
// place.
type combatSubscriberDeps struct {
	bus        *eventbus.Bus
	sessions   *session.Registry
	characters repo.CharacterRepo
	mobs       repo.MobInstanceRepo
	items      repo.ItemRepo
	rooms      repo.RoomRepo
	exits      repo.ExitRepo
	clock      *world.Clock
}

// setupCombatSubscribers wires every combat-render subscriber onto the
// event bus. Inline broadcast closures (combatBroadcast /
// combatBroadcastExcept / combatBroadcastSkip) capture d.sessions so
// per-event subscribers don't have to thread it.
//
// Phase D #18 combat-render subscribers. Each one snapshots names
// via best-effort repo lookups so a despawned participant still
// produces readable output. The "skip-N-sessions" broadcasts use
// inline Snapshot loops rather than chaining combatBroadcastExcept
// — for Hit/Miss the attacker AND defender both need second-person
// echoes, so the third-person broadcast must skip both.
func setupCombatSubscribers(d combatSubscriberDeps) {
	combatBroadcast := func(roomID int64, msg string) {
		for _, peer := range d.sessions.Snapshot() {
			if peer == nil || peer.CurrentRoomID != roomID {
				continue
			}
			if err := peer.WriteAsync(msg); err != nil {
				slog.Debug("combat: broadcast write failed", "error", err)
			}
		}
	}
	// combatBroadcastExcept is the §19 player-death variant that
	// skips one specific session — used so the dying player doesn't
	// see "X falls dead!" alongside their "You die!" line, and the
	// respawned player doesn't see "X appears" stacked on top of
	// their own bound-room render.
	combatBroadcastExcept := func(roomID int64, msg string, exclude *telnet.Session) {
		for _, peer := range d.sessions.Snapshot() {
			if peer == nil || peer == exclude || peer.CurrentRoomID != roomID {
				continue
			}
			if err := peer.WriteAsync(msg); err != nil {
				slog.Debug("combat: broadcast write failed", "error", err)
			}
		}
	}
	eventbus.Subscribe(d.bus, func(ctx context.Context, ev combat.CombatStance) {
		name := combatActorName(ctx, ev.Actor, d.characters, d.mobs)
		switch ev.Kind {
		case "parry":
			combatBroadcast(ev.RoomID, "{{"+name+" raises a weapon to parry.}}::yellow\r\n")
		case "dodge":
			combatBroadcast(ev.RoomID, "{{"+name+" drops into a wary crouch.}}::yellow\r\n")
		case "sidestep":
			tgt := combatActorName(ctx, ev.Target, d.characters, d.mobs)
			combatBroadcast(ev.RoomID, "{{"+name+" eyes "+tgt+"'s footing.}}::yellow\r\n")
		}
	})
	eventbus.Subscribe(d.bus, func(ctx context.Context, ev combat.CombatParry) {
		def := combatActorName(ctx, ev.Defender, d.characters, d.mobs)
		atk := combatActorName(ctx, ev.Attacker, d.characters, d.mobs)
		combatBroadcast(ev.RoomID, "{{"+def+" parries "+atk+"'s blow!}}::cyan\r\n")
	})
	eventbus.Subscribe(d.bus, func(ctx context.Context, ev combat.CombatFlee) {
		if ev.Success {
			// Source-room broadcast lands inline from the FleeMover so
			// the third-person line is colocated with the actual move.
			// The CombatFlee subscriber only emits failure feedback so
			// peers see the tense beat ("an orc tries to flee but is
			// cut off!").
			return
		}
		name := combatActorName(ctx, ev.Actor, d.characters, d.mobs)
		combatBroadcast(ev.RoomID, "{{"+name+" tries to flee but is cut off!}}::yellow\r\n")
	})

	// combatBroadcastSkip is a two-exclude variant of combatBroadcast.
	// Inline so the closure can capture sessions without forcing every
	// per-event subscriber to thread an extra arg.
	combatBroadcastSkip := func(roomID int64, msg string, a, b *telnet.Session) {
		for _, peer := range d.sessions.Snapshot() {
			if peer == nil || peer == a || peer == b || peer.CurrentRoomID != roomID {
				continue
			}
			if err := peer.WriteAsync(msg); err != nil {
				slog.Debug("combat: broadcast write failed", "error", err)
			}
		}
	}

	eventbus.Subscribe(d.bus, func(ctx context.Context, ev combat.CombatDodgeAvoided) {
		def := combatActorName(ctx, ev.Defender, d.characters, d.mobs)
		atk := combatActorName(ctx, ev.Attacker, d.characters, d.mobs)
		var defSess, atkSess *telnet.Session
		if ev.Defender.Kind == combat.ActorKindCharacter {
			defSess = cmd.LookupByCharacterID(d.sessions, ev.Defender.ID)
			if defSess != nil {
				_ = defSess.WriteAsync("{{You twist aside, evading " + atk + "'s blow.}}::cyan\r\n")
			}
		}
		if ev.Attacker.Kind == combat.ActorKindCharacter {
			atkSess = cmd.LookupByCharacterID(d.sessions, ev.Attacker.ID)
			if atkSess != nil {
				_ = atkSess.WriteAsync("{{" + def + " twists aside — your swing whiffs.}}::yellow\r\n")
			}
		}
		combatBroadcastSkip(ev.RoomID,
			"{{"+def+" ducks "+atk+"'s blow!}}::cyan\r\n", atkSess, defSess)
	})

	// CombatHit: per-attacker echo, per-defender echo (if player),
	// third-person line to room peers excluding both. Crit adds a
	// suffix so the player sees the dice-result of their roll.
	eventbus.Subscribe(d.bus, func(ctx context.Context, ev combat.CombatHit) {
		atkName := combatActorName(ctx, ev.Attacker, d.characters, d.mobs)
		defName := combatActorName(ctx, ev.Defender, d.characters, d.mobs)
		// SRD-style critical confirmation (Phase D #18 polish): a
		// threatened crit that confirms renders the existing "(critical!)"
		// tail; a threat that fails to confirm tells the player why a
		// natural 20 didn't multiply damage. Non-threat hits get no tail.
		critTail := ""
		switch {
		case ev.IsCrit:
			critTail = " {{(critical!)}}::yellow|bold"
		case ev.Threat:
			critTail = " {{(threat — but fails to confirm)}}::yellow"
		}
		// Phase D slice 4: off-hand swings carry an "(off-hand)" tag on
		// the attacker and room broadcast lines so dual-wielders can
		// tell their chains apart. Defender line stays unchanged — the
		// target doesn't care which hand bit them.
		offTag := ""
		if ev.OffHand {
			offTag = " {{(off-hand)}}::gray"
		}
		var atkSess, defSess *telnet.Session
		if ev.Attacker.Kind == combat.ActorKindCharacter {
			atkSess = cmd.LookupByCharacterID(d.sessions, ev.Attacker.ID)
			if atkSess != nil {
				_ = atkSess.WriteAsync(fmt.Sprintf(variantHitSelfFormat(ev.Variant),
					defName, ev.Damage, critTail) + offTag)
			}
		}
		if ev.Defender.Kind == combat.ActorKindCharacter {
			defSess = cmd.LookupByCharacterID(d.sessions, ev.Defender.ID)
			if defSess != nil {
				_ = defSess.WriteAsync(fmt.Sprintf("{{%s hits you for %d damage.}}::red%s",
					atkName, ev.Damage, critTail))
			}
		}
		combatBroadcastSkip(ev.RoomID,
			fmt.Sprintf("{{%s hits %s for %d damage.}}::yellow%s%s\r\n",
				atkName, defName, ev.Damage, critTail, offTag),
			atkSess, defSess)
	})

	// CombatMiss: symmetric to Hit but no damage line; both
	// participants and the room see the swing-and-miss beat.
	// Fumble (nat-1) replaces the regular miss line with a fumble
	// flavor line. When the side-effect dropped a weapon
	// (WeaponDroppedID != 0), the weapon name is interpolated so
	// every audience sees what hit the floor.
	eventbus.Subscribe(d.bus, func(ctx context.Context, ev combat.CombatMiss) {
		atkName := combatActorName(ctx, ev.Attacker, d.characters, d.mobs)
		defName := combatActorName(ctx, ev.Defender, d.characters, d.mobs)
		var atkSess, defSess *telnet.Session
		if ev.Attacker.Kind == combat.ActorKindCharacter {
			atkSess = cmd.LookupByCharacterID(d.sessions, ev.Attacker.ID)
		}
		if ev.Defender.Kind == combat.ActorKindCharacter {
			defSess = cmd.LookupByCharacterID(d.sessions, ev.Defender.ID)
		}
		if ev.Fumble {
			weaponName := ""
			if ev.WeaponDroppedID != 0 {
				if it, err := d.items.GetByID(ctx, ev.WeaponDroppedID); err == nil {
					weaponName = it.Name
				}
			}
			var selfMsg, defMsg, roomMsg string
			if weaponName != "" {
				selfMsg = fmt.Sprintf("{{You fumble your %s! It clatters to the ground.}}::yellow", weaponName)
				defMsg = fmt.Sprintf("{{%s fumbles their %s!}}::yellow", atkName, weaponName)
				roomMsg = fmt.Sprintf("{{%s fumbles their %s!}}::yellow\r\n", atkName, weaponName)
			} else {
				selfMsg = "{{You stumble badly.}}::yellow"
				defMsg = fmt.Sprintf("{{%s stumbles badly.}}::yellow", atkName)
				roomMsg = fmt.Sprintf("{{%s stumbles badly.}}::yellow\r\n", atkName)
			}
			if atkSess != nil {
				_ = atkSess.WriteAsync(selfMsg)
			}
			if defSess != nil {
				_ = defSess.WriteAsync(defMsg)
			}
			combatBroadcastSkip(ev.RoomID, roomMsg, atkSess, defSess)
			return
		}
		offTag := ""
		if ev.OffHand {
			offTag = " {{(off-hand)}}::gray"
		}
		if atkSess != nil {
			_ = atkSess.WriteAsync(fmt.Sprintf(variantMissSelfFormat(ev.Variant), defName) + offTag)
		}
		if defSess != nil {
			_ = defSess.WriteAsync(fmt.Sprintf("{{%s swings at you and misses.}}::gray", atkName))
		}
		combatBroadcastSkip(ev.RoomID,
			fmt.Sprintf("{{%s swings at %s and misses.}}::gray%s\r\n", atkName, defName, offTag),
			atkSess, defSess)
	})

	// CombatDeath for mob victims: "You killed X" to the killer (if
	// a player) and "X falls dead!" to room peers excluding the
	// killer. Player victims are handled by the CharacterDied
	// subscriber below — gate on Victim.Kind to avoid double-render.
	eventbus.Subscribe(d.bus, func(ctx context.Context, ev combat.CombatDeath) {
		if ev.Victim.Kind != combat.ActorKindMob {
			return
		}
		// Prefer the publish-time snapshot since the mob_instance row
		// is already gone by the time this fires; combatActorName
		// would fall back to "A creature" otherwise.
		victimName := ev.VictimName
		if victimName == "" {
			victimName = combatActorName(ctx, ev.Victim, d.characters, d.mobs)
		}
		var killerSess *telnet.Session
		if ev.Killer.Kind == combat.ActorKindCharacter {
			killerSess = cmd.LookupByCharacterID(d.sessions, ev.Killer.ID)
			if killerSess != nil {
				_ = killerSess.WriteAsync("{{You killed " + victimName + "!}}::green|bold")
			}
		}
		combatBroadcastExcept(ev.RoomID,
			"{{"+victimName+" falls dead!}}::red|bold\r\n", killerSess)
	})

	// CombatXPAwarded: private "You gain N XP." to the awardee, with
	// an optional XP-debt-drain suffix when DebtTaken > 0 so the
	// player understands why their gain looks smaller than the
	// gross share.
	eventbus.Subscribe(d.bus, func(ctx context.Context, ev combat.CombatXPAwarded) {
		if ev.Awardee.Kind != combat.ActorKindCharacter {
			return
		}
		sess := cmd.LookupByCharacterID(d.sessions, ev.Awardee.ID)
		if sess == nil {
			return
		}
		msg := fmt.Sprintf("{{You gain %d xp.}}::cyan", ev.Amount)
		if ev.DebtTaken > 0 {
			msg += fmt.Sprintf("  {{(%d xp went to clearing your xp debt)}}::gray", ev.DebtTaken)
		}
		_ = sess.WriteAsync(msg)
	})

	// Phase D §19 player-death subscribers. CharacterDied broadcasts
	// the death-room line + a "You die!" private message to the
	// dying player. CharacterRespawned then stamps the victim's
	// session room, renders the bound room, and broadcasts to peers
	// in the new room. The repo layer (handleCharacterDeath) already
	// persisted the room change via RecordRoom; the subscriber just
	// updates the live in-memory session.
	eventbus.Subscribe(d.bus, func(ctx context.Context, ev combat.CharacterDied) {
		victim := cmd.LookupByCharacterID(d.sessions, ev.Victim.ID)
		name := combatActorName(ctx, ev.Victim, d.characters, d.mobs)
		// Broadcast "X falls dead!" to peers — but skip the dying
		// player so they don't see it alongside "You die!" on the
		// next line. Their CurrentRoomID is still DeathRoomID at
		// this point (SetCurrentRoom runs in the respawn handler).
		combatBroadcastExcept(ev.DeathRoomID,
			"{{"+name+" falls dead!}}::red|bold\r\n", victim)
		if victim != nil {
			msg := "{{You die!}}::red|bold"
			if ev.XPDebtAdded > 0 {
				msg += fmt.Sprintf("  {{(+%d xp debt)}}::gray", ev.XPDebtAdded)
			}
			if err := victim.WriteAsync(msg); err != nil {
				slog.Debug("character died: victim notify failed",
					"char", ev.Victim.ID, "error", err)
			}
		}
	})
	eventbus.Subscribe(d.bus, func(ctx context.Context, ev combat.CharacterRespawned) {
		victim := cmd.LookupByCharacterID(d.sessions, ev.Character.ID)
		name := combatActorName(ctx, ev.Character, d.characters, d.mobs)
		if victim != nil {
			victim.SetCurrentRoom(ev.RoomID)
			// Detached context so the render isn't bound to whatever
			// resolveAction's ctx was; mirrors transferToCaller.
			if err := cmd.RenderRoom(context.Background(), victim, d.rooms, d.exits, d.items, d.mobs, d.clock); err != nil {
				slog.Debug("character respawned: render failed",
					"char", ev.Character.ID, "error", err)
			}
		}
		// Skip the respawned player on the arrival broadcast — they
		// just got their own bound-room render above; "X appears,
		// eyes hollow." stacked on top would be noise.
		combatBroadcastExcept(ev.RoomID,
			"{{"+name+" appears, eyes hollow.}}::cyan\r\n", victim)
	})

	// affects.Expired subscriber: emits one cfmt line per entry to
	// the owning session via WriteAsync (cross-session output rule).
	// Catalog-driven affects carry an authored MessageOnExpire string
	// (Phase E #25 slice 3); admin-applied affects (slice 1) leave it
	// empty and fall back to the generic fade line.
	eventbus.Subscribe(d.bus, func(_ context.Context, ev affects.Expired) {
		victim := cmd.LookupByCharacterID(d.sessions, ev.CharacterID)
		if victim == nil {
			return
		}
		for _, e := range ev.Entries {
			var msg string
			if e.Message != "" {
				msg = "{{" + e.Message + "}}::cyan\r\n"
			} else {
				msg = "{{Your " + e.Name + " fades.}}::cyan\r\n"
			}
			if err := victim.WriteAsync(msg); err != nil {
				slog.Debug("affects: expired notify failed",
					"char", ev.CharacterID, "name", e.Name, "error", err)
			}
		}
	})

	// affects.TickDamaged subscriber: per-tick HP delta lines from
	// poison/bleed/regen affects. Phase E #25 slice 2.
	eventbus.Subscribe(d.bus, func(_ context.Context, ev affects.TickDamaged) {
		victim := cmd.LookupByCharacterID(d.sessions, ev.CharacterID)
		if victim == nil {
			return
		}
		for _, te := range ev.Events {
			if te.Delta == 0 {
				continue
			}
			var msg string
			if te.Delta < 0 {
				msg = fmt.Sprintf("{{You suffer %d damage from %s.}}::red\r\n", -te.Delta, te.Name)
			} else {
				msg = fmt.Sprintf("{{You recover %d hp from %s.}}::green\r\n", te.Delta, te.Name)
			}
			if err := victim.WriteAsync(msg); err != nil {
				slog.Debug("affects: tick notify failed",
					"char", ev.CharacterID, "name", te.Name, "error", err)
			}
		}
	})

	// ScriptDamageDealt / ScriptHealingApplied subscribers (Phase F
	// #32 slice 5a). Renders a default narration line to the target's
	// session if they have one. Scripts that want custom flavor call
	// say / emote themselves — these defaults are the fallback so
	// silent HP changes from Lua aren't invisible to the player.
	// Death narration on lethal hits already flows from CharacterDied
	// / CombatDeath, so we suppress the default line when Lethal is
	// true to avoid double-narration.
	eventbus.Subscribe(d.bus, func(_ context.Context, ev combat.ScriptDamageDealt) {
		if ev.Lethal || ev.Target.Kind != combat.ActorKindCharacter {
			return
		}
		victim := cmd.LookupByCharacterID(d.sessions, ev.Target.ID)
		if victim == nil {
			return
		}
		src := ev.Source
		if src == "" {
			src = "an unseen force"
		}
		msg := fmt.Sprintf("{{You suffer %d damage from %s.}}::red\r\n", ev.Amount, defangScriptSource(src))
		if err := victim.WriteAsync(msg); err != nil {
			slog.Debug("script_damage: notify failed",
				"char", ev.Target.ID, "source", ev.Source, "error", err)
		}
	})
	eventbus.Subscribe(d.bus, func(_ context.Context, ev combat.ScriptHealingApplied) {
		if ev.Target.Kind != combat.ActorKindCharacter {
			return
		}
		victim := cmd.LookupByCharacterID(d.sessions, ev.Target.ID)
		if victim == nil {
			return
		}
		var msg string
		if ev.Amount == 0 {
			msg = "{{A warm light touches you, but you are already whole.}}::green\r\n"
		} else {
			msg = fmt.Sprintf("{{A warm light suffuses you; you recover %d hp.}}::green\r\n", ev.Amount)
		}
		if err := victim.WriteAsync(msg); err != nil {
			slog.Debug("script_healing: notify failed",
				"char", ev.Target.ID, "error", err)
		}
	})
}
