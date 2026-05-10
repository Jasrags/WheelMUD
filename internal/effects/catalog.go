package effects

import (
	"fmt"
	"io/fs"

	"github.com/Jasrags/WheelMUD/internal/affects"
	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"gopkg.in/yaml.v3"
)

// Catalog is the in-memory effect index keyed by string ID. Built
// once at boot via Load + Validate.
type Catalog struct {
	byID   map[string]Effect
	byHash map[int32]string // chargen.HashID(id) → id, for ConsumableStats.EffectID resolution
}

// Get returns the effect with the given catalog ID and a presence bool.
func (c *Catalog) Get(id string) (Effect, bool) {
	if c == nil {
		return Effect{}, false
	}
	e, ok := c.byID[id]
	return e, ok
}

// IDForHash reverses chargen.HashID — given the int32 effect ID
// stored on a ConsumableStats row, returns the string id (and a
// presence bool) so callers can resolve the catalog entry.
func (c *Catalog) IDForHash(hash int32) (string, bool) {
	if c == nil {
		return "", false
	}
	id, ok := c.byHash[hash]
	return id, ok
}

// IDs returns every catalog id, primarily for boot-time validation
// against world content.
func (c *Catalog) IDs() []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.byID))
	for id := range c.byID {
		out = append(out, id)
	}
	return out
}

// Load parses every *.yaml file under root, validates the result, and
// returns a populated Catalog. Errors are wrapped with the offending
// filename so a typo fails the boot loudly.
func Load(root fs.FS) (*Catalog, error) {
	cat := &Catalog{
		byID:   map[string]Effect{},
		byHash: map[int32]string{},
	}
	entries, err := fs.ReadDir(root, ".")
	if err != nil {
		return nil, fmt.Errorf("effects: read root: %w", err)
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !hasYAMLSuffix(name) {
			continue
		}
		data, err := fs.ReadFile(root, name)
		if err != nil {
			return nil, fmt.Errorf("effects: read %s: %w", name, err)
		}
		var batch struct {
			Effects []Effect `yaml:"effects"`
		}
		if err := yaml.Unmarshal(data, &batch); err != nil {
			return nil, fmt.Errorf("effects: parse %s: %w", name, err)
		}
		for _, e := range batch.Effects {
			if _, dup := cat.byID[e.ID]; dup {
				return nil, fmt.Errorf("effects: duplicate id %q (in %s)", e.ID, name)
			}
			cat.byID[e.ID] = e
			h := chargen.HashID(e.ID)
			if existing, dup := cat.byHash[h]; dup {
				return nil, fmt.Errorf("effects: hash collision: %q and %q both hash to %d", e.ID, existing, h)
			}
			cat.byHash[h] = e.ID
		}
	}
	if err := cat.Validate(); err != nil {
		return nil, err
	}
	return cat, nil
}

// Validate enforces the well-formedness rules from the slice-2 plan:
//   - non-empty Name + ID
//   - DurationTicks > 0
//   - StatMod fields are in affects.AllowedStatModFields
//   - TickDamage != 0 requires TickEffect != "" (and vice versa)
//   - ConditionMask bits are all in the defined creature.Condition range
func (c *Catalog) Validate() error {
	allowed := map[string]bool{}
	for _, f := range affects.AllowedStatModFields {
		allowed[f] = true
	}
	for id, e := range c.byID {
		if e.ID == "" {
			return fmt.Errorf("effects: entry with blank id (Name=%q)", e.Name)
		}
		if e.Name == "" {
			return fmt.Errorf("effects: %s has blank Name", id)
		}
		if e.DurationTicks <= 0 {
			return fmt.Errorf("effects: %s has non-positive DurationTicks (%d)", id, e.DurationTicks)
		}
		for _, m := range e.Modifiers {
			if !allowed[m.Field] {
				return fmt.Errorf("effects: %s references unknown StatMod field %q", id, m.Field)
			}
		}
		if (e.TickDamage != 0) != (e.TickEffect != "") {
			return fmt.Errorf("effects: %s: TickDamage and TickEffect must both be set or both empty (got TickDamage=%d, TickEffect=%q)", id, e.TickDamage, e.TickEffect)
		}
		if e.ConditionMask != 0 && (e.ConditionMask&^validConditionMask) != 0 {
			return fmt.Errorf("effects: %s: ConditionMask has bits outside the defined enum (%032b)", id, e.ConditionMask)
		}
	}
	return nil
}

// validConditionMask is every defined creature.Cond* OR'd together.
// New creature.Condition bits MUST be added here — Validate uses this
// to reject typo'd YAML mask values.
const validConditionMask creature.Condition = creature.CondAbilityDamaged |
	creature.CondAbilityDrained |
	creature.CondBlinded |
	creature.CondChecked |
	creature.CondCowering |
	creature.CondDazed |
	creature.CondDeafened |
	creature.CondDisabled |
	creature.CondDying |
	creature.CondEntangled |
	creature.CondExhausted |
	creature.CondFatigued |
	creature.CondFlatFooted |
	creature.CondFrightened |
	creature.CondGrappled |
	creature.CondHeld |
	creature.CondHelpless |
	creature.CondPanicked |
	creature.CondParalyzed |
	creature.CondPinned |
	creature.CondProne |
	creature.CondShaken |
	creature.CondStable |
	creature.CondStaggered |
	creature.CondStunned |
	creature.CondUnconscious

func hasYAMLSuffix(name string) bool {
	return len(name) >= 5 && name[len(name)-5:] == ".yaml" ||
		len(name) >= 4 && name[len(name)-4:] == ".yml"
}
