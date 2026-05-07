// Package chargen owns the YAML-on-disk character-creation content
// catalog (backgrounds, classes, feats, skills, weaves) and the
// typed Catalog struct the multi-step CharacterCreate mode reads
// at runtime.
//
// Layout (see WORLD_DIR-style env: CHARGEN_DIR, default
// ./data/chargen):
//
//	backgrounds.yaml — 11 human backgrounds + Ogier
//	classes.yaml     — 7 hero classes
//	feats.yaml       — background feats and 1st-level feats
//	skills.yaml      — class-skill universe
//	weaves.yaml      — level-0 starting weaves for channelers
//
// The catalog is content, not state — it lives in YAML and never
// touches the DB. Loader is parallel to internal/news and
// internal/world: load once at boot, share an immutable Catalog
// across goroutines.
//
// References: docs/reference/{abilities,backgrounds,classes,feats,
// heroic-characteristics,equipment,the-one-power}.md.
package chargen

import (
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// SaveProgression marks a class save as "high" (good) or "low"
// (poor). The numeric BAB / save table is derivable from class +
// level; this field keys the formula.
type SaveProgression string

const (
	SaveHigh SaveProgression = "high"
	SaveLow  SaveProgression = "low"
)

// BABProgression keys the per-class base attack bonus formula.
// "high" → +1/level (Algai, Armsman, Woodsman). "medium" → +3/4/level
// (Initiate, Noble, Wilder). "low" → +1/2/level (Wanderer in some
// editions; placeholder for future edition variants).
type BABProgression string

const (
	BABHigh   BABProgression = "high"
	BABMedium BABProgression = "medium"
	BABLow    BABProgression = "low"
)

// Background is one entry from docs/reference/backgrounds.md Table
// 2-1. The Race field gates which races can pick it: human
// backgrounds list "human"; the synthetic Ogier entry lists "ogier".
//
// EquipmentOptions are mutually-exclusive starting bundles — pick
// one. Each Items entry is a free-form label for now; resolution to
// item external_ids happens in step #15 once the equipment catalog
// is wired.
type Background struct {
	ID                string            `yaml:"id"`
	Name              string            `yaml:"name"`
	Race              string            `yaml:"race"` // "human" | "ogier"
	HomeLanguage      string            `yaml:"home_language"`
	BonusLanguages    []string          `yaml:"bonus_languages"`
	BonusFeats        []string          `yaml:"bonus_feats"`       // feat IDs
	BackgroundSkills  []string          `yaml:"background_skills"` // skill IDs
	SkillRestriction  string            `yaml:"skill_restriction,omitempty"`
	WeaponRestriction string            `yaml:"weapon_restriction,omitempty"`
	RequiredSkill     string            `yaml:"required_skill,omitempty"`
	HeightModIn       int               `yaml:"height_mod_in,omitempty"`
	EquipmentOptions  []EquipmentOption `yaml:"equipment_options"`
	Description       string            `yaml:"description"`

	// Enum maps to creature.Background. Optional in YAML; loader
	// resolves it from ID via knownBackgroundEnum and panics on
	// mismatch. Ogier entries leave it at its zero default.
	Enum creature.Background `yaml:"-"`
}

// EquipmentOption is one of the "Equipment Options" alternatives
// from a background table. Items is a free-form list for now;
// later wiring (#15) maps these to item external_ids and spawns
// them via ItemRepo.Create.
type EquipmentOption struct {
	Label string   `yaml:"label"`
	Items []string `yaml:"items"`
}

// Class is one of the seven hero classes. KeyAbilities is the
// suggested-priority list from each class's narrative blurb. The
// Channeler bool is the gate the chargen branch in #15 checks before
// asking about Source/affinities/starting weaves.
type Class struct {
	ID            string          `yaml:"id"`
	Name          string          `yaml:"name"`
	Abbrev        string          `yaml:"abbrev"`
	HitDie        int             `yaml:"hit_die"` // 4, 6, 8, 10
	BAB           BABProgression  `yaml:"bab"`
	SaveFort      SaveProgression `yaml:"save_fort"`
	SaveRef       SaveProgression `yaml:"save_ref"`
	SaveWill      SaveProgression `yaml:"save_will"`
	SkillPoints   int             `yaml:"skill_points"` // per-level base, ×4 at 1st
	ClassSkills   []string        `yaml:"class_skills"` // skill IDs
	KeyAbilities  []string        `yaml:"key_abilities"`
	Channeler     bool            `yaml:"channeler"`
	ChannelSource string          `yaml:"channel_source,omitempty"` // "saidin"|"saidar"|"either"
	Description   string          `yaml:"description"`

	// Enum maps to creature.Class. Loader fills it.
	Enum creature.Class `yaml:"-"`
}

// Feat covers both 1st-level general feats and background-only
// feats (Blooded, Cosmopolitan, …). Backgrounds is the set of
// background IDs whose 1st-level slot can pick this feat;
// background:false means it's a general 1st-level feat available
// to anyone.
type Feat struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Background  bool     `yaml:"background"` // background-feat-only
	Backgrounds []string `yaml:"backgrounds,omitempty"`
	Description string   `yaml:"description"`
}

// Skill is one entry in the class-skill universe. Ability is the
// keyed ability ("Str", "Dex", "Con", "Int", "Wis", "Cha"). Used
// by chargen to render skill point allocation in #15.
type Skill struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Ability     string `yaml:"ability"`
	Description string `yaml:"description,omitempty"`
}

// ItemTemplate is one starting-equipment template — the chargen-side
// counterpart to a `data/world/` item. The chargen catalog is content,
// not state, so these never live in any room or container; they're
// cloned into a freshly-created character's inventory at finalize
// time via ItemRepo.Create with a unique runtime external_id.
//
// Schema mirrors world items (see data/world/README.md): typed Stats
// (decoded from a flat YAML stats: block via the same JSON-marshal
// path the world loader uses) for weapon/armor/shield/container/
// consumable/light/key/tool; clothing/food/trade_good/trash carry no
// Stats. Currency strings parse via currency.Parse.
type ItemTemplate struct {
	ID      string           `yaml:"id"`
	Name    string           `yaml:"name"`
	Short   string           `yaml:"short"`
	Type    repo.ItemType    `yaml:"type"`
	Weight  float64          `yaml:"weight"`
	Value   string           `yaml:"value"` // currency.Parse — "5mk", "100mk"
	Quality repo.ItemQuality `yaml:"quality"`
	Flags   []string         `yaml:"flags"`
	Stats   map[string]any   `yaml:"stats"`

	// Resolved values stamped by the loader — never set in YAML.
	parsedValue currency.Amount
	parsedFlags repo.ItemFlags
	parsedStats repo.ItemStats
}

// ParsedValue returns the loader-resolved currency amount.
func (it *ItemTemplate) ParsedValue() currency.Amount { return it.parsedValue }

// ParsedFlags returns the loader-resolved flag bitset.
func (it *ItemTemplate) ParsedFlags() repo.ItemFlags { return it.parsedFlags }

// ParsedStats returns the loader-resolved typed Stats struct (or nil
// for the untyped tier — clothing/food/trade_good/trash).
func (it *ItemTemplate) ParsedStats() repo.ItemStats { return it.parsedStats }

// Weave is a level-0 starting weave option for channelers. Power
// is one of "Air"/"Earth"/"Fire"/"Water"/"Spirit". Chargen step #15
// uses Power to filter by selected affinities.
type Weave struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Level       int    `yaml:"level"`
	Power       string `yaml:"power"`
	Description string `yaml:"description,omitempty"`
}
