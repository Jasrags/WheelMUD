package chargen

// Starting-equipment template loader. Mirrors the world item taxonomy
// (internal/world/item_taxonomy.go) but lives in chargen because the
// templates are content the chargen flow spawns into a brand-new
// character — they don't live in any room and never go through the
// world loader.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

const fileItems = "items.yaml"

// itemFlagByName mirrors internal/world/item_taxonomy.go's table. Kept
// local so chargen doesn't import internal/world (which would invite a
// cycle once world grows chargen-aware code).
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

// resolveItemTemplate fills in the parsed* fields from the YAML entry's
// raw value / flags / stats. Returns a list of validation messages
// instead of an error so the catalog validate() can aggregate them
// alongside background / class issues.
func resolveItemTemplate(it *ItemTemplate) []string {
	var errs []string

	if it.ID == "" {
		errs = append(errs, "item: missing id")
		return errs
	}
	if it.Name == "" {
		errs = append(errs, fmt.Sprintf("item %q: missing name", it.ID))
	}
	if !it.Type.IsValid() {
		errs = append(errs, fmt.Sprintf("item %q: invalid type %q", it.ID, it.Type))
		return errs
	}
	// Quality defaults to normal when blank.
	if it.Quality == "" {
		it.Quality = repo.QualityNormal
	}
	if !it.Quality.IsValid() {
		errs = append(errs, fmt.Sprintf("item %q: invalid quality %q", it.ID, it.Quality))
	}

	// Currency — empty is "free / no listed value".
	v := strings.TrimSpace(it.Value)
	if v != "" {
		amt, err := currency.Parse(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("item %q: invalid value %q: %v", it.ID, v, err))
		} else {
			it.parsedValue = amt
		}
	}

	// Flags — unknown names fail.
	var flags repo.ItemFlags
	for _, name := range it.Flags {
		bit, ok := itemFlagByName[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("item %q: unknown flag %q", it.ID, name))
			continue
		}
		flags |= bit
	}
	it.parsedFlags = flags

	// Stats — type-discriminated decode through a JSON round trip so
	// each per-type schema gets its existing json tag handling.
	target := repo.StatsForType(it.Type)
	if target == nil {
		// Untyped tier (clothing / food / trade_good / trash) — reject
		// a stats: block so a typo doesn't pass silently.
		if len(it.Stats) > 0 {
			errs = append(errs, fmt.Sprintf("item %q: type %q does not accept a stats block", it.ID, it.Type))
		}
		it.parsedStats = nil
	} else {
		if len(it.Stats) == 0 {
			it.parsedStats = target // zero-value typed struct
		} else {
			raw, err := json.Marshal(it.Stats)
			if err != nil {
				errs = append(errs, fmt.Sprintf("item %q: encode stats: %v", it.ID, err))
			} else {
				dec := json.NewDecoder(strings.NewReader(string(raw)))
				dec.DisallowUnknownFields()
				if err := dec.Decode(target); err != nil {
					errs = append(errs, fmt.Sprintf("item %q: decode %s stats: %v", it.ID, it.Type, err))
				} else {
					it.parsedStats = target
				}
			}
		}
	}
	return errs
}

// validateItems runs resolveItemTemplate over every template, then
// cross-checks every Background.EquipmentOptions[].Items reference
// against the templates map. Returns a sorted slice of errors so the
// catalog validate() can splice them into its message.
func (c *Catalog) validateItems() []string {
	var errs []string
	if len(c.items) == 0 {
		errs = append(errs, "items: empty catalog")
		return errs
	}
	// Resolve typed fields on every entry.
	ids := make([]string, 0, len(c.items))
	for id := range c.items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		errs = append(errs, resolveItemTemplate(c.items[id])...)
	}
	// Cross-check background equipment options.
	for _, bg := range c.backgrounds {
		for _, opt := range bg.EquipmentOptions {
			for _, ref := range opt.Items {
				if _, ok := c.items[ref]; !ok {
					errs = append(errs, fmt.Sprintf("background %q: equipment_options reference unknown item %q", bg.ID, ref))
				}
			}
		}
	}
	return errs
}

// Item returns the entry by id, ok=false on miss.
func (c *Catalog) Item(id string) (*ItemTemplate, bool) {
	v, ok := c.items[id]
	return v, ok
}

// Items returns all entries in declaration order.
func (c *Catalog) Items() []*ItemTemplate {
	out := make([]*ItemTemplate, 0, len(c.itemOrder))
	for _, id := range c.itemOrder {
		out = append(out, c.items[id])
	}
	return out
}
