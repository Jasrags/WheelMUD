// Package effects authors timed buff/debuff entries that producers
// (consumables, weave casts, on-hit) instantiate as creature.Affect
// values. The catalog is content, not state — it's loaded once at
// boot from YAML and never touches the DB. Producers translate a
// catalog Effect into a creature.Affect at apply time.
//
// Phase E #25 slice 2.
package effects

import "github.com/Jasrags/WheelMUD/internal/creature"

// Effect is one authored buff/debuff. Mirrors creature.Affect but
// with content-time fields (ID, on-apply / on-expire messages) the
// producer side wants to keep separate from the runtime row.
type Effect struct {
	ID            string                 `yaml:"id"`
	Name          string                 `yaml:"name"`
	DurationTicks int32                  `yaml:"duration_ticks"`
	Modifiers     []creature.StatMod     `yaml:"modifiers"`
	ConditionMask creature.Condition     `yaml:"condition_mask"`
	TickEffect    string                 `yaml:"tick_effect"`
	TickDamage    int16                  `yaml:"tick_damage"`
	MessageOnApply  string               `yaml:"message_on_apply"`
	MessageOnExpire string               `yaml:"message_on_expire"`
}

// ToAffect builds a runtime creature.Affect from this catalog entry,
// stamping the supplied Source. Pure copy; the catalog row is not
// mutated.
func (e Effect) ToAffect(source int64) creature.Affect {
	mods := append([]creature.StatMod(nil), e.Modifiers...)
	return creature.Affect{
		Source:        source,
		Name:          e.Name,
		Modifiers:     mods,
		DurationTicks: e.DurationTicks,
		TickEffect:    e.TickEffect,
		ConditionMask: e.ConditionMask,
		TickDamage:    e.TickDamage,
		ExpireMessage: e.MessageOnExpire,
	}
}
