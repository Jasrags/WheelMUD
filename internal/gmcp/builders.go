package gmcp

import (
	"sort"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// buildCharName extracts the display name for the Char.Name package.
// Fullname is reserved for an honorific / title surface (e.g. "Bob the
// Apprentice"); we don't have titles in V1, so it mirrors Name.
func buildCharName(c *repo.Character) CharName {
	return CharName{Name: c.Name, Fullname: c.Name}
}

// buildCharVitals projects the four-number HP/stamina snapshot. The
// stamina fields ride along even when StaminaMax is 0 so Mudlet's
// gauge math doesn't divide by zero — the gauge will show "full" in
// that case, which is correct: no stamina pool = no drain.
func buildCharVitals(c *repo.Character) CharVitals {
	return CharVitals{
		HP:    c.Core.HPCurrent,
		MaxHP: c.Core.HPMax,
		SP:    c.Core.StaminaCurrent,
		MaxSP: c.Core.StaminaMax,
	}
}

// buildCharStatus turns the character's identity slice into the
// Mudlet-standard Char.Status payload. Level is the d20 sum of all
// ClassLevels; Class is the dominant entry with an alphabetical
// tie-break so the surfaced label is stable across logins.
func buildCharStatus(c *repo.Character) CharStatus {
	return CharStatus{
		Name:  c.Name,
		Race:  raceDisplayName(c.Race),
		Class: dominantClassName(c.ClassLevels),
		Level: characterLevel(c.ClassLevels),
	}
}

// buildRoomInfo turns a room + its outbound exits into the auto-mapper
// payload Mudlet expects. Zone is the room's zone external id (empty
// when unmapped); Exits keys are the canonical short names — see
// exitShortName for the mapping.
func buildRoomInfo(room repo.Room, exits []repo.Exit, zoneExternalID string) RoomInfo {
	out := RoomInfo{
		Num:   room.ID,
		Name:  room.Name,
		Zone:  zoneExternalID,
		Desc:  room.LongDesc,
		Exits: make(map[string]int64, len(exits)),
	}
	for _, e := range exits {
		short := exitShortName(e.Direction)
		if short == "" {
			continue
		}
		out.Exits[short] = e.ToRoomID
	}
	return out
}

// exitShortName accepts an exit's stored direction code and returns
// the Mudlet-mapper-friendly short form. The DB stores short codes
// already (repo.DirNorth = "n", etc.); we still validate against the
// known set so an unrecognized code is dropped rather than passed
// through unsanitized.
func exitShortName(dir string) string {
	switch dir {
	case repo.DirNorth, repo.DirSouth, repo.DirEast, repo.DirWest,
		repo.DirUp, repo.DirDown,
		repo.DirNortheast, repo.DirNorthwest,
		repo.DirSoutheast, repo.DirSouthwest:
		return dir
	default:
		return ""
	}
}

// characterLevel implements the d20 multiclass convention: total
// character level = sum of class levels.
func characterLevel(levels map[creature.Class]int8) int {
	total := 0
	for _, lvl := range levels {
		total += int(lvl)
	}
	return total
}

// dominantClassName returns the display name of the class with the
// highest level. Ties are broken alphabetically (on the display name)
// so the surfaced label is deterministic across server restarts.
func dominantClassName(levels map[creature.Class]int8) string {
	if len(levels) == 0 {
		return ""
	}
	type entry struct {
		name  string
		level int8
	}
	out := make([]entry, 0, len(levels))
	for c, l := range levels {
		out = append(out, entry{name: classDisplayName(c), level: l})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].level != out[j].level {
			return out[i].level > out[j].level
		}
		return out[i].name < out[j].name
	})
	return out[0].name
}

// classDisplayName maps the creature.Class enum to the player-facing
// label. Kept here (not on the enum) because the enum is content-
// neutral and the labels are MUD-flavor.
func classDisplayName(c creature.Class) string {
	switch c {
	case creature.ClassAlgaiDSiswai:
		return "Algai'd'siswai"
	case creature.ClassArmsman:
		return "Armsman"
	case creature.ClassInitiate:
		return "Initiate"
	case creature.ClassNoble:
		return "Noble"
	case creature.ClassWanderer:
		return "Wanderer"
	case creature.ClassWilder:
		return "Wilder"
	case creature.ClassWoodsman:
		return "Woodsman"
	default:
		return ""
	}
}

// raceDisplayName maps creature.Race to its label.
func raceDisplayName(r creature.Race) string {
	switch r {
	case creature.RaceHuman:
		return "Human"
	case creature.RaceOgier:
		return "Ogier"
	default:
		return ""
	}
}
