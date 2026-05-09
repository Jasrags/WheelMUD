package world

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/dialogue"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"gopkg.in/yaml.v3"
)

// validate runs every cross-reference + format check against w. It is
// strict and fail-fast — the first error found is returned, so the
// loader never produces a partial DB load. Errors are formatted with
// the originating file:line so a builder can jump straight to the YAML.
func validate(w *World) error {
	if err := validateZones(w.Zones); err != nil {
		return err
	}
	if err := validateRoomIDs(w.Rooms); err != nil {
		return err
	}
	if err := validateStarter(w.Rooms); err != nil {
		return err
	}
	if err := validateExits(w.Rooms); err != nil {
		return err
	}
	if err := validateItems(w.Items, w.Rooms); err != nil {
		return err
	}
	if err := validateMobs(w.Mobs, w.Rooms, w.Items); err != nil {
		return err
	}
	return nil
}

// validExternalID enforces the same charset as the rest of the
// codebase: printable ASCII, no whitespace.
func validExternalID(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 {
			return false
		}
		if c <= ' ' || c == 0x7F {
			return false
		}
	}
	return true
}

// validateZones enforces zone-level invariants: external_id format +
// uniqueness, reset_mode in the known enum, level range bounds,
// non-negative reset interval. Catching these here means the SQLite
// CHECK constraints in migration 0016 are a backstop, not the front
// line — the loader can return a builder-friendly error pointing at
// the offending zone.yaml instead of a generic SQL "constraint failed".
func validateZones(zones []Zone) error {
	seen := make(map[string]Zone, len(zones))
	for _, z := range zones {
		// SourceDir is a directory like "starter" or
		// "westlands/andor/two_rivers/emonds_field"; "<dir>/zone.yaml"
		// is the file builders expect to edit.
		zonePath := z.SourceDir + "/zone.yaml"
		if !validExternalID(z.ID) {
			return fmt.Errorf("%s: invalid zone id %q (must be non-empty ASCII, no whitespace)", zonePath, z.ID)
		}
		if prev, dup := seen[z.ID]; dup {
			return fmt.Errorf("%s: duplicate zone id %q (also at %s/zone.yaml)", zonePath, z.ID, prev.SourceDir)
		}
		if z.ResetMode != "" && !validZoneResetModes[z.ResetMode] {
			return fmt.Errorf("%s: zone %q has invalid reset_mode %q (want always|empty|never)",
				zonePath, z.ID, z.ResetMode)
		}
		if z.ResetIntervalS < 0 {
			return fmt.Errorf("%s: zone %q has negative reset_interval_s %d",
				zonePath, z.ID, z.ResetIntervalS)
		}
		if z.LevelRange != nil {
			if z.LevelRange.Min < 1 {
				return fmt.Errorf("%s: zone %q level_range.min %d < 1",
					zonePath, z.ID, z.LevelRange.Min)
			}
			if z.LevelRange.Max < z.LevelRange.Min {
				return fmt.Errorf("%s: zone %q level_range.max %d < min %d",
					zonePath, z.ID, z.LevelRange.Max, z.LevelRange.Min)
			}
		}
		seen[z.ID] = z
	}
	return nil
}

var validZoneResetModes = map[string]bool{
	"always": true,
	"empty":  true,
	"never":  true,
}

func validateRoomIDs(rooms []Room) error {
	seen := make(map[string]Room, len(rooms))
	for _, r := range rooms {
		if !validExternalID(r.ID) {
			return fmt.Errorf("%s:%d: invalid room id %q (must be non-empty ASCII, no whitespace)", r.SourceFile, r.Line, r.ID)
		}
		if prev, dup := seen[r.ID]; dup {
			return fmt.Errorf("%s:%d: duplicate room id %q (also at %s:%d)", r.SourceFile, r.Line, r.ID, prev.SourceFile, prev.Line)
		}
		if r.Sector != "" && !validSectors[repo.Sector(r.Sector)] {
			return fmt.Errorf("%s:%d: room %q has invalid sector %q", r.SourceFile, r.Line, r.ID, r.Sector)
		}
		if err := validateTriggers(r.SourceFile, r.Line, "room "+r.ID, r.Triggers); err != nil {
			return err
		}
		seen[r.ID] = r
	}
	return nil
}

