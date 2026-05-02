package world

import (
	"fmt"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// validate runs every cross-reference + format check against w. It is
// strict and fail-fast — the first error found is returned, so the
// loader never produces a partial DB load. Errors are formatted with
// the originating file:line so a builder can jump straight to the YAML.
func validate(w *World) error {
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
	if err := validateMobs(w.Mobs, w.Rooms); err != nil {
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

func validateMobs(mobs []Mob, rooms []Room) error {
	known := roomSet(rooms)
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
