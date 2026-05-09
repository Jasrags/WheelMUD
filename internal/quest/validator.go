package quest

import (
	"errors"
	"fmt"
)

// ErrInvalidCatalog is the canonical typed error returned by Validate.
// Callers wrap with their own context (file path) before surfacing.
var ErrInvalidCatalog = errors.New("invalid quest catalog")

// ValidateRefs is the optional cross-reference validator: caller passes
// the set of known mob_template ExternalIDs and room ExternalIDs from
// the world loader. A quest referencing an unknown mob or room ID
// fails the boot. nil sets disable the check (useful for unit tests).
type RefSets struct {
	Mobs  map[string]bool
	Rooms map[string]bool
}

// Validate enforces V1 invariants on a parsed Catalog:
//
//   - At least one quest entry.
//   - Every quest has a non-empty ID, Name, and at least one Step.
//   - Quest IDs are unique (Load already deduplicates by map key, so
//     the loader catches dupes earlier — this is defense in depth).
//   - Every Step has a known Kind, the kind-specific required fields,
//     and a non-empty Prompt.
//   - StepKillN.Count > 0.
//
// If refs is non-nil, also enforces:
//
//   - StepTalkTo.Mob, StepKillN.Mob exist in refs.Mobs.
//   - StepReachRoom.Room exists in refs.Rooms.
//   - Quest.GiverMob, when non-empty, exists in refs.Mobs.
//
// Cross-ref validation is opt-in so unit tests can validate a catalog
// without standing up a world; production runs cross-ref at boot.
func Validate(cat *Catalog, refs *RefSets) error {
	if cat == nil {
		return fmt.Errorf("%w: nil catalog", ErrInvalidCatalog)
	}
	// An empty catalog is valid — quests are content, and a fresh
	// deploy is allowed to ship with none authored yet. The engine
	// + verb are no-ops in that case.
	if len(cat.ByID) == 0 {
		return nil
	}
	for id, q := range cat.ByID {
		if q == nil {
			return fmt.Errorf("%w: quest %q is nil", ErrInvalidCatalog, id)
		}
		if q.ID == "" {
			return fmt.Errorf("%w: quest %q has empty ID", ErrInvalidCatalog, id)
		}
		if q.ID != id {
			return fmt.Errorf("%w: quest map key %q does not match Quest.ID %q", ErrInvalidCatalog, id, q.ID)
		}
		if q.Name == "" {
			return fmt.Errorf("%w: quest %q has empty Name", ErrInvalidCatalog, id)
		}
		if len(q.Steps) == 0 {
			return fmt.Errorf("%w: quest %q has no steps", ErrInvalidCatalog, id)
		}
		if refs != nil && q.GiverMob != "" && !refs.Mobs[q.GiverMob] {
			return fmt.Errorf("%w: quest %q giver_mob %q is not a known mob_template",
				ErrInvalidCatalog, id, q.GiverMob)
		}
		for i, s := range q.Steps {
			if err := validateStep(s, refs); err != nil {
				return fmt.Errorf("%w: quest %q step[%d]: %s", ErrInvalidCatalog, id, i, err.Error())
			}
		}
	}
	return nil
}

func validateStep(s Step, refs *RefSets) error {
	if s.Prompt == "" {
		return fmt.Errorf("prompt is empty")
	}
	switch s.Kind {
	case StepTalkTo:
		if s.Mob == "" {
			return fmt.Errorf("talk_to requires mob")
		}
		if refs != nil && !refs.Mobs[s.Mob] {
			return fmt.Errorf("talk_to mob %q is not a known mob_template", s.Mob)
		}
	case StepKillN:
		if s.Mob == "" {
			return fmt.Errorf("kill_n requires mob")
		}
		if s.Count <= 0 {
			return fmt.Errorf("kill_n requires count > 0 (got %d)", s.Count)
		}
		if refs != nil && !refs.Mobs[s.Mob] {
			return fmt.Errorf("kill_n mob %q is not a known mob_template", s.Mob)
		}
	case StepReachRoom:
		if s.Room == "" {
			return fmt.Errorf("reach_room requires room")
		}
		if refs != nil && !refs.Rooms[s.Room] {
			return fmt.Errorf("reach_room room %q is not a known room", s.Room)
		}
	default:
		return fmt.Errorf("unknown step kind %q", s.Kind)
	}
	return nil
}