var validSectors = map[repo.Sector]bool{
	repo.SectorCity:        true,
	repo.SectorForest:      true,
	repo.SectorField:       true,
	repo.SectorHills:       true,
	repo.SectorMountain:    true,
	repo.SectorDesert:      true,
	repo.SectorWater:       true,
	repo.SectorUnderwater:  true,
	repo.SectorAir:         true,
	repo.SectorUnderground: true,
	// Migration 0025 (WoT terrain extensions). Keep in lock-step with
	// the rooms.sector CHECK constraint and repo.Sector constants.
	repo.SectorBlight:   true,
	repo.SectorWaste:    true,
	repo.SectorStedding: true,
	repo.SectorSwamp:    true,
}

func validateStarter(rooms []Room) error {
	var starters []Room
	for _, r := range rooms {
		if r.Starter {
			starters = append(starters, r)
		}
	}
	switch len(starters) {
	case 0:
		return fmt.Errorf("no room marked `starter: true` — exactly one is required")
	case 1:
		return nil
	default:
		first := starters[0]
		second := starters[1]
		return fmt.Errorf("%s:%d: multiple rooms marked `starter: true` (also %s:%d)",
			first.SourceFile, first.Line, second.SourceFile, second.Line)
	}
}

var validDirections = map[string]bool{
	repo.DirNorth:     true,
	repo.DirSouth:     true,
	repo.DirEast:      true,
	repo.DirWest:      true,
	repo.DirUp:        true,
	repo.DirDown:      true,
	repo.DirNortheast: true,
	repo.DirNorthwest: true,
	repo.DirSoutheast: true,
	repo.DirSouthwest: true,
}

func validateExits(rooms []Room) error {
	knownRooms := make(map[string]bool, len(rooms))
	for _, r := range rooms {
		knownRooms[r.ID] = true
	}
	for _, r := range rooms {
		seenDir := make(map[string]bool, len(r.Exits))
		for dir, exit := range r.Exits {
			if !validDirections[dir] {
				return fmt.Errorf("%s:%d: room %q has invalid exit direction %q (want one of n/s/e/w/u/d/ne/nw/se/sw)",
					r.SourceFile, r.Line, r.ID, dir)
			}
			if seenDir[dir] {
				// yaml.v3 maps would already collapse duplicate keys,
				// so this is a paranoia check.
				return fmt.Errorf("%s:%d: room %q has duplicate exit %q", r.SourceFile, r.Line, r.ID, dir)
			}
			seenDir[dir] = true
			if exit.To == "" {
				return fmt.Errorf("%s:%d: room %q exit %q has no `to` target",
					r.SourceFile, r.Line, r.ID, dir)
			}
			if !knownRooms[exit.To] {
				return fmt.Errorf("%s:%d: room %q exit %q points to unknown room %q",
					r.SourceFile, r.Line, r.ID, dir, exit.To)
			}
			if exit.LockDifficulty < 0 {
				return fmt.Errorf("%s:%d: room %q exit %q has negative lock difficulty %d",
					r.SourceFile, r.Line, r.ID, dir, exit.LockDifficulty)
			}
		}
	}
	return nil
}

