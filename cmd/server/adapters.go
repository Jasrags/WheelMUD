package main

import (
	"context"

	"github.com/Jasrags/WheelMUD/internal/affects"
	"github.com/Jasrags/WheelMUD/internal/channeling"
	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/flow"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// variantHitSelfFormat returns the complete Sprintf format string
// for a CombatHit subscriber's first-person echo line. cfmt wrap is
// baked in so the returned value can be passed as a literal format
// to Sprintf and `go vet` statically validates the verb/arg counts.
// Verbs: %s (defender name), %d (damage), %s (crit tail). Phase L #61.
func variantHitSelfFormat(v combat.AttackVariant) string {
	switch v {
	case combat.VariantPower:
		return "{{You lunge with a power strike at %s for %d damage.}}::cyan%s"
	case combat.VariantQuick:
		return "{{You flick a quick jab at %s for %d damage.}}::cyan%s"
	default:
		return "{{You hit %s for %d damage.}}::cyan%s"
	}
}

// variantMissSelfFormat is the miss-side mirror of
// variantHitSelfFormat. Verb: %s (defender name).
func variantMissSelfFormat(v combat.AttackVariant) string {
	switch v {
	case combat.VariantPower:
		return "{{You lunge wide of %s and miss.}}::gray"
	case combat.VariantQuick:
		return "{{You flick at %s and miss.}}::gray"
	default:
		return "{{You swing at %s and miss.}}::gray"
	}
}

// combatActorName resolves an ActorRef to a display name for combat
// broadcasts. Best-effort: a despawned/logged-out participant falls
// back to "Someone" / "A creature" so the broadcast still reads.
// Routes through display.Defang so a builder-authored mob name (or
// future loosened character-name policy) can't smuggle cfmt markers
// or control bytes into a {{...}} broadcast.
func combatActorName(ctx context.Context, ref combat.ActorRef, chars repo.CharacterRepo, mobs repo.MobInstanceRepo) string {
	switch ref.Kind {
	case combat.ActorKindCharacter:
		ch, err := chars.GetByID(ctx, ref.ID)
		if err == nil {
			return display.Defang(ch.Core.Name, "Someone")
		}
		return "Someone"
	case combat.ActorKindMob:
		mob, err := mobs.GetByID(ctx, ref.ID)
		if err == nil {
			return display.Defang(mob.Core.Name, "A creature")
		}
		return "A creature"
	}
	return "Someone"
}

// characterAffectsLoader adapts repo.CharacterRepo to the slim
// affects.CharLoader interface. Only the Affects field of the loaded
// row is exposed to the ticker — everything else stays in the repo.
type characterAffectsLoader struct {
	chars repo.CharacterRepo
}

func (a characterAffectsLoader) GetByID(ctx context.Context, id int64) (affects.Character, error) {
	ch, err := a.chars.GetByID(ctx, id)
	if err != nil {
		return affects.Character{}, err
	}
	return affects.Character{
		Affects:   ch.Core.Affects,
		HPCurrent: ch.Core.HPCurrent,
		HPMax:     ch.Core.HPMax,
		Subdual:   ch.Core.Subdual,
		Condition: ch.Core.Conditions,
		Position:  ch.Core.Position,
	}, nil
}

func (a characterAffectsLoader) RecordAffects(ctx context.Context, id int64, list []creature.Affect) error {
	return a.chars.RecordAffects(ctx, id, list)
}

func (a characterAffectsLoader) RecordHP(ctx context.Context, id int64, hp, subdual int32) error {
	return a.chars.RecordHP(ctx, id, hp, subdual)
}

// characterChannelingLoader adapts repo.CharacterRepo to the slim
// channeling.CharLoader interface. Phase E #27 — only the Channeling
// pointer is exposed to the ticker.
type characterChannelingLoader struct {
	chars repo.CharacterRepo
}

func (a characterChannelingLoader) GetByID(ctx context.Context, id int64) (channeling.Character, error) {
	ch, err := a.chars.GetByID(ctx, id)
	if err != nil {
		return channeling.Character{}, err
	}
	return channeling.Character{Channeling: ch.Channeling}, nil
}

func (a characterChannelingLoader) RecordChanneling(ctx context.Context, id int64, c *creature.Channeling) error {
	return a.chars.RecordChanneling(ctx, id, c)
}

// eventbusAdapter wraps *eventbus.Bus to satisfy affects.EventPublisher.
// affects.EventPublisher takes an `any` so the affects package stays
// free of the eventbus import; eventbus.Event is interface{}, so any
// payload (including the typed affects.Expired struct) round-trips
// through reflection inside Publish.
type eventbusAdapter struct {
	bus *eventbus.Bus
}

func (a eventbusAdapter) Publish(ctx context.Context, ev any) {
	if a.bus == nil {
		return
	}
	a.bus.Publish(ctx, ev)
}

// flowRepoPersister bridges repo.FlowStateRepo to flow.Persister so
// the engine never sees the repo type. Save translates the engine
// struct into a repo struct; Delete is a thin passthrough. §O.2.
type flowRepoPersister struct {
	repo repo.FlowStateRepo
}

func (p flowRepoPersister) Save(ctx context.Context, s *flow.State) error {
	return p.repo.Save(ctx, repo.FlowState{
		AccountID:   s.AccountID,
		FlowID:      s.FlowID,
		CurrentStep: string(s.Current),
		Values:      s.Values,
		StartedAt:   s.StartedAt,
		UpdatedAt:   s.UpdatedAt,
	})
}

func (p flowRepoPersister) Delete(ctx context.Context, accountID int64, flowID string) error {
	return p.repo.Delete(ctx, accountID, flowID)
}
