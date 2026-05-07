// Package world owns the YAML-on-disk world definition: parsing,
// validation, and one-shot population of the SQL world tables on boot.
//
// The runtime never reads YAML; only LoadAndSync does. Once it returns,
// the look / move commands keep using the existing repo interfaces
// against SQLite as before.
package world

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"gopkg.in/yaml.v3"
)

// Zone is the metadata for a single zone folder. The folder name on
// disk is the zone's source-of-truth identifier; this struct carries
// the human display name plus the per-zone behavior fields the
// runtime keys off of (level gating, reset cadence, ambient ticker,
// climate). Every behavior field is optional in YAML — defaults are
// applied at insert time so authoring stays terse for stub zones.
type Zone struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Builder string `yaml:"builder"`

	// LevelRange is advisory content gating. nil → 1..60 default
	// applied at insert time. Pointer so a missing block stays
	// distinguishable from {min:0, max:0} and surfaces as the default.
	LevelRange *LevelRange `yaml:"level_range"`

	// ResetIntervalS is how often the §9 areaReset bucket fires this
	// zone, in seconds. 0 → 600 (10 minutes) default applied at
	// insert time. Set explicitly to a value > 0 to override.
	ResetIntervalS int `yaml:"reset_interval_s"`

	// ResetMode is one of "always" / "empty" / "never". Empty string
	// → "empty" default applied at insert time. Validation rejects
	// any other value.
	ResetMode string `yaml:"reset_mode"`

	// Climate is a free-text bucket consumed by §10 ambient/weather
	// rendering ("temperate", "arid", "blighted", ...). Empty is fine.
	Climate string `yaml:"climate"`

	// Ambient is the rotating set of zone-wide ambient lines emitted
	// by §10's ambient ticker. Empty list disables the ticker for
	// this zone.
	Ambient []string `yaml:"ambient"`

	// SourceDir is the folder this zone was loaded from, populated by
	// the loader after Decode. Used in error messages.
	SourceDir string `yaml:"-"`
}

// LevelRange mirrors the zones.min_level / max_level columns. Both
// fields default to 0 when unmarshaled from a missing block; the
// loader replaces a nil *LevelRange with the schema defaults. When
// the block is present, validation enforces min >= 1 and max >= min.
type LevelRange struct {
	Min int `yaml:"min"`
	Max int `yaml:"max"`
}

// Room is one entry in `rooms.yaml`. The loader walks rooms[].exits
// after parsing to construct Exit rows; exits don't get their own file.
type Room struct {
	ID      string          `yaml:"id"`
	Starter bool            `yaml:"starter"`
	Name    string          `yaml:"name"`
	Short   string          `yaml:"short"`
	Long    string          `yaml:"long"`
	Exits   map[string]Exit `yaml:"exits"`

	// Flags / sector / lighting / coords land in migration 0012.
	// Builders may omit any of these; defaults: outdoors, city
	// sector, fully lit, origin coords.
	Flags      RoomFlags `yaml:"flags"`
	Sector     string    `yaml:"sector"`
	LightLevel *int      `yaml:"light_level"` // pointer so 0 ≠ "unspecified"
	Coords     *Coords   `yaml:"coords"`

	// Descriptions: keyword -> long-form text, rendered by
	// `look <keyword>`. Keys are lowercased on insert by the repo so
	// authoring case doesn't matter.
	Descriptions map[string]string `yaml:"descriptions"`

	SourceFile string `yaml:"-"`
	Line       int    `yaml:"-"`

	// ZoneExternalID is stamped by the loader after parseZone so
	// downstream insertion knows which zones row this room belongs
	// to. Not authored in YAML; the directory wins.
	ZoneExternalID string `yaml:"-"`
}

// Exit is a YAML-side exit entry. Supports two authoring forms:
//
//	exits:
//	  north: gate.entry           # shorthand, target room id only
//	  south: { to: gate.bridge, closed: true, locked: true,
//	           key: iron.key, difficulty: 15,
//	           description: "A heavy oak door bound with iron." }
//
// The shorthand decodes through UnmarshalYAML and produces an Exit
// with all flags false. The object form fills any subset of fields.
// Pickable defaults to true (a locked exit is pickable unless the
// builder explicitly disables it).
type Exit struct {
	To             string `yaml:"to"`
	Closed         bool   `yaml:"closed"`
	Locked         bool   `yaml:"locked"`
	Pickable       *bool  `yaml:"pickable"` // pointer so zero ≠ "unspecified"
	Hidden         bool   `yaml:"hidden"`
	NoPass         bool   `yaml:"nopass"`
	Key            string `yaml:"key"`        // item external_id
	LockDifficulty int    `yaml:"difficulty"` // 0 means no skill check
	Description    string `yaml:"description"`
}