func validateItems(items []Item, rooms []Room) error {
	known := roomSet(rooms)
	seen := make(map[string]Item, len(items))
	for _, it := range items {
		if !validExternalID(it.ID) {
			return fmt.Errorf("%s:%d: invalid item id %q", it.SourceFile, it.Line, it.ID)
		}
		if prev, dup := seen[it.ID]; dup {
			return fmt.Errorf("%s:%d: duplicate item id %q (also at %s:%d)",
				it.SourceFile, it.Line, it.ID, prev.SourceFile, prev.Line)
		}
		seen[it.ID] = it
		if it.Name == "" {
			return fmt.Errorf("%s:%d: item %q has empty name", it.SourceFile, it.Line, it.ID)
		}
		if it.Room == "" {
			return fmt.Errorf("%s:%d: item %q has no room", it.SourceFile, it.Line, it.ID)
		}
		if !known[it.Room] {
			return fmt.Errorf("%s:%d: item %q references unknown room %q",
				it.SourceFile, it.Line, it.ID, it.Room)
		}
		if it.Weight < 0 {
			return fmt.Errorf("%s:%d: item %q has negative weight %g",
				it.SourceFile, it.Line, it.ID, it.Weight)
		}
		if it.Type != "" && !repo.ItemType(it.Type).IsValid() {
			return fmt.Errorf("%s:%d: item %q has unknown type %q",
				it.SourceFile, it.Line, it.ID, it.Type)
		}
		if it.Quality != "" && !repo.ItemQuality(it.Quality).IsValid() {
			return fmt.Errorf("%s:%d: item %q has unknown quality %q",
				it.SourceFile, it.Line, it.ID, it.Quality)
		}
		for _, f := range it.Flags {
			if _, ok := itemFlagByName[f]; !ok {
				return fmt.Errorf("%s:%d: item %q has unknown flag %q",
					it.SourceFile, it.Line, it.ID, f)
			}
		}
		if _, err := decodeItemValue(it.Value); err != nil {
			return fmt.Errorf("%s:%d: item %q: %w", it.SourceFile, it.Line, it.ID, err)
		}
		if _, err := convertItemStats(it); err != nil {
			return fmt.Errorf("%s:%d: item %q: %w", it.SourceFile, it.Line, it.ID, err)
		}
	}
	return nil
}

func validateMobs(mobs []Mob, rooms []Room, items []Item) error {
	known := roomSet(rooms)
	itemIDs := make(map[string]bool, len(items))
	for _, it := range items {
		itemIDs[it.ID] = true
	}
	seen := make(map[string]Mob, len(mobs))
	for _, m := range mobs {
		if !validExternalID(m.ID) {
			return fmt.Errorf("%s:%d: invalid mob id %q", m.SourceFile, m.Line, m.ID)
		}
		if prev, dup := seen[m.ID]; dup {
			return fmt.Errorf("%s:%d: duplicate mob id %q (also at %s:%d)",
				m.SourceFile, m.Line, m.ID, prev.SourceFile, prev.Line)
		}
		seen[m.ID] = m
		if m.Name == "" {
			return fmt.Errorf("%s:%d: mob %q has empty name", m.SourceFile, m.Line, m.ID)
		}
		if m.Room == "" {
			return fmt.Errorf("%s:%d: mob %q has no room", m.SourceFile, m.Line, m.ID)
		}
		if !known[m.Room] {
			return fmt.Errorf("%s:%d: mob %q references unknown room %q",
				m.SourceFile, m.Line, m.ID, m.Room)
		}
		if m.Shop != nil {
			if err := validateShop(m, itemIDs); err != nil {
				return err
			}
		}
		if m.Banker != nil {
			if err := validateBanker(m); err != nil {
				return err
			}
		}
		if m.Trainer != nil {
			if err := validateTrainer(m); err != nil {
				return err
			}
		}
		if m.WeaveTeacher != nil {
			if err := validateWeaveTeacher(m); err != nil {
				return err
			}
		}
		if err := validateTriggers(m.SourceFile, m.Line, "mob "+m.ID, m.Triggers); err != nil {
			return err
		}
		if m.Dialogue != nil {
			if err := checkDialogueDupes(m.Dialogue); err != nil {
				return fmt.Errorf("%s:%d: mob %s dialogue: %w",
					m.SourceFile, m.Line, m.ID, err)
			}
			if err := dialogue.Validate(decodeDialogueTree(m.Dialogue)); err != nil {
				return fmt.Errorf("%s:%d: mob %s dialogue: %w",
					m.SourceFile, m.Line, m.ID, err)
			}
		}
	}
	return nil
}

