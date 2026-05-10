package world

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// itemFlagByName resolves YAML flag strings to repo bits. Builder-
// facing names are kebab/snake-tolerant: `bind_on_pickup`,
// `bind-on-pickup`, and `bindOnPickup` would all map identically if
// added later — for now we accept the canonical snake_case form
// shown in CLAUDE.md / ROADMAP.md.
var itemFlagByName = map[string]repo.ItemFlags{
	"notake":         repo.FlagNoTake,
	"nodrop":         repo.FlagNoDrop,
	"nosell":         repo.FlagNoSell,
	"bind_on_pickup": repo.FlagBindOnPickup,
	"magic":          repo.FlagMagic,
	"glow":           repo.FlagGlow,
	"hum":            repo.FlagHum,
	"trade_good":     repo.FlagTradeGood,
}

// decodeItemFlags packs the YAML flag-name list into the repo bitset.
// Validation already rejects unknown names; this is the happy-path
// pack only.
func decodeItemFlags(names []string) repo.ItemFlags {
	var f repo.ItemFlags
	for _, n := range names {
		f |= itemFlagByName[n]
	}
	return f
}

// decodeItemValue parses the YAML `value:` string into a copper-penny
// Amount. Empty string means "free / no listed value" and decodes to
// zero. Anything else goes through currency.Parse, which already
// understands "5 mk", "2 gc 1 sp", etc.
func decodeItemValue(s string) (currency.Amount, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	amt, err := currency.Parse(s)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q: %w", s, err)
	}
	return amt, nil
}

// convertItemStats turns the loose YAML stats map into the typed
// stats struct that matches the item's type. Round-trips through
// JSON so each per-type schema gets the standard json tag handling
// (`damage_type` instead of `DamageType`, etc.). Trash / clothing /
// food / trade_good return nil because they have no stats sub-record.
//
// An empty Stats map for a typed item (e.g. type=weapon with no
// stats:) yields a zero-valued struct, not an error — builders can
// stub items in early authoring without filling in every column.
func convertItemStats(it Item) (repo.ItemStats, error) {
	t := repo.ItemType(it.Type)
	if t == "" {
		t = repo.ItemTypeTrash
	}
	target := repo.StatsForType(t)
	if target == nil {
		// Untyped (trash / clothing / food / trade_good): stats blob
		// is meaningless. Reject it so a typo doesn't pass silently.
		if len(it.Stats) > 0 {
			return nil, fmt.Errorf("type %q does not accept a stats block", t)
		}
		return nil, nil
	}
	if len(it.Stats) == 0 {
		return target, nil
	}
	stats := it.Stats
	if t == repo.ItemTypeConsumable {
		// Allow builders to author the effect by string id via
		// `effect_id_string`; translate to the int32 hash key the
		// runtime expects. Mutually exclusive with explicit `effect_id`.
		var err error
		stats, err = translateConsumableEffectID(stats)
		if err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(stats)
	if err != nil {
		return nil, fmt.Errorf("encode stats: %w", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return nil, fmt.Errorf("decode %s stats: %w", t, err)
	}
	return target, nil
}

// translateConsumableEffectID rewrites a consumable stats map so a
// builder-friendly `effect_id_string` key resolves into the runtime
// `effect_id` int32 (via chargen.HashID). Unknown effect IDs are NOT
// rejected here — the loader's validate step cross-checks against the
// effect catalog so the failure surfaces with the offending YAML
// filename. Returns a new map; the input is not mutated.
//
// Specifying both `effect_id` and `effect_id_string` in the same
// stats block is an authoring error and returns a non-nil error so
// the loader fails the boot loudly instead of silently discarding the
// numeric value.
func translateConsumableEffectID(in map[string]any) (map[string]any, error) {
	if in == nil {
		return nil, nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	raw, ok := out["effect_id_string"]
	if !ok {
		return out, nil
	}
	if _, both := out["effect_id"]; both {
		return nil, fmt.Errorf("cannot set both effect_id and effect_id_string on a consumable")
	}
	delete(out, "effect_id_string")
	if id, ok := raw.(string); ok && id != "" {
		out["effect_id"] = chargen.HashID(id)
	}
	return out, nil
}
