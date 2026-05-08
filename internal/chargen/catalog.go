package chargen

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"gopkg.in/yaml.v3"
)

// Catalog is the immutable bundle of chargen content built once at
// boot from data/chargen/*.yaml. All accessors return defensive
// references — callers must not mutate the returned structs or
// slices.
type Catalog struct {
	backgrounds map[string]*Background
	classes     map[string]*Class
	feats       map[string]*Feat
	skills      map[string]*Skill
	weaves      map[string]*Weave
	items       map[string]*ItemTemplate

	// Stable ordering for menus.
	bgOrder    []string
	classOrder []string
	featOrder  []string
	skillOrder []string
	weaveOrder []string
	itemOrder  []string
}

// Filenames the loader expects under fsys's root.
const (
	fileBackgrounds = "backgrounds.yaml"
	fileClasses     = "classes.yaml"
	fileFeats       = "feats.yaml"
	fileSkills      = "skills.yaml"
	fileWeaves      = "weaves.yaml"
	// fileItems is declared in items.go alongside the loader logic.
)

// knownBackgroundEnum maps catalog id → creature.Background. The
// loader requires every background id to be in this table so
// in-memory characters can serialize their selection through the
// existing creature enums.
var knownBackgroundEnum = map[string]creature.Background{
	"aiel":         creature.BackgroundAiel,
	"athaan_miere": creature.BackgroundAthaanMiere,
	"borderlander": creature.BackgroundBorderlander,
	"cairhienin":   creature.BackgroundCairhienin,
	"domani":       creature.BackgroundDomani,
	"ebou_dari":    creature.BackgroundEbouDari,
	"illianer":     creature.BackgroundIllianer,
	"midlander":    creature.BackgroundMidlander,
	"taraboner":    creature.BackgroundTaraboner,
	"tairen":       creature.BackgroundTairen,
	"tar_valoner":  creature.BackgroundTarValoner,
}

// knownClassEnum maps catalog id → creature.Class.
var knownClassEnum = map[string]creature.Class{
	"algai_d_siswai": creature.ClassAlgaiDSiswai,
	"armsman":        creature.ClassArmsman,
	"initiate":       creature.ClassInitiate,
	"noble":          creature.ClassNoble,
	"wanderer":       creature.ClassWanderer,
	"wilder":         creature.ClassWilder,
	"woodsman":       creature.ClassWoodsman,
}

// validRaces are the YAML race tokens accepted on background
// entries. Mirrors creature.Race.
var validRaces = map[string]struct{}{
	"human": {},
	"ogier": {},
}

var validAbilities = map[string]struct{}{
	"Str": {}, "Dex": {}, "Con": {}, "Int": {}, "Wis": {}, "Cha": {},
}

var validPowers = map[string]struct{}{
	"Air": {}, "Earth": {}, "Fire": {}, "Water": {}, "Spirit": {},
}

// validChannelSources matches the YAML tokens for which Source(s) of
// the One Power a channeler class draws on. "either" covers wilders
// and the open-gender Initiate seed; future class entries (Asha'man,
// Aes Sedai branches) will pick one.
var validChannelSources = map[string]struct{}{
	"saidin": {}, "saidar": {}, "either": {},
}

