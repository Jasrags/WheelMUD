package combat

// Phase F #32 slice 5a — external damage / healing entry points used
// by Lua scripts (and any other out-of-combat source) to mutate a
// target's HP without entering the attack-resolution hot path.
//
// Design notes:
//
//   - Damage is raw — no DR / resists, no crit multiplier, no
//     to-hit roll. The script author chose the amount and is
//     responsible for whatever flavor the source implies. Authors
//     who want elemental DR / resist should layer apply_affect
//     with a DoT effect instead.
//   - No threat-table mutation. Matches the existing rule that
//     non-combat damage (DoT ticks via HandleAffectDeath) doesn't
//     generate threat. If "mob fires Lua to taunt + damage during
//     combat" becomes a content idiom we'll revisit.
//   - No feat / on-hit affect hook fan-out. Those ride the attack
//     pipeline deliberately; script damage is outside it.
//   - Both methods follow the same lock posture as
//     HandleAffectDeath: all repo IO runs outside m.mu, and the
//     death pipeline (when triggered) acquires the lock internally
//     in handleCharacterDeath / handleMobDeath.

import (
	"context"
	"fmt"
)

// ApplyDamageExternal subtracts amount from target's HPCurrent
// (clamped at zero), persists the new value, publishes a
// ScriptDamageDealt event, and routes lethal damage through the
// matching death handler. Returns an error only for argument /
// repo failures; a successful zero-effect call (already-dead
// target) returns nil.
//
// killer is the ActorRef to credit on death events — pass the
// firing actor when one exists (e.g. a character running a script
// effect), or ActorRef{} for environmental / anonymous damage.
// Slice 5a content authors using the Lua `deal_damage` binding get
// an anonymous killer because the binding closure can't reliably
// pick one (a script fired from an `on_say` trigger could mean the
// speaker, the room, or "nobody"). Future slices may thread an
// authored attribution through Source.
func (m *Manager) ApplyDamageExternal(ctx context.Context, killer, target ActorRef, amount int32, source string) error {
	if amount <= 0 {
		return fmt.Errorf("combat: ApplyDamageExternal: amount must be > 0 (got %d)", amount)
	}
	if target.ID == 0 || target.Kind == ActorKindUnknown {
		return fmt.Errorf("combat: ApplyDamageExternal: invalid target %v", target)
	}

	switch target.Kind {
	case ActorKindCharacter:
		if m.chars == nil {
			return fmt.Errorf("combat: ApplyDamageExternal: no character repo wired")
		}
		ch, err := m.chars.GetByID(ctx, target.ID)
		if err != nil {
			return fmt.Errorf("combat: ApplyDamageExternal: char lookup: %w", err)
		}
		if ch.Core.HPCurrent <= 0 {
			// Already-dead target: no-op, no event, no error.
			return nil
		}
		newHP := ch.Core.HPCurrent - amount
		if newHP < 0 {
			newHP = 0
		}
		if err := m.chars.RecordCore(ctx, target.ID, newHP, ch.Core.Subdual, ch.Core.Conditions, ch.Core.Position); err != nil {
			return fmt.Errorf("combat: ApplyDamageExternal: char hp write-back: %w", err)
		}
		roomID := ch.CurrentRoomID
		lethal := newHP == 0
		if m.bus != nil {
			m.bus.Publish(ctx, ScriptDamageDealt{
				RoomID: roomID,
				Target: target,
				Amount: amount,
				Source: source,
				Lethal: lethal,
			})
		}
		if lethal {
			// Mirror the affect-death routing: characterDeath handles
			// XP debt, respawn vitals, room move, and CharacterDied /
			// CharacterRespawned publishes. handleCharacterDeath is
			// safe to call even when no Fight covers the death room.
			m.handleCharacterDeath(ctx, killer, target)
		}
		return nil

	case ActorKindMob:
		if m.mobs == nil {
			return fmt.Errorf("combat: ApplyDamageExternal: no mob repo wired")
		}
		mob, err := m.mobs.GetByID(ctx, target.ID)
		if err != nil {
			return fmt.Errorf("combat: ApplyDamageExternal: mob lookup: %w", err)
		}
		if mob.Core.HPCurrent <= 0 {
			return nil
		}
		newHP := mob.Core.HPCurrent - amount
		if newHP < 0 {
			newHP = 0
		}
		if err := m.mobs.UpdateLive(ctx, target.ID, newHP, mob.Core.Subdual, mob.Core.Conditions, mob.Core.Position); err != nil {
			return fmt.Errorf("combat: ApplyDamageExternal: mob hp write-back: %w", err)
		}
		roomID := mob.Core.CurrentRoomID
		lethal := newHP == 0
		if m.bus != nil {
			m.bus.Publish(ctx, ScriptDamageDealt{
				RoomID: roomID,
				Target: target,
				Amount: amount,
				Source: source,
				Lethal: lethal,
			})
		}
		if lethal {
			m.handleMobDeath(ctx, killer, target)
		}
		return nil
	}
	return fmt.Errorf("combat: ApplyDamageExternal: unknown actor kind %v", target.Kind)
}

