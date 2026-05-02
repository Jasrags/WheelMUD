package repo

import (
	"context"
	"errors"
	"time"

	"github.com/Jasrags/WheelMUD/internal/currency"
)

// ItemType is the closed enum tracked by the items.type column. Every
// non-trash entry pairs with a stats sub-record decoded from
// stats_json. New types require a migration (the CHECK constraint
// rejects unknown values) and a corresponding stats struct here.
type ItemType string

const (
	ItemTypeWeapon     ItemType = "weapon"
	ItemTypeArmor      ItemType = "armor"
	ItemTypeShield     ItemType = "shield"
	ItemTypeContainer  ItemType = "container"
	ItemTypeConsumable ItemType = "consumable"
	ItemTypeLight      ItemType = "light"
	ItemTypeKey        ItemType = "key"
	ItemTypeTool       ItemType = "tool"
	ItemTypeClothing   ItemType = "clothing"
	ItemTypeFood       ItemType = "food"
	ItemTypeTradeGood  ItemType = "trade_good"
	ItemTypeTrash      ItemType = "trash"
)

// IsValid reports whether the type is one of the recognized enum
// values. Used by the YAML loader to fail fast on typos before the
// row hits the DB CHECK constraint.
func (t ItemType) IsValid() bool {
	switch t {
	case ItemTypeWeapon, ItemTypeArmor, ItemTypeShield, ItemTypeContainer,
		ItemTypeConsumable, ItemTypeLight, ItemTypeKey, ItemTypeTool,
		ItemTypeClothing, ItemTypeFood, ItemTypeTradeGood, ItemTypeTrash:
		return true
	}
	return false
}

// ItemQuality marks masterwork/masterpiece/power-wrought tiers. Each
// tier grants a bonus elsewhere (attack roll, armor check penalty, or
// unbreakable+attack-and-damage); this column just records the tier.
type ItemQuality string

const (
	QualityNormal       ItemQuality = "normal"
	QualityMasterwork   ItemQuality = "masterwork"
	QualityMasterpiece  ItemQuality = "masterpiece"
	QualityPowerWrought ItemQuality = "power_wrought"
)

func (q ItemQuality) IsValid() bool {
	switch q {
	case QualityNormal, QualityMasterwork, QualityMasterpiece, QualityPowerWrought:
		return true
	}
	return false
}

// ItemFlags is the bitset stored on items.flags. Compose with bitwise
// OR; check membership with `flags & FlagX != 0`. New bits append.
type ItemFlags uint64

const (
	FlagNoTake       ItemFlags = 1 << iota // pickup forbidden
	FlagNoDrop                             // drop forbidden once held
	FlagNoSell                             // shopkeepers refuse it
	FlagBindOnPickup                       // bound to first taker
	FlagMagic                              // detect-magic-visible
	FlagGlow                               // emits its own light
	FlagHum                                // emits a soft tone
	FlagTradeGood                          // exempt from sell-at-half rule
)

// Item is one world object. Bare-bones identity (id/name/short/room)
// covers `look` rendering; the type/quality/flags/stats fields layer
// on the gameplay-relevant fact pattern from the WoT equipment ref.
//
// RoomID == 0 means "not currently in a room" — a future inventory or
// respawn-pool slice will populate that state without a schema change.
type Item struct {
	ID         int64
	ExternalID string
	Name       string
	NameLower  string
	ShortDesc  string
	RoomID     int64

	Type    ItemType
	Weight  float64         // in pounds (matches the WoT tables)
	Value   currency.Amount // copper pennies
	Quality ItemQuality
	Flags   ItemFlags
	// Stats is the type-discriminated stat block. Decoded from
	// stats_json on load; one of the *Stats structs below or nil for
	// trash. Repos enforce that the concrete type matches Type.
	Stats ItemStats

	CreatedAt time.Time
}

// HasFlag reports whether the named bit is set. Tiny helper to keep
// callers from reaching for bitwise ops every time.
func (i Item) HasFlag(f ItemFlags) bool { return i.Flags&f != 0 }

// ItemStats is the marker interface implemented by every per-type
// stat struct. The interface is intentionally empty — it exists so
// Item.Stats can hold any of them without a typed any-cast.
type ItemStats interface{ itemStatsMarker() }

