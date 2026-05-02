package world

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// itemFlagByName resolves YAML flag strings to repo bits. Builder-
// facing names are kebab/snake-tolerant: `bind_on_pickup`,
// `bind-on-pickup`, and `bindOnPickup` would all map identically if
// added later — for now we accept the canonical snake_case form
// shown in CLAUDE.md / ROADMAP.md.
var itemFlagByName = map[string]repo.ItemFlags{
	"notake":          repo.FlagNoTake,
	"nodrop":          repo.FlagNoDrop,
	"nosell":          repo.FlagNoSell,
	"bind_on_pickup":  repo.FlagBindOnPickup,
	"magic":           repo.FlagMagic,
	"glow":            repo.FlagGlow,
	"hum":             repo.FlagHum,
	"trade_good":      repo.FlagTradeGood,
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
	raw, err := json.Marshal(it.Stats)
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