// ApplyHealing adds amount to target's HPCurrent (clamped at HPMax),
// persists the new value, and publishes ScriptHealingApplied. Dead
// targets (HPCurrent == 0) are a no-op — raising the dead requires
// the respawn pipeline, not a heal. Returns an error only for
// argument / repo failures.
//
// When the target is already at full HP the persistence step is
// skipped and the event publishes with Amount == 0 so subscribers
// can still render a "you feel a warm light, but you are unhurt"
// flavor line if they want.
func (m *Manager) ApplyHealing(ctx context.Context, target ActorRef, amount int32) error {
	if amount <= 0 {
		return fmt.Errorf("combat: ApplyHealing: amount must be > 0 (got %d)", amount)
	}
	if target.ID == 0 || target.Kind == ActorKindUnknown {
		return fmt.Errorf("combat: ApplyHealing: invalid target %v", target)
	}

	switch target.Kind {
	case ActorKindCharacter:
		if m.chars == nil {
			return fmt.Errorf("combat: ApplyHealing: no character repo wired")
		}
		ch, err := m.chars.GetByID(ctx, target.ID)
		if err != nil {
			return fmt.Errorf("combat: ApplyHealing: char lookup: %w", err)
		}
		if ch.Core.HPCurrent <= 0 {
			// Dead target — heal can't raise. Caller's responsibility
			// to recognize this; no event, no error.
			return nil
		}
		delta := amount
		newHP := ch.Core.HPCurrent + amount
		if newHP > ch.Core.HPMax {
			newHP = ch.Core.HPMax
			delta = newHP - ch.Core.HPCurrent
		}
		if delta > 0 {
			if err := m.chars.RecordCore(ctx, target.ID, newHP, ch.Core.Subdual, ch.Core.Conditions, ch.Core.Position); err != nil {
				return fmt.Errorf("combat: ApplyHealing: char hp write-back: %w", err)
			}
		}
		if m.bus != nil {
			m.bus.Publish(ctx, ScriptHealingApplied{
				RoomID: ch.CurrentRoomID,
				Target: target,
				Amount: delta,
			})
		}
		return nil

	case ActorKindMob:
		if m.mobs == nil {
			return fmt.Errorf("combat: ApplyHealing: no mob repo wired")
		}
		mob, err := m.mobs.GetByID(ctx, target.ID)
		if err != nil {
			return fmt.Errorf("combat: ApplyHealing: mob lookup: %w", err)
		}
		if mob.Core.HPCurrent <= 0 {
			return nil
		}
		delta := amount
		newHP := mob.Core.HPCurrent + amount
		if newHP > mob.Core.HPMax {
			newHP = mob.Core.HPMax
			delta = newHP - mob.Core.HPCurrent
		}
		if delta > 0 {
			if err := m.mobs.UpdateLive(ctx, target.ID, newHP, mob.Core.Subdual, mob.Core.Conditions, mob.Core.Position); err != nil {
				return fmt.Errorf("combat: ApplyHealing: mob hp write-back: %w", err)
			}
		}
		if m.bus != nil {
			m.bus.Publish(ctx, ScriptHealingApplied{
				RoomID: mob.Core.CurrentRoomID,
				Target: target,
				Amount: delta,
			})
		}
		return nil
	}
	return fmt.Errorf("combat: ApplyHealing: unknown actor kind %v", target.Kind)
}