// WeaponStats matches the columns in Table 7-4. Damage dice are kept
// as the WoT shorthand string ("1d6", "2d4", "1d6/1d6" for double
// weapons) — the combat pipeline parses dice on the way to the roll.
// ThreatLow=0 means the default 20-only crit range; CritMult defaults
// to 2 when zero.
type WeaponStats struct {
	Proficiency  string   `json:"proficiency"`   // simple | martial | exotic
	Size         string   `json:"size"`          // tiny | small | medium | large
	Range        string   `json:"range"`         // melee | ranged | thrown
	Damage       string   `json:"damage"`        // "1d6", "1d6/1d6"
	ThreatLow    int      `json:"threat_low"`    // e.g. 18 for 18-20 threat
	CritMult     int      `json:"crit_mult"`     // 2/3/4
	RangeIncFt   int      `json:"range_inc_ft"`  // 0 for pure melee
	DamageType   []string `json:"damage_type"`   // any of B / P / S
	Special      []string `json:"special"`       // reach, double, set, trip, finesse, ...
	Subdual      bool     `json:"subdual"`       // §-tagged in the table
}

func (WeaponStats) itemStatsMarker() {}

// ArmorStats matches Table 7-5 (body armor only). Shields use
// ShieldStats so the bash/buckler/tower distinctions stay typed.
type ArmorStats struct {
	WeightClass  string `json:"weight_class"`  // light | medium | heavy
	Bonus        int    `json:"bonus"`         // armor bonus to defense
	MaxDex       int    `json:"max_dex"`       // dex cap to defense
	CheckPenalty int    `json:"check_penalty"` // negative number
	SpeedFt      int    `json:"speed_ft"`      // base 30 = unencumbered
}

func (ArmorStats) itemStatsMarker() {}

// ShieldStats covers buckler / small-wood / small-steel / large-wood /
// large-steel / tower. Bashing semantics live with the combat layer.
type ShieldStats struct {
	Kind         string `json:"kind"`          // buckler | small_wood | small_steel | large_wood | large_steel | tower
	Bonus        int    `json:"bonus"`         // armor bonus from the shield
	CheckPenalty int    `json:"check_penalty"` // negative; stacks with armor
}

func (ShieldStats) itemStatsMarker() {}

// ContainerStats covers backpacks, sacks, chests, barrels, pouches.
// LiquidPints non-zero marks the container as liquid-only (pitcher,
// waterskin, etc.) and disables the dry capacity fields. Real
// take/put enforcement waits on §14.
type ContainerStats struct {
	CapacityLbs  float64 `json:"capacity_lbs"`
	CapacityCuFt float64 `json:"capacity_cuft"`
	LiquidPints  float64 `json:"liquid_pints"`
	DepthCap     int     `json:"depth_cap"`     // 0 = use global default
	WeightMult   float64 `json:"weight_mult"`   // 1.0 = no reduction; bag-of-holding ≈ 0.1
}

func (ContainerStats) itemStatsMarker() {}

// ConsumableStats covers grenade flasks, healer's balm, antitoxin,
// trail rations, ale jugs. EffectID is a forward reference into the
// §11 affects catalog and is just an integer until that lands.
type ConsumableStats struct {
	Charges  int   `json:"charges"`
	EffectID int32 `json:"effect_id"`
}

func (ConsumableStats) itemStatsMarker() {}

// LightStats covers torches, lamps, lanterns. RadiusFt is the bright
// radius (lamp 15, lantern 30, torch 20). FuelTicks ≈ minutes of fuel
// at the current scheduler tick rate; the persist manager will trim
// the counter when ticks run.
type LightStats struct {
	RadiusFt  int `json:"radius_ft"`
	FuelTicks int `json:"fuel_ticks"`
}

func (LightStats) itemStatsMarker() {}

// KeyStats matches a key item to an exit. The KeyID string is what
// Exit.KeyExternalID compares against. Keeping it in a struct rather
// than reusing Item.ExternalID lets one key item open multiple
// exits, and lets `lock`/`unlock` find keys cheaply.
type KeyStats struct {
	KeyID string `json:"key_id"`
}

func (KeyStats) itemStatsMarker() {}

// ToolStats covers Table 7-7 toolkits and Table 7-9 masterwork tools.
// SkillTag is the §12 skill name the kit modifies (informational
// today; the pipeline reads it once skills are wired).
type ToolStats struct {
	SkillTag string `json:"skill_tag"`
	Charges  int    `json:"charges"` // 0 = unlimited
}

func (ToolStats) itemStatsMarker() {}

// ItemRepo is the persistence boundary for items.
type ItemRepo interface {
	// ListInRoom returns every item whose room_id equals the given id,
	// sorted by name. An empty result is not an error.
	ListInRoom(ctx context.Context, roomID int64) ([]Item, error)
	// Create inserts a new item. ExternalID must be non-empty.
	Create(ctx context.Context, i Item) (Item, error)
}

// ErrItemStatsTypeMismatch is returned when an Item's Stats concrete
// type doesn't match its declared Type (e.g. Type=weapon with
// ArmorStats). Repos surface this on Create rather than persisting
// nonsense; the YAML loader normalizes types before this point.
var ErrItemStatsTypeMismatch = errors.New("repo: item stats type does not match item type")