// validateTriggers checks every entry in a `triggers:` block. event +
// action must be non-empty and event must be one of the allow-listed
// names. Payload must be decodable to JSON when present.
func validateTriggers(file string, line int, owner string, triggers []TriggerDecl) error {
	for i, td := range triggers {
		idx := i + 1
		if td.Event == "" {
			return fmt.Errorf("%s:%d: %s trigger #%d: event is empty",
				file, line, owner, idx)
		}
		if !repo.ValidTriggerEvent(repo.TriggerEvent(td.Event)) {
			return fmt.Errorf("%s:%d: %s trigger #%d: unknown event %q "+
				"(must be on_enter|on_say|on_attack|on_death|on_tick)",
				file, line, owner, idx, td.Event)
		}
		if strings.TrimSpace(td.Action) == "" {
			return fmt.Errorf("%s:%d: %s trigger #%d: action is empty",
				file, line, owner, idx)
		}
		if _, err := marshalTriggerPayload(td.Payload); err != nil {
			return fmt.Errorf("%s:%d: %s trigger #%d: payload: %w",
				file, line, owner, idx, err)
		}
	}
	return nil
}

// marshalTriggerPayload converts a YAML payload node into the compact
// JSON stored in triggers.payload. A zero / nil node returns "{}".
// Shared between validation and the loader's INSERT path.
//
// Payloads MUST be YAML mappings — a scalar (e.g. `payload: "hello"`)
// or sequence is rejected here so the failure surfaces at boot with
// a builder-friendly file:line, instead of at fire time as a silent
// JSON-unmarshal mismatch in the action handler.
func marshalTriggerPayload(node yaml.Node) (string, error) {
	if node.Kind == 0 {
		return "{}", nil
	}
	if node.Kind != yaml.MappingNode {
		return "", fmt.Errorf("payload must be a mapping, got %s", yamlKindName(node.Kind))
	}
	var v interface{}
	if err := node.Decode(&v); err != nil {
		return "", fmt.Errorf("decode yaml: %w", err)
	}
	if v == nil {
		return "{}", nil
	}
	v = normalizeYAMLForJSON(v)
	out, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}
	return string(out), nil
}

// yamlKindName renders a yaml.Kind for human-friendly error messages.
func yamlKindName(k yaml.Kind) string {
	switch k {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	}
	return "unknown"
}

