package world

import (
	"fmt"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// ItemSpec is the loader's recreate-this-item recipe surfaced to the
// ZoneResetter. One spec per YAML-authored item. The spec carries
// the room external_id (not the int id) because the zone reset path
// runs long after boot, when the room id is known via repo lookup.
//
// The spec is built once per boot during YAML parse + validate (even
// when the DB already has rows so the loader's insert path
// short-circuits) so the resetter has a stable in-memory anchor list
// for the lifetime of the process.
type ItemSpec struct {
	ZoneExternalID string
	RoomExternalID string
	Item           repo.Item // ready to feed to ItemRepo.Create except for RoomID
}

// LoadedWorld is the public result of LoadAndSync. ItemSpecsByZone
// keys by zone external_id so callers can correlate against the
// in-memory zone map; the resetter joins this against the live
// `repo.Zone.ExternalID` it pulls from the DB.
type LoadedWorld struct {
	ItemSpecsByZone map[string][]ItemSpec
}

// buildItemSpec converts a YAML-decoded Item entry plus its zone
// external_id into a recipe the resetter can replay. Returns an
// error if the item's stat block fails the same conversions
// insertItems performs. Validation has already passed at this point,
// so any error here is a loader bug, not a builder mistake.
func buildItemSpec(it Item, zoneExternalID string) (ItemSpec, error) {
	t := repo.ItemType(it.Type)
	if t == "" {
		t = repo.ItemTypeTrash
	}
	q := repo.ItemQuality(it.Quality)
	if q == "" {
		q = repo.QualityNormal
	}
	value, err := decodeItemValue(it.Value)
	if err != nil {
		return ItemSpec{}, fmt.Errorf("item %q: %w", it.ID, err)
	}
	stats, err := convertItemStats(it)
	if err != nil {
		return ItemSpec{}, fmt.Errorf("item %q: %w", it.ID, err)
	}
	flags := decodeItemFlags(it.Flags)
	return ItemSpec{
		ZoneExternalID: zoneExternalID,
		RoomExternalID: it.Room,
		Item: repo.Item{
			ExternalID: it.ID,
			Name:       it.Name,
			NameLower:  strings.ToLower(it.Name),
			ShortDesc:  it.Short,
			Type:       t,
			Weight:     it.Weight,
			Value:      value,
			Quality:    q,
			Flags:      flags,
			Stats:      stats,
		},
	}, nil
}

// buildItemSpecs collects one ItemSpec per parsed item, keyed by
// zone external_id. The room→zone map is built from the parsed
// rooms, since YAML items reference rooms by external_id and rooms
// reference zones by external_id.
func buildItemSpecs(w *World) (map[string][]ItemSpec, error) {
	roomZone := make(map[string]string, len(w.Rooms))
	for _, r := range w.Rooms {
		roomZone[r.ID] = r.ZoneExternalID
	}
	out := make(map[string][]ItemSpec)
	for _, it := range w.Items {
		zoneExt, ok := roomZone[it.Room]
		if !ok {
			// validate() already caught this; defensively keep the
			// loader from panicking on a stale parse.
			return nil, fmt.Errorf("item %q references unknown room %q", it.ID, it.Room)
		}
		spec, err := buildItemSpec(it, zoneExt)
		if err != nil {
			return nil, err
		}
		out[zoneExt] = append(out[zoneExt], spec)
	}
	return out, nil
}
