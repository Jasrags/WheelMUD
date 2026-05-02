package repo

import (
	"encoding/json"
	"fmt"
)

// StatsForType returns a fresh empty stats struct of the kind that
// matches itemType. Used when decoding stats_json so json.Unmarshal
// fills the right concrete type. Returns nil for the untyped tier
// (trash / clothing / food / trade_good), which carries no stats
// sub-record. Exported so the YAML loader can build the same
// per-type structs without reaching for an internal helper.
func StatsForType(t ItemType) ItemStats {
	switch t {
	case ItemTypeWeapon:
		return &WeaponStats{}
	case ItemTypeArmor:
		return &ArmorStats{}
	case ItemTypeShield:
		return &ShieldStats{}
	case ItemTypeContainer:
		return &ContainerStats{}
	case ItemTypeConsumable:
		return &ConsumableStats{}
	case ItemTypeLight:
		return &LightStats{}
	case ItemTypeKey:
		return &KeyStats{}
	case ItemTypeTool:
		return &ToolStats{}
	}
	return nil
}

// statsTypeMatches reports whether the concrete Stats kind agrees
// with t. The untyped tier (trash / clothing / food / trade_good)
// requires Stats to be nil. Typed items require a non-nil pointer
// of the matching concrete type — a typed item with nil Stats is
// rejected at Create time so a misuse can't silently round-trip
// as a zero-valued struct.
func statsTypeMatches(t ItemType, s ItemStats) bool {
	switch t {
	case ItemTypeWeapon:
		_, ok := s.(*WeaponStats)
		return ok
	case ItemTypeArmor:
		_, ok := s.(*ArmorStats)
		return ok
	case ItemTypeShield:
		_, ok := s.(*ShieldStats)
		return ok
	case ItemTypeContainer:
		_, ok := s.(*ContainerStats)
		return ok
	case ItemTypeConsumable:
		_, ok := s.(*ConsumableStats)
		return ok
	case ItemTypeLight:
		_, ok := s.(*LightStats)
		return ok
	case ItemTypeKey:
		_, ok := s.(*KeyStats)
		return ok
	case ItemTypeTool:
		_, ok := s.(*ToolStats)
		return ok
	case ItemTypeClothing, ItemTypeFood, ItemTypeTradeGood, ItemTypeTrash:
		return s == nil
	}
	return false
}

// encodeStats serializes Stats to a JSON string for the stats_json
// column. Nil stats become "{}" so the column NOT NULL contract holds.
func encodeStats(s ItemStats) (string, error) {
	if s == nil {
		return "{}", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("encode item stats: %w", err)
	}
	return string(b), nil
}

// decodeStats parses raw stats_json into the concrete struct that
// matches t. The items.stats_json column is NOT NULL DEFAULT '{}',
// so empty input is unexpected and treated as data corruption — we
// surface it as an error instead of silently returning a zero
// struct. Trash / clothing / food / trade_good return nil regardless
// of input (those tiers have no stats sub-record).
func decodeStats(t ItemType, raw string) (ItemStats, error) {
	stats := StatsForType(t)
	if stats == nil {
		return nil, nil
	}
	if raw == "" {
		return nil, fmt.Errorf("decode %s stats: empty stats_json (corrupt row?)", t)
	}
	if err := json.Unmarshal([]byte(raw), stats); err != nil {
		return nil, fmt.Errorf("decode %s stats: %w", t, err)
	}
	return stats, nil
}