// Load reads the chargen YAML catalog from fsys (typically
// os.DirFS("./data/chargen")) and validates cross-references. The
// returned Catalog is safe to share across goroutines.
func Load(fsys fs.FS) (*Catalog, error) {
	c := &Catalog{
		backgrounds: map[string]*Background{},
		classes:     map[string]*Class{},
		feats:       map[string]*Feat{},
		skills:      map[string]*Skill{},
		weaves:      map[string]*Weave{},
		items:       map[string]*ItemTemplate{},
	}

	if err := readYAMLList(fsys, fileSkills, &c.skills, &c.skillOrder, "skill"); err != nil {
		return nil, err
	}
	if err := readYAMLList(fsys, fileFeats, &c.feats, &c.featOrder, "feat"); err != nil {
		return nil, err
	}
	if err := readYAMLList(fsys, fileBackgrounds, &c.backgrounds, &c.bgOrder, "background"); err != nil {
		return nil, err
	}
	if err := readYAMLList(fsys, fileClasses, &c.classes, &c.classOrder, "class"); err != nil {
		return nil, err
	}
	if err := readYAMLList(fsys, fileWeaves, &c.weaves, &c.weaveOrder, "weave"); err != nil {
		return nil, err
	}
	if err := readYAMLList(fsys, fileItems, &c.items, &c.itemOrder, "item"); err != nil {
		return nil, err
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// readYAMLList parses a top-level list of T from `name` under fsys
// and indexes it into target[id] = &entry, also recording the
// declaration order in orderOut for stable menu rendering.
//
// kind is used for error wrapping ("background", "class", …).
func readYAMLList[T any](fsys fs.FS, name string, target *map[string]*T, orderOut *[]string, kind string) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("chargen: read %s: %w", name, err)
	}
	var raw []T
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("chargen: parse %s: %w", name, err)
	}
	if len(raw) == 0 {
		return fmt.Errorf("chargen: %s: empty catalog", name)
	}
	for i := range raw {
		entry := raw[i]
		id := getID(&entry)
		if id == "" {
			return fmt.Errorf("chargen: %s: entry %d missing id", name, i)
		}
		if _, dup := (*target)[id]; dup {
			return fmt.Errorf("chargen: %s: duplicate %s id %q", name, kind, id)
		}
		(*target)[id] = &raw[i]
		*orderOut = append(*orderOut, id)
	}
	return nil
}

// getID is a tiny generic shim — every catalog struct exposes ID
// as the first string field tagged `yaml:"id"`. We avoid reflection
// by type-asserting through a known-types switch.
func getID[T any](v *T) string {
	switch x := any(v).(type) {
	case *Background:
		return x.ID
	case *Class:
		return x.ID
	case *Feat:
		return x.ID
	case *Skill:
		return x.ID
	case *Weave:
		return x.ID
	case *ItemTemplate:
		return x.ID
	default:
		return ""
	}
}