// normalizeYAMLForJSON converts any map[interface{}]interface{} the
// YAML decoder produces into map[string]interface{} so encoding/json
// can serialize it. Recursive.
func normalizeYAMLForJSON(v interface{}) interface{} {
	switch t := v.(type) {
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, vv := range t {
			out[fmt.Sprintf("%v", k)] = normalizeYAMLForJSON(vv)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, vv := range t {
			out[k] = normalizeYAMLForJSON(vv)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, vv := range t {
			out[i] = normalizeYAMLForJSON(vv)
		}
		return out
	}
	return v
}

// validateTrainer checks the optional `trainer:` block: class must be
// a non-empty external-id (Phase E #23). Cross-referencing against the
// chargen catalog happens at the cmd-layer `train` verb so a content
// swap of the catalog doesn't require a DB migration.
func validateTrainer(m Mob) error {
	if m.Trainer.Class == "" {
		return fmt.Errorf("%s:%d: mob %q trainer.class is empty",
			m.SourceFile, m.Line, m.ID)
	}
	if !validExternalID(m.Trainer.Class) {
		return fmt.Errorf("%s:%d: mob %q trainer.class %q has invalid format",
			m.SourceFile, m.Line, m.ID, m.Trainer.Class)
	}
	return nil
}

// validateWeaveTeacher checks the optional `weave_teacher:` block:
// max_level_taught in [0, 9] and every affinity_filter entry is one
// of the five Power names. Phase E #28.
func validateWeaveTeacher(m Mob) error {
	if m.WeaveTeacher.MaxLevelTaught < 0 || m.WeaveTeacher.MaxLevelTaught > 9 {
		return fmt.Errorf("%s:%d: mob %q weave_teacher.max_level_taught %d out of range [0,9]",
			m.SourceFile, m.Line, m.ID, m.WeaveTeacher.MaxLevelTaught)
	}
	for _, p := range m.WeaveTeacher.AffinityFilter {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "air", "earth", "fire", "water", "spirit":
		default:
			return fmt.Errorf("%s:%d: mob %q weave_teacher.affinity_filter %q is not a Power",
				m.SourceFile, m.Line, m.ID, p)
		}
	}
	return nil
}

// validateBanker checks the optional `banker:` block: hours are 0..23.
// The runtime treats OpenHour == CloseHour as the always-open sentinel
// (default 0/0 → 24h banker). Same range as shops for consistency.
func validateBanker(m Mob) error {
	if h := m.Banker.OpenHour; h != nil && (*h < 0 || *h > 23) {
		return fmt.Errorf("%s:%d: mob %q banker.open_hour %d out of range [0,23]",
			m.SourceFile, m.Line, m.ID, *h)
	}
	if h := m.Banker.CloseHour; h != nil && (*h < 0 || *h > 23) {
		return fmt.Errorf("%s:%d: mob %q banker.close_hour %d out of range [0,23]",
			m.SourceFile, m.Line, m.ID, *h)
	}
	return nil
}

// validateShop checks the optional `shop:` block: every BuyTypes entry
// is a known ItemType, hours are 0..23, and every stock line points at
// an item template defined elsewhere in the world. Stock qty/qty_max
// must agree on the infinite sentinel (both -1 or both >= 0).
func validateShop(m Mob, itemIDs map[string]bool) error {
	for _, raw := range m.Shop.BuyTypes {
		if !repo.ItemType(raw).IsValid() {
			return fmt.Errorf("%s:%d: mob %q shop.buy_types contains unknown item type %q",
				m.SourceFile, m.Line, m.ID, raw)
		}
	}
	if h := m.Shop.OpenHour; h != nil && (*h < 0 || *h > 23) {
		return fmt.Errorf("%s:%d: mob %q shop.open_hour %d out of range [0,23]",
			m.SourceFile, m.Line, m.ID, *h)
	}
	if h := m.Shop.CloseHour; h != nil && (*h < 0 || *h > 23) {
		return fmt.Errorf("%s:%d: mob %q shop.close_hour %d out of range [0,23]",
			m.SourceFile, m.Line, m.ID, *h)
	}
	for _, line := range m.Shop.Stock {
		if line.Item == "" {
			return fmt.Errorf("%s:%d: mob %q shop.stock entry missing `item`",
				m.SourceFile, m.Line, m.ID)
		}
		if !itemIDs[line.Item] {
			return fmt.Errorf("%s:%d: mob %q shop.stock references unknown item %q",
				m.SourceFile, m.Line, m.ID, line.Item)
		}
		if (line.Qty < 0) != (line.QtyMax < 0) {
			return fmt.Errorf("%s:%d: mob %q shop.stock %q: qty=%d and qty_max=%d disagree on infinite sentinel (both must be < 0 or both >= 0)",
				m.SourceFile, m.Line, m.ID, line.Item, line.Qty, line.QtyMax)
		}
		if line.QtyMax == 0 {
			return fmt.Errorf("%s:%d: mob %q shop.stock %q: qty_max=0 means no stock and no restock — use qty=-1 qty_max=-1 for infinite, or qty_max>=1 for a real cap",
				m.SourceFile, m.Line, m.ID, line.Item)
		}
		if line.QtyMax >= 0 && line.Qty > line.QtyMax {
			return fmt.Errorf("%s:%d: mob %q shop.stock %q: qty=%d exceeds qty_max=%d",
				m.SourceFile, m.Line, m.ID, line.Item, line.Qty, line.QtyMax)
		}
	}
	return nil
}

func roomSet(rooms []Room) map[string]bool {
	s := make(map[string]bool, len(rooms))
	for _, r := range rooms {
		s[r.ID] = true
	}
	return s
}