// UnmarshalYAML accepts either a scalar (shorthand: target room id)
// or a mapping (full object form). Anything else is a parse error.
func (e *Exit) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		e.To = node.Value
		return nil
	case yaml.MappingNode:
		// Decode into a parallel struct to avoid recursion through
		// our own UnmarshalYAML.
		type rawExit Exit
		var r rawExit
		if err := node.Decode(&r); err != nil {
			return err
		}
		*e = Exit(r)
		return nil
	default:
		return fmt.Errorf("exit: expected scalar or mapping, got %v", node.Kind)
	}
}

// RoomFlags is the YAML-side mirror of repo.RoomFlags. Defaults are
// all false; the loader maps these onto the rooms.* INTEGER columns.
type RoomFlags struct {
	Indoors    bool `yaml:"indoors"`
	NoPVP      bool `yaml:"nopvp"`
	NoTeleport bool `yaml:"noteleport"`
	Dark       bool `yaml:"dark"`
	Silent     bool `yaml:"silent"`
	Peaceful   bool `yaml:"peaceful"`
	// NoMap hides the room from the §10 BFS minimap. See repo.RoomFlags.NoMap.
	NoMap bool `yaml:"nomap"`
}

// Coords is an optional position used by §10 map/track. Authoring is
// optional; rooms without coords sort to (0,0,0) which is fine for
// zones that don't need a grid.
type Coords struct {
	X int `yaml:"x"`
	Y int `yaml:"y"`
	Z int `yaml:"z"`
}

// Item is one entry in `items.yaml`. The Type/Weight/Value/Quality/
// Flags/Stats fields land in migration 0015 (§9 Item taxonomy).
// Builders may omit any of them — defaults are type=trash, weight=0,
// value=0, quality=normal, no flags, empty stats. The Stats sub-block
// is YAML-decoded as a generic map and converted to the typed struct
// inside the loader so a typo in `weapon: damage` fails at validate
// time instead of silently zeroing.
type Item struct {
	ID    string `yaml:"id"`
	Room  string `yaml:"room"`
	Name  string `yaml:"name"`
	Short string `yaml:"short"`

	Type    string         `yaml:"type"`
	Weight  float64        `yaml:"weight"`
	Value   string         `yaml:"value"`   // currency.Parse — "5 mk", "2 gc 1sp"
	Quality string         `yaml:"quality"` // normal | masterwork | masterpiece | power_wrought
	Flags   []string       `yaml:"flags"`   // notake, nodrop, ...
	Stats   map[string]any `yaml:"stats"`   // type-discriminated sub-block

	SourceFile string `yaml:"-"`
	Line       int    `yaml:"-"`
}

// Mob is one entry in `mobs.yaml`.
type Mob struct {
	ID    string `yaml:"id"`
	Room  string `yaml:"room"`
	Name  string `yaml:"name"`
	Short string `yaml:"short"`
	// WanderChance overrides the §10 wander tick's per-template
	// probability for this mob. Optional; nil keeps the
	// creature.DefaultWanderChance baseline. Values outside [0, 1]
	// are clamped at the repo layer.
	WanderChance *float64 `yaml:"wander_chance,omitempty"`

	// Shop, if present, marks this mob as a shopkeeper (§14). The
	// loader inserts the matching `shops` row keyed to the
	// mob_template the mob spawns from. Stock lines materialize as
	// `shop_stock` rows referencing existing item templates by
	// external_id. Sentinels: qty=-1 + qty_max=-1 → infinite stock.
	Shop *Shop `yaml:"shop,omitempty"`

	// Banker, if present, marks this mob as a banker (§14). The
	// loader inserts the matching `bankers` row keyed to the
	// mob_template. V1 carries operating hours only — no fees, no
	// min-deposit, no item vault.
	Banker *Banker `yaml:"banker,omitempty"`

	SourceFile string `yaml:"-"`
	Line       int    `yaml:"-"`
}

// Shop is the optional `shop:` sub-block on a mob YAML entry.
// Defaults are filled in by the loader: SellMarkup=1.0,
// BuyMarkdown=0.5, RestockIntervalS=3600, OpenHour==CloseHour=>24h.
type Shop struct {
	BuyTypes         []string    `yaml:"buy_types"`
	SellMarkup       *float64    `yaml:"sell_markup,omitempty"`
	BuyMarkdown      *float64    `yaml:"buy_markdown,omitempty"`
	OpenHour         *int        `yaml:"open_hour,omitempty"`
	CloseHour        *int        `yaml:"close_hour,omitempty"`
	RestockIntervalS *int        `yaml:"restock_interval_s,omitempty"`
	Stock            []ShopStock `yaml:"stock"`
}