// validate enforces cross-reference and enum integrity across the
// loaded catalogs. Failure here is a hard boot error — content bugs
// belong in CI, not in production.
func (c *Catalog) validate() error {
	var errs []string

	// Skills: ability must be one of Str..Cha.
	for _, s := range c.skills {
		if _, ok := validAbilities[s.Ability]; !ok {
			errs = append(errs, fmt.Sprintf("skill %q: invalid ability %q", s.ID, s.Ability))
		}
	}

	// Feats: backgrounds must resolve when listed; non-background
	// feats must not carry a background list (these are mutually
	// exclusive — a feat is either background-restricted or general).
	for _, f := range c.feats {
		for _, bg := range f.Backgrounds {
			if _, ok := c.backgrounds[bg]; !ok {
				errs = append(errs, fmt.Sprintf("feat %q: unknown background %q", f.ID, bg))
			}
		}
		if !f.Background && len(f.Backgrounds) > 0 {
			errs = append(errs, fmt.Sprintf("feat %q: backgrounds set but background:false", f.ID))
		}
	}

	// Backgrounds: race valid; bonus_feats and background_skills
	// resolve; required_skill resolves when set; enum stamped from
	// id table for human entries.
	for _, bg := range c.backgrounds {
		if _, ok := validRaces[bg.Race]; !ok {
			errs = append(errs, fmt.Sprintf("background %q: invalid race %q", bg.ID, bg.Race))
		}
		for _, fid := range bg.BonusFeats {
			if _, ok := c.feats[fid]; !ok {
				errs = append(errs, fmt.Sprintf("background %q: unknown feat %q", bg.ID, fid))
			}
		}
		for _, sid := range bg.BackgroundSkills {
			if _, ok := c.skills[sid]; !ok {
				errs = append(errs, fmt.Sprintf("background %q: unknown skill %q", bg.ID, sid))
			}
		}
		if bg.RequiredSkill != "" {
			if _, ok := c.skills[bg.RequiredSkill]; !ok {
				errs = append(errs, fmt.Sprintf("background %q: required_skill unknown %q", bg.ID, bg.RequiredSkill))
			}
		}
		if len(bg.EquipmentOptions) == 0 {
			errs = append(errs, fmt.Sprintf("background %q: no equipment_options", bg.ID))
		}
		// Every background id must be mapped to a creature.Background
		// enum so Phase D/E switches on bg.Enum cannot misroute. When
		// Ogier lands in #14, add its id to knownBackgroundEnum (and
		// to creature.Background as a new constant) at the same time.
		enum, ok := knownBackgroundEnum[bg.ID]
		if !ok {
			errs = append(errs, fmt.Sprintf("background %q: no creature.Background enum mapping", bg.ID))
		} else {
			bg.Enum = enum
		}
	}

	// Classes: hit_die ∈ {4,6,8,10}; saves valid; class_skills
	// resolve; channel_source set when channeler; enum stamped.
	for _, cl := range c.classes {
		if !validHitDie(cl.HitDie) {
			errs = append(errs, fmt.Sprintf("class %q: invalid hit_die %d", cl.ID, cl.HitDie))
		}
		if !validSave(cl.SaveFort) || !validSave(cl.SaveRef) || !validSave(cl.SaveWill) {
			errs = append(errs, fmt.Sprintf("class %q: save progression must be high|low", cl.ID))
		}
		if !validBAB(cl.BAB) {
			errs = append(errs, fmt.Sprintf("class %q: bab must be high|medium|low", cl.ID))
		}
		if cl.SkillPoints <= 0 {
			errs = append(errs, fmt.Sprintf("class %q: skill_points must be > 0", cl.ID))
		}
		for _, sid := range cl.ClassSkills {
			if _, ok := c.skills[sid]; !ok {
				errs = append(errs, fmt.Sprintf("class %q: unknown class_skill %q", cl.ID, sid))
			}
		}
		if cl.Channeler {
			if cl.ChannelSource == "" {
				errs = append(errs, fmt.Sprintf("class %q: channeler requires channel_source", cl.ID))
			} else if _, ok := validChannelSources[cl.ChannelSource]; !ok {
				errs = append(errs, fmt.Sprintf("class %q: invalid channel_source %q", cl.ID, cl.ChannelSource))
			}
		}
		if enum, ok := knownClassEnum[cl.ID]; ok {
			cl.Enum = enum
		} else {
			errs = append(errs, fmt.Sprintf("class %q: no creature.Class enum mapping", cl.ID))
		}
	}

	// Weaves: power must be one of the five.
	for _, w := range c.weaves {
		if _, ok := validPowers[w.Power]; !ok {
			errs = append(errs, fmt.Sprintf("weave %q: invalid power %q", w.ID, w.Power))
		}
		if w.Level < 0 || w.Level > 9 {
			errs = append(errs, fmt.Sprintf("weave %q: level %d out of range", w.ID, w.Level))
		}
		if w.PracticeCost < 0 || w.PracticeCost > 255 {
			errs = append(errs, fmt.Sprintf("weave %q: practice_cost %d out of range [0,255]", w.ID, w.PracticeCost))
		}
	}

	// Items: typed Stats decode + cross-reference every
	// background.equipment_options[].items entry.
	errs = append(errs, c.validateItems()...)

	if len(errs) > 0 {
		sort.Strings(errs)
		return errors.New("chargen: validation: " + strings.Join(errs, "; "))
	}
	return nil
}

func validHitDie(n int) bool {
	return n == 4 || n == 6 || n == 8 || n == 10
}

func validSave(s SaveProgression) bool {
	return s == SaveHigh || s == SaveLow
}

func validBAB(b BABProgression) bool {
	return b == BABHigh || b == BABMedium || b == BABLow
}

