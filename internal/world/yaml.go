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
// disk is the zone's source-of-truth identifier; this struct just
// carries the human display name and any future per-zone settings.
type Zone struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`

	// SourceDir is the folder this zone was loaded from, populated by
	// the loader after Decode. Used in error messages.
	SourceDir string `yaml:"-"`
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
}

// Exit is a YAML-side exit entry. Supports two authoring forms:
//
//   exits:
//     north: gate.entry           # shorthand, target room id only
//     south: { to: gate.bridge, closed: true, locked: true,
//              key: iron.key, difficulty: 15,
//              description: "A heavy oak door bound with iron." }
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
}

// Coords is an optional position used by §10 map/track. Authoring is
// optional; rooms without coords sort to (0,0,0) which is fine for
// zones that don't need a grid.
type Coords struct {
	X int `yaml:"x"`
	Y int `yaml:"y"`
	Z int `yaml:"z"`
}

// Item is one entry in `items.yaml`.
type Item struct {
	ID    string `yaml:"id"`
	Room  string `yaml:"room"`
	Name  string `yaml:"name"`
	Short string `yaml:"short"`

	SourceFile string `yaml:"-"`
	Line       int    `yaml:"-"`
}

// Mob is one entry in `mobs.yaml`.
type Mob struct {
	ID    string `yaml:"id"`
	Room  string `yaml:"room"`
	Name  string `yaml:"name"`
	Short string `yaml:"short"`

	SourceFile string `yaml:"-"`
	Line       int    `yaml:"-"`
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