// ShopStock is one inventory line under a shop block.
type ShopStock struct {
	Item   string `yaml:"item"`    // item external_id
	Qty    int    `yaml:"qty"`     // -1 = infinite
	QtyMax int    `yaml:"qty_max"` // -1 = no restock cap
}

// Banker is the optional `banker:` sub-block on a mob YAML entry.
// Defaults are filled in by the loader: OpenHour==CloseHour=>24h.
type Banker struct {
	OpenHour  *int `yaml:"open_hour,omitempty"`
	CloseHour *int `yaml:"close_hour,omitempty"`
}

// World is the parsed-and-validated set of every zone the loader saw.
type World struct {
	Zones []Zone
	Rooms []Room
	Items []Item
	Mobs  []Mob
}

// parseZone reads zone.yaml + rooms.yaml + items.yaml + mobs.yaml from
// dir on src. Missing items.yaml / mobs.yaml is OK; missing zone.yaml
// or rooms.yaml is an error (a zone with no rooms is a typo we want to
// surface). Each parsed entity remembers its source file + line so
// validation errors point back to the YAML.
func parseZone(src fs.FS, dir string) (Zone, []Room, []Item, []Mob, error) {
	var z Zone
	zonePath := path.Join(dir, "zone.yaml")
	if err := decodeOne(src, zonePath, &z); err != nil {
		return Zone{}, nil, nil, nil, err
	}
	if z.ID == "" {
		return Zone{}, nil, nil, nil, fmt.Errorf("%s: zone.id is empty", zonePath)
	}
	z.SourceDir = dir

	roomsPath := path.Join(dir, "rooms.yaml")
	rooms, err := decodeRooms(src, roomsPath)
	if err != nil {
		return Zone{}, nil, nil, nil, err
	}
	if len(rooms) == 0 {
		return Zone{}, nil, nil, nil, fmt.Errorf("%s: zone has no rooms", roomsPath)
	}
	// Stamp every room with its owning zone's external id so
	// insertRooms can resolve it to a zones.id without re-walking
	// the source tree.
	for i := range rooms {
		rooms[i].ZoneExternalID = z.ID
	}

	items, err := decodeItems(src, path.Join(dir, "items.yaml"))
	if err != nil {
		return Zone{}, nil, nil, nil, err
	}
	mobs, err := decodeMobs(src, path.Join(dir, "mobs.yaml"))
	if err != nil {
		return Zone{}, nil, nil, nil, err
	}
	return z, rooms, items, mobs, nil
}

// decodeOne reads a single YAML document into out. Used for zone.yaml.
func decodeOne(src fs.FS, p string, out any) error {
	body, err := fs.ReadFile(src, p)
	if err != nil {
		return fmt.Errorf("read %s: %w", p, err)
	}
	if err := yaml.Unmarshal(body, out); err != nil {
		return fmt.Errorf("parse %s: %w", p, err)
	}
	return nil
}

// decodeRooms reads a YAML sequence of rooms and stamps each with its
// source file + start line for downstream validation errors.
func decodeRooms(src fs.FS, p string) ([]Room, error) {
	body, err := fs.ReadFile(src, p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: expected a YAML sequence", p)
	}
	seq := doc.Content[0]
	out := make([]Room, 0, len(seq.Content))
	for _, n := range seq.Content {
		var r Room
		if err := n.Decode(&r); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", p, n.Line, err)
		}
		r.SourceFile = p
		r.Line = n.Line
		out = append(out, r)
	}
	return out, nil
}

// decodeItems reads a YAML sequence of items. Returns nil (not an
// error) if the file is missing — items.yaml is optional per zone.
func decodeItems(src fs.FS, p string) ([]Item, error) {
	body, err := fs.ReadFile(src, p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	if doc.Content[0].Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: expected a YAML sequence", p)
	}
	seq := doc.Content[0]
	out := make([]Item, 0, len(seq.Content))
	for _, n := range seq.Content {
		var i Item
		if err := n.Decode(&i); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", p, n.Line, err)
		}
		i.SourceFile = p
		i.Line = n.Line
		out = append(out, i)
	}
	return out, nil
}

// decodeMobs is the structural twin of decodeItems.
func decodeMobs(src fs.FS, p string) ([]Mob, error) {
	body, err := fs.ReadFile(src, p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	if doc.Content[0].Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: expected a YAML sequence", p)
	}
	seq := doc.Content[0]
	out := make([]Mob, 0, len(seq.Content))
	for _, n := range seq.Content {
		var m Mob
		if err := n.Decode(&m); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", p, n.Line, err)
		}
		m.SourceFile = p
		m.Line = n.Line
		out = append(out, m)
	}
	return out, nil
}