// --- accessors -----------------------------------------------------

// Background returns the entry by id, ok=false on miss.
func (c *Catalog) Background(id string) (*Background, bool) { v, ok := c.backgrounds[id]; return v, ok }

// Class returns the entry by id, ok=false on miss.
func (c *Catalog) Class(id string) (*Class, bool) { v, ok := c.classes[id]; return v, ok }

// Feat returns the entry by id, ok=false on miss.
func (c *Catalog) Feat(id string) (*Feat, bool) { v, ok := c.feats[id]; return v, ok }

// Skill returns the entry by id, ok=false on miss.
func (c *Catalog) Skill(id string) (*Skill, bool) { v, ok := c.skills[id]; return v, ok }

// Weave returns the entry by id, ok=false on miss.
func (c *Catalog) Weave(id string) (*Weave, bool) { v, ok := c.weaves[id]; return v, ok }

// Backgrounds returns all entries in declaration order.
func (c *Catalog) Backgrounds() []*Background {
	out := make([]*Background, 0, len(c.bgOrder))
	for _, id := range c.bgOrder {
		out = append(out, c.backgrounds[id])
	}
	return out
}

// BackgroundsForRace returns the subset of backgrounds whose Race
// matches the given token ("human" or "ogier"). Used by chargen #13
// to filter the menu after the player picks a race.
func (c *Catalog) BackgroundsForRace(race string) []*Background {
	out := []*Background{}
	for _, id := range c.bgOrder {
		bg := c.backgrounds[id]
		if bg.Race == race {
			out = append(out, bg)
		}
	}
	return out
}

// Classes returns all classes in declaration order.
func (c *Catalog) Classes() []*Class {
	out := make([]*Class, 0, len(c.classOrder))
	for _, id := range c.classOrder {
		out = append(out, c.classes[id])
	}
	return out
}

// ClassesForRace returns classes legal for a given race. Ogier may
// not channel (book lore — see backgrounds.md §Ogier), so Initiate
// and Wilder are filtered out for ogier picks. Everything else is
// race-agnostic for now; Phase E may layer additional gates.
func (c *Catalog) ClassesForRace(race string) []*Class {
	out := []*Class{}
	for _, id := range c.classOrder {
		cl := c.classes[id]
		if race == "ogier" && cl.Channeler {
			continue
		}
		out = append(out, cl)
	}
	return out
}

// Feats returns all feats in declaration order.
func (c *Catalog) Feats() []*Feat {
	out := make([]*Feat, 0, len(c.featOrder))
	for _, id := range c.featOrder {
		out = append(out, c.feats[id])
	}
	return out
}

// FeatsForBackground returns the background-restricted feats whose
// Backgrounds list includes bgID. General (non-background) feats
// are filtered out — they live in a separate menu in chargen #15.
func (c *Catalog) FeatsForBackground(bgID string) []*Feat {
	out := []*Feat{}
	for _, id := range c.featOrder {
		f := c.feats[id]
		if !f.Background {
			continue
		}
		for _, b := range f.Backgrounds {
			if b == bgID {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// Skills returns all skills in declaration order.
func (c *Catalog) Skills() []*Skill {
	out := make([]*Skill, 0, len(c.skillOrder))
	for _, id := range c.skillOrder {
		out = append(out, c.skills[id])
	}
	return out
}

// Weaves returns all weaves in declaration order.
func (c *Catalog) Weaves() []*Weave {
	out := make([]*Weave, 0, len(c.weaveOrder))
	for _, id := range c.weaveOrder {
		out = append(out, c.weaves[id])
	}
	return out
}

// WeavesAtLevel returns weaves with the given level. Chargen #15
// uses this with level=0 for starting weaves.
func (c *Catalog) WeavesAtLevel(level int) []*Weave {
	out := []*Weave{}
	for _, id := range c.weaveOrder {
		w := c.weaves[id]
		if w.Level == level {
			out = append(out, w)
		}
	}
	return out
}
