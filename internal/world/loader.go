package world

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/dialogue"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// LoadAndSync reads YAML zone folders from src, validates the world,
// and performs an additive resync against the DB on every boot: new
// zones / rooms / exits / items / mob_templates that exist in YAML
// but not yet in the DB get inserted; existing rows are left exactly
// as they are. The returned LoadedWorld always carries the in-memory
// recipes ZoneResetter consumes regardless of how many rows landed.
//
// Resync semantics (strictly additive — no updates, no deletes):
//
//   - A YAML edit that renames an existing row's `name` is INVISIBLE
//     to the resync. The DB row's name stays as it was at first load.
//     Operators who want the rename must wipe the row first.
//   - A YAML row that disappears stays in the DB. Operator-driven GC.
//   - For mob_templates: if the template's external_id already
//     exists, the whole bundle is skipped (template + initial
//     instance + shop / banker / trainer / weave_teacher / dialogue
//     / triggers). Refreshing aux blocks would either stomp operator
//     edits or duplicate UNIQUE rows.
//   - If a pre-existing starter sits at id=1 and YAML declares a
//     different starter, the YAML's starter is inserted as a regular
//     auto-increment row. First-load starter wins.
//
// All inserts happen in a single transaction so a partial failure
// rolls back to the pre-resync state rather than a half-loaded DB.
//
// The probe (SELECTs for existing rows) and subsequent inserts run
// in the same transaction, so concurrent boots remain safe modulo
// SQLite's BEGIN-level locking. LoadAndSync should still be called
// once per process at boot; if it ever becomes an admin endpoint,
// guard with an application-level mutex on top of the transaction.
func LoadAndSync(ctx context.Context, db *sql.DB, src fs.FS) (LoadedWorld, error) {
	world, err := parseWorld(src)
	if err != nil {
		return LoadedWorld{}, fmt.Errorf("world: parse: %w", err)
	}
	if err := validate(world); err != nil {
		return LoadedWorld{}, fmt.Errorf("world: validate: %w", err)
	}

	started := time.Now()
	summary, err := resyncWorld(ctx, db, world)
	if err != nil {
		return LoadedWorld{}, fmt.Errorf("world: resync: %w", err)
	}
	slog.Info("world: resync complete",
		"zones_new", summary.zones,
		"rooms_new", summary.rooms,
		"exits_new", summary.exits,
		"items_new", summary.items,
		"mobs_new", summary.mobs,
		"yaml_zones", len(world.Zones),
		"yaml_rooms", len(world.Rooms),
		"yaml_items", len(world.Items),
		"yaml_mobs", len(world.Mobs),
		"elapsed", time.Since(started),
	)

	itemSpecs, err := buildItemSpecs(world)
	if err != nil {
		return LoadedWorld{}, fmt.Errorf("world: build item specs: %w", err)
	}
	return LoadedWorld{ItemSpecsByZone: itemSpecs}, nil
}

// resyncSummary is the per-table count of newly-inserted rows. Zero
// across the board means the DB was already in sync with the YAML.
type resyncSummary struct {
	zones int
	rooms int
	exits int
	items int
	mobs  int
}

// parseWorld walks src for `*/zone.yaml` and parses each matched zone.
// Each zone contributes rooms / items / mobs to the combined World.
func parseWorld(src fs.FS) (*World, error) {
	zoneDirs, err := findZoneDirs(src)
	if err != nil {
		return nil, err
	}
	if len(zoneDirs) == 0 {
		return nil, errors.New("no zone.yaml files found under source filesystem")
	}

	w := &World{}
	for _, dir := range zoneDirs {
		zone, rooms, items, mobs, err := parseZone(src, dir)
		if err != nil {
			return nil, err
		}
		w.Zones = append(w.Zones, zone)
		w.Rooms = append(w.Rooms, rooms...)
		w.Items = append(w.Items, items...)
		w.Mobs = append(w.Mobs, mobs...)
	}
	return w, nil
}

// findZoneDirs returns every directory under src that contains a
// `zone.yaml`, sorted lexically so loads are deterministic.
func findZoneDirs(src fs.FS) ([]string, error) {
	var dirs []string
	err := fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "zone.yaml" {
			return nil
		}
		// p is like "starter/zone.yaml" — directory is everything up to
		// the last slash. fs.FS uses forward slashes regardless of OS.
		for i := len(p) - 1; i >= 0; i-- {
			if p[i] == '/' {
				dirs = append(dirs, p[:i])
				return nil
			}
		}
		dirs = append(dirs, ".")
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk world fs: %w", err)
	}
	sort.Strings(dirs)
	return dirs, nil
}

// resyncWorld writes the new (YAML-but-not-yet-in-DB) world rows
// inside a single transaction and returns the per-table count of
// inserts. Existing rows are skipped via per-table pre-load probes.
//
// The starter room is forced to id=1 only when id=1 isn't already
// occupied; see insertRooms for the starterOccupied logic.
func resyncWorld(ctx context.Context, db *sql.DB, w *World) (resyncSummary, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return resyncSummary{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var s resyncSummary

	zoneIDs, nZones, err := insertZones(ctx, tx, w.Zones)
	if err != nil {
		return s, err
	}
	s.zones = nZones

	roomIDs, roomZones, nRooms, err := insertRooms(ctx, tx, w.Rooms, zoneIDs)
	if err != nil {
		return s, err
	}
	s.rooms = nRooms

	nExits, err := insertExits(ctx, tx, w.Rooms, roomIDs)
	if err != nil {
		return s, err
	}
	s.exits = nExits

	nItems, err := insertItems(ctx, tx, w.Items, roomIDs)
	if err != nil {
		return s, err
	}
	s.items = nItems

	// roomZones came back from insertRooms pre-loaded from DB and
	// extended with newly-inserted rooms. Mobs may reference rooms
	// that came from EITHER source — insertRooms returned the union
	// of both, so we feed it directly to insertMobs.
	roomAdjacency := buildRoomAdjacency(w.Rooms)
	nMobs, err := insertMobs(ctx, tx, w.Mobs, roomIDs, roomZones, roomAdjacency)
	if err != nil {
		return s, err
	}
	s.mobs = nMobs

	if err := tx.Commit(); err != nil {
		return s, fmt.Errorf("commit: %w", err)
	}
	return s, nil
}

// insertZones writes every zone row and returns a map from
// external_id → int id so insertRooms can stamp rooms.zone_id without
// re-querying. Defaults are applied here (not in the YAML struct) so
// authoring stays terse and the schema's documented defaults remain
// the single source of truth: builder="", levels=1..60,
// reset_interval_s=600, reset_mode="empty", climate="", ambient=[].
//
// Validation has already proved zone external_ids are unique and the
// reset_mode is one of the known values, so the only failure path
// here is a transport-level driver error.
func insertZones(ctx context.Context, tx *sql.Tx, zones []Zone) (map[string]int64, int, error) {
	out, err := loadExistingZoneIDs(ctx, tx)
	if err != nil {
		return nil, 0, fmt.Errorf("preload zones: %w", err)
	}
	inserted := 0
	for _, z := range zones {
		if _, exists := out[z.ID]; exists {
			continue
		}
		minLevel, maxLevel := 1, 60
		if z.LevelRange != nil {
			minLevel, maxLevel = z.LevelRange.Min, z.LevelRange.Max
		}
		resetInterval := z.ResetIntervalS
		if resetInterval == 0 {
			resetInterval = 600
		}
		resetMode := z.ResetMode
		if resetMode == "" {
			resetMode = string(repo.ZoneResetEmpty)
		}
		ambientJSON := "[]"
		if len(z.Ambient) > 0 {
			raw, err := json.Marshal(z.Ambient)
			if err != nil {
				// Marshal of []string can't fail in practice;
				// fall back to "[]" rather than panic.
				ambientJSON = "[]"
			} else {
				ambientJSON = string(raw)
			}
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO zones(external_id, name, builder,
				min_level, max_level, reset_interval_s, reset_mode,
				climate, ambient_json)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			z.ID, z.Name, z.Builder,
			minLevel, maxLevel, resetInterval, resetMode,
			z.Climate, ambientJSON,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("insert zone %q: %w", z.ID, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, 0, fmt.Errorf("last insert id for zone %q: %w", z.ID, err)
		}
		out[z.ID] = id
		inserted++
	}
	return out, inserted, nil
}

// loadExistingZoneIDs pre-populates the resync's external_id → id map
// from the DB. Resync uses this so the per-row insert loop can skip
// rows that already exist and downstream insertRooms can still
// resolve every room's owning zone — whether the zone is brand new
// or has been in the DB for months.
func loadExistingZoneIDs(ctx context.Context, tx *sql.Tx) (map[string]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT external_id, id FROM zones`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var ext string
		var id int64
		if err := rows.Scan(&ext, &id); err != nil {
			return nil, err
		}
		out[ext] = id
	}
	return out, rows.Err()
}

// insertRooms inserts every room and returns a map from external_id ->
// int id. The starter room is inserted first with an explicit id=1 so
// repo.StarterRoomID stays accurate.
//
// zoneIDs maps zone external_id → int id; insertRooms looks up each
// room's owning zone (stamped during parseZone) and supplies it as
// rooms.zone_id. A room whose ZoneExternalID is absent from the map
// is a loader bug — fail loud rather than silently writing zone_id=0.
//
// Note: this writes raw SQL into *sql.Tx instead of going through
// repo.RoomRepo.Create. The repo Create takes *sql.DB, so calling it
// from inside a transaction is not possible without either a tx-aware
// variant of the interface or rewriting the loader to not be
// transactional. Atomicity across all four kinds matters more here
// than reuse, so the column list is duplicated. Keep the INSERT
// columns in sync with room_sqlite.go::Create if either changes.
func insertRooms(ctx context.Context, tx *sql.Tx, rooms []Room, zoneIDs map[string]int64) (map[string]int64, map[string]int64, int, error) {
	out, roomZones, err := loadExistingRoomIDs(ctx, tx)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("preload rooms: %w", err)
	}
	triggers := repo.NewSQLiteTriggerRepo(tx)

	resolveZone := func(r Room) (int64, error) {
		id, ok := zoneIDs[r.ZoneExternalID]
		if !ok {
			return 0, fmt.Errorf("room %q references unknown zone %q", r.ID, r.ZoneExternalID)
		}
		return id, nil
	}

	// starterOccupied tracks whether some pre-existing row already
	// sits at repo.StarterRoomID. If yes, the YAML's starter (which
	// the validator guarantees exists exactly once) gets inserted
	// later in the regular auto-increment path rather than forcing
	// id=1 — that would violate the UNIQUE PK and abort the resync.
	// The original starter from the bootstrap load wins; subsequent
	// "starter: true" YAML declarations land as ordinary rooms.
	// Operators who genuinely want to change which room is the
	// starter must wipe the row first.
	starterOccupied := false
	for _, id := range out {
		if id == repo.StarterRoomID {
			starterOccupied = true
			break
		}
	}

	// Validation has already established exactly one starter exists.
	var starterIdx int
	for i, r := range rooms {
		if r.Starter {
			starterIdx = i
			break
		}
	}

	inserted := 0

	// If the starter slot is unoccupied AND the YAML starter isn't
	// already in the DB, insert it FIRST with explicit id=1. Doing
	// this ahead of any auto-increment insert is load-bearing: if a
	// non-starter room grabs id=1 via auto-increment first, the later
	// explicit `INSERT id=1` violates the UNIQUE PK. (This is what
	// goes wrong when the starter's zone sorts alphabetically AFTER
	// some other zone in the combined world.Rooms slice.)
	starter := rooms[starterIdx]
	if _, exists := out[starter.ID]; !exists && !starterOccupied {
		zoneID, err := resolveZone(starter)
		if err != nil {
			return nil, nil, 0, err
		}
		cols, vals := roomInsertValues(starter, zoneID)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO rooms(id, `+cols+`) VALUES (?, `+repo.Placeholders(len(vals))+`)`,
			append([]any{repo.StarterRoomID}, vals...)...,
		); err != nil {
			return nil, nil, 0, fmt.Errorf("insert starter room %q: %w", starter.ID, err)
		}
		out[starter.ID] = repo.StarterRoomID
		roomZones[starter.ID] = zoneID
		if err := insertRoomTriggers(ctx, triggers, repo.StarterRoomID, starter); err != nil {
			return nil, nil, 0, err
		}
		inserted++
	}

	// Insert the rest (or the starter as a regular auto-increment row
	// when starterOccupied=true and its external_id isn't yet in DB).
	for i, r := range rooms {
		if _, exists := out[r.ID]; exists {
			continue
		}
		// Skip the bootstrap starter slot we already handled above. If
		// starterOccupied is true OR the starter was already in DB, the
		// guard above didn't run and the starter falls through to the
		// regular auto-increment path here.
		if i == starterIdx && !starterOccupied {
			continue
		}
		zoneID, err := resolveZone(r)
		if err != nil {
			return nil, nil, 0, err
		}
		cols, vals := roomInsertValues(r, zoneID)
		res, err := tx.ExecContext(ctx,
			`INSERT INTO rooms(`+cols+`) VALUES (`+repo.Placeholders(len(vals))+`)`,
			vals...,
		)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("insert room %q: %w", r.ID, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, nil, 0, fmt.Errorf("last insert id for room %q: %w", r.ID, err)
		}
		out[r.ID] = id
		roomZones[r.ID] = zoneID
		if err := insertRoomTriggers(ctx, triggers, id, r); err != nil {
			return nil, nil, 0, err
		}
		inserted++
	}
	return out, roomZones, inserted, nil
}

// loadExistingRoomIDs pre-populates both lookup maps the resync
// downstream stages depend on: external_id → int id, AND external_id
// → owning zone_id. Pulling both columns in one SELECT keeps the
// boot probe to a single query and lets us route mob spawn-anchor
// metadata at the DB-canonical zone (rather than at whatever the
// YAML currently declares — which may have drifted).
func loadExistingRoomIDs(ctx context.Context, tx *sql.Tx) (map[string]int64, map[string]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT external_id, id, zone_id FROM rooms`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	ids := map[string]int64{}
	zones := map[string]int64{}
	for rows.Next() {
		var ext string
		var id, zone int64
		if err := rows.Scan(&ext, &id, &zone); err != nil {
			return nil, nil, err
		}
		ids[ext] = id
		zones[ext] = zone
	}
	return ids, zones, rows.Err()
}

// insertRoomTriggers materialises a Room's `triggers:` block as
// triggers rows keyed by owner_kind='room'. Validation has already
// confirmed event/action/payload shape.
func insertRoomTriggers(ctx context.Context, triggers repo.TriggerRepo, roomID int64, r Room) error {
	for i, td := range r.Triggers {
		payload, err := marshalTriggerPayload(td.Payload)
		if err != nil {
			return fmt.Errorf("room %q trigger #%d payload: %w", r.ID, i+1, err)
		}
		if _, err := triggers.Create(ctx, repo.Trigger{
			OwnerKind: repo.TriggerOwnerRoom,
			OwnerID:   roomID,
			Event:     repo.TriggerEvent(td.Event),
			Match:     td.Match,
			Action:    td.Action,
			Payload:   payload,
			Priority:  td.Priority,
		}); err != nil {
			return fmt.Errorf("insert trigger for room %q: %w", r.ID, err)
		}
	}
	return nil
}

// roomInsertValues materializes the column list + values for one room
// row, applying defaults (sector=city, light=DefaultLightLevel for
// non-dark rooms) when the YAML left them blank. zoneID is the
// rooms.zone_id value resolved by insertRooms.
func roomInsertValues(r Room, zoneID int64) (string, []any) {
	sector := r.Sector
	if sector == "" {
		sector = string(repo.SectorCity)
	}
	light := repo.DefaultLightLevel
	if r.LightLevel != nil {
		light = *r.LightLevel
	} else if r.Flags.Dark {
		light = 0
	}
	// Day/night gate (§9): an outdoor room with light=0 and no Dark
	// flag will always render pitch black under the cycle. Defaults
	// (light unset → 100; Dark → 0) avoid this; warn the builder when
	// they've authored an explicit 0 outdoors so the surprise is
	// visible at load time.
	if light == 0 && !r.Flags.Dark && !r.Flags.Indoors {
		isSheltered := sector == string(repo.SectorUnderground) || sector == string(repo.SectorUnderwater)
		if !isSheltered {
			slog.Warn("world: outdoor room with light_level=0 and no dark flag; will render pitch black",
				"room_external_id", r.ID, "sector", sector)
		}
	}
	var x, y, z int
	if r.Coords != nil {
		x, y, z = r.Coords.X, r.Coords.Y, r.Coords.Z
	}
	extraJSON := "{}"
	if len(r.Descriptions) > 0 {
		normalized := make(map[string]string, len(r.Descriptions))
		for k, v := range r.Descriptions {
			normalized[strings.ToLower(strings.TrimSpace(k))] = v
		}
		raw, err := json.Marshal(normalized)
		if err != nil {
			// validate() rejected non-string maps; this should be
			// unreachable, but fall back to "{}" rather than panicking.
			extraJSON = "{}"
		} else {
			extraJSON = string(raw)
		}
	}
	// coords_auto stamps the auto-coord BFS runner's anchor flag (see
	// migration 0026 and repo.CoordsAutoInt). When YAML provides an
	// explicit `coords:` block the room is a builder-authored anchor
	// and the runner must not overwrite it. CoordsAutoInt centralises
	// the bool→int inversion so this raw-SQL path stays in lock-step
	// with repo.Create.
	coordsAuto := repo.CoordsAutoInt(r.Coords != nil)
	cols := `external_id, name, short_desc, long_desc,
		indoors, nopvp, noteleport, dark, silent, peaceful, nomap, bindable,
		sector, light_level, coord_x, coord_y, coord_z, coords_auto,
		extra_descs_json, zone_id`
	vals := []any{
		r.ID, r.Name, r.Short, r.Long,
		repo.BoolToInt(r.Flags.Indoors), repo.BoolToInt(r.Flags.NoPVP),
		repo.BoolToInt(r.Flags.NoTeleport), repo.BoolToInt(r.Flags.Dark),
		repo.BoolToInt(r.Flags.Silent), repo.BoolToInt(r.Flags.Peaceful),
		repo.BoolToInt(r.Flags.NoMap), repo.BoolToInt(r.Flags.Bindable),
		sector, light, x, y, z, coordsAuto, extraJSON,
		zoneID,
	}
	return cols, vals
}

// exitKey is the composite of the exits table's UNIQUE constraint
// (from_room_id, direction). The resync pre-loads the existing set
// so new exits land while existing ones are left undisturbed —
// notably preserving any runtime door state (closed/locked) that
// the OLC editor or AreaReset may have toggled away from the YAML
// authoring.
type exitKey struct {
	from int64
	dir  string
}

func insertExits(ctx context.Context, tx *sql.Tx, rooms []Room, roomIDs map[string]int64) (int, error) {
	existing, err := loadExistingExitKeys(ctx, tx)
	if err != nil {
		return 0, fmt.Errorf("preload exits: %w", err)
	}
	inserted := 0
	for _, r := range rooms {
		from := roomIDs[r.ID]
		// Exits are sorted by direction so insert order is
		// deterministic — useful for tests that assert on row ids.
		dirs := make([]string, 0, len(r.Exits))
		for d := range r.Exits {
			dirs = append(dirs, d)
		}
		sort.Strings(dirs)
		for _, dir := range dirs {
			if existing[exitKey{from: from, dir: dir}] {
				continue
			}
			ex := r.Exits[dir]
			to, ok := roomIDs[ex.To]
			if !ok {
				// validate() already caught this, but defensively.
				return 0, fmt.Errorf("exit from %q dir %q targets unknown room %q", r.ID, dir, ex.To)
			}
			pickable := true // schema default
			if ex.Pickable != nil {
				pickable = *ex.Pickable
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO exits(from_room_id, to_room_id, direction,
					closed, locked, pickable, hidden, nopass,
					key_external_id, lock_difficulty, description,
					authored_closed, authored_locked)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				from, to, dir,
				repo.BoolToInt(ex.Closed), repo.BoolToInt(ex.Locked),
				repo.BoolToInt(pickable), repo.BoolToInt(ex.Hidden),
				repo.BoolToInt(ex.NoPass),
				ex.Key, ex.LockDifficulty, ex.Description,
				// Authored values mirror the YAML closed/locked at load
				// time. ZoneResetter reads these on each AreaReset pass.
				repo.BoolToInt(ex.Closed), repo.BoolToInt(ex.Locked),
			); err != nil {
				return 0, fmt.Errorf("insert exit %q->%q: %w", r.ID, dir, err)
			}
			inserted++
		}
	}
	return inserted, nil
}

// loadExistingExitKeys returns a set of (from_room_id, direction)
// pairs already present in the exits table. Used by the resync to
// skip exits the DB has already seen on prior boots.
func loadExistingExitKeys(ctx context.Context, tx *sql.Tx) (map[exitKey]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT from_room_id, direction FROM exits`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[exitKey]bool{}
	for rows.Next() {
		var k exitKey
		if err := rows.Scan(&k.from, &k.dir); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}

func insertItems(ctx context.Context, tx *sql.Tx, items []Item, roomIDs map[string]int64) (int, error) {
	existing, err := loadExistingItemExternalIDs(ctx, tx)
	if err != nil {
		return 0, fmt.Errorf("preload items: %w", err)
	}
	inserted := 0
	for _, it := range items {
		if existing[it.ID] {
			continue
		}
		roomID, ok := roomIDs[it.Room]
		if !ok || roomID == 0 {
			// Validation has already caught unknown rooms in the
			// in-memory world. Defensive guard against a roomIDs map
			// that doesn't include the target — would otherwise
			// silently insert with room_id=0, which is the original
			// "resolve home room" bug this resync is fixing.
			return 0, fmt.Errorf("item %q targets unknown room %q", it.ID, it.Room)
		}
		// Validation has already proved Type/Quality/Flags/Stats are
		// well-formed, so the conversions below cannot fail in
		// practice — but we still propagate any error rather than
		// panic, since a future loader change might decouple them.
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
			return 0, fmt.Errorf("insert item %q: %w", it.ID, err)
		}
		stats, err := convertItemStats(it)
		if err != nil {
			return 0, fmt.Errorf("insert item %q: %w", it.ID, err)
		}
		statsJSON, err := encodeItemStatsJSON(stats)
		if err != nil {
			return 0, fmt.Errorf("insert item %q: %w", it.ID, err)
		}
		flags := decodeItemFlags(it.Flags)
		// Loader-spawned items always start on a room floor — no owner,
		// no parent container. owner_character_id and parent_item_id
		// stay NULL; the inventory verbs flip them on `get` / `put`.
		// YAML support for nested contents is a future slice; see the
		// container-semantics plan.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO items(external_id, name, name_lower, short_desc, room_id, owner_character_id, parent_item_id,
				type, weight_lbs, value_cp, quality, flags, stats_json, decay_expires_at)
			 VALUES (?, ?, ?, ?, ?, NULL, NULL, ?, ?, ?, ?, ?, ?, NULL)`,
			it.ID, it.Name, strings.ToLower(it.Name), it.Short, roomID,
			string(t), it.Weight, int64(value), string(q),
			int64(flags), statsJSON,
		); err != nil {
			return 0, fmt.Errorf("insert item %q: %w", it.ID, err)
		}
		inserted++
	}
	return inserted, nil
}

// loadExistingItemExternalIDs returns the set of items.external_id
// already in the DB. Used by the resync to skip items the DB
// already knows about — preserving any runtime mutations like
// `owner_character_id` (a player picked it up) or `parent_item_id`
// (it's inside a container now).
func loadExistingItemExternalIDs(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT external_id FROM items`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var ext string
		if err := rows.Scan(&ext); err != nil {
			return nil, err
		}
		out[ext] = true
	}
	return out, rows.Err()
}

// encodeItemStatsJSON marshals the typed stats struct produced by
// convertItemStats to the wire-format the items.stats_json column
// expects. Nil → "{}" so the column NOT NULL contract holds.
func encodeItemStatsJSON(s repo.ItemStats) (string, error) {
	if s == nil {
		return "{}", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("encode stats: %w", err)
	}
	return string(b), nil
}

// insertMobs writes each YAML mob entry as a (mob_template,
// mob_instance) pair: one bespoke template per entry plus a single
// spawn into the named room. This is the v1 loader contract — every
// YAML mob is one-of-a-kind. When a template-and-spawn split YAML
// schema lands, this function fans out to two passes (templates
// first, then spawn references). Until then, builders just author
// `mobs.yaml` as before and the loader manufactures defaults for
// the rest of the Core stat block (Medium humanoid, HP 1, Defense
// 10, ChallengeCode 'A').
// buildRoomAdjacency returns a map from room external_id → set of
// reachable room external_ids via walkable exits at load time.
// Hidden / closed / locked / NoPass exits are excluded so the
// adjacency mirrors the wander tick's exitWalkable gate. Used by
// Phase F #32a slice 1 to validate authored mob paths.
func buildRoomAdjacency(rooms []Room) map[string]map[string]bool {
	adj := make(map[string]map[string]bool, len(rooms))
	for _, r := range rooms {
		set := make(map[string]bool, len(r.Exits))
		for _, ex := range r.Exits {
			if ex.To == "" {
				continue
			}
			if ex.Hidden || ex.Closed || ex.Locked || ex.NoPass {
				continue
			}
			set[ex.To] = true
		}
		adj[r.ID] = set
	}
	return adj
}

// validateMobPath checks the Phase F #32a slice 1 invariants on an
// authored mob path: length >= 2, no duplicate entries, each entry
// resolves to a known room external_id, and each consecutive pair
// (incl. the closed-loop wraparound) is connected by a walkable
// exit. Returns a wrapped error naming the offending entry.
func validateMobPath(mobID string, path []string, roomIDs map[string]int64, adj map[string]map[string]bool) error {
	if len(path) < 2 {
		return fmt.Errorf("mob %q: path must have at least 2 entries, got %d", mobID, len(path))
	}
	seen := make(map[string]bool, len(path))
	for _, ext := range path {
		if _, ok := roomIDs[ext]; !ok {
			return fmt.Errorf("mob %q: path room %q is not a known room external_id", mobID, ext)
		}
		if seen[ext] {
			return fmt.Errorf("mob %q: path contains duplicate room %q", mobID, ext)
		}
		seen[ext] = true
	}
	// Adjacency: each path[i] → path[(i+1)%len] must have a walkable
	// exit, so the closed loop is traversable end-to-end.
	for i := 0; i < len(path); i++ {
		from := path[i]
		to := path[(i+1)%len(path)]
		if !adj[from][to] {
			return fmt.Errorf("mob %q: no walkable exit from %q to %q (path step %d)",
				mobID, from, to, i)
		}
	}
	return nil
}

func insertMobs(ctx context.Context, tx *sql.Tx, mobs []Mob, roomIDs, roomZones map[string]int64, roomAdj map[string]map[string]bool) (int, error) {
	templates := repo.NewSQLiteMobTemplateRepo(tx)
	instances := repo.NewSQLiteMobInstanceRepo(tx)
	shops := repo.NewSQLiteShopRepo(tx)
	bankers := repo.NewSQLiteBankerRepo(tx)
	trainers := repo.NewSQLiteTrainerRepo(tx)
	weaveTeachers := repo.NewSQLiteWeaveTeacherRepo(tx)
	triggers := repo.NewSQLiteTriggerRepo(tx)

	// Pre-load existing template external_ids. A YAML mob whose
	// template is already in the DB skips its entire bundle —
	// template + initial instance + shop/banker/trainer/
	// weave_teacher/dialogue/triggers. Refreshing the auxiliary
	// blocks would either stomp on operator edits or duplicate rows
	// (most aux tables UNIQUE on mob_template_id). To replay
	// authoring changes the operator wipes the template row.
	existingExternals, err := templates.ListExternalIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("preload mob templates: %w", err)
	}
	existing := make(map[string]bool, len(existingExternals))
	for _, ext := range existingExternals {
		existing[ext] = true
	}

	inserted := 0
	for _, m := range mobs {
		if existing[m.ID] {
			continue
		}
		if m.XPValue < 0 {
			return 0, fmt.Errorf("mob %q: xp_value must be >= 0, got %d "+
				"(0 = fall back to challenge_code table)", m.ID, m.XPValue)
		}
		roomID := roomIDs[m.Room]
		wander := creature.DefaultWanderChance
		if m.WanderChance != nil {
			wander = *m.WanderChance
		}
		if len(m.Path) > 0 {
			if err := validateMobPath(m.ID, m.Path, roomIDs, roomAdj); err != nil {
				return 0, err
			}
		}
		// PathRoomIDs is the resolved-at-boot cache. Built here so
		// the wander tick reads internal room IDs without re-resolving
		// per pulse; the persisted external_ids on the column stay
		// the canonical form.
		var pathRoomIDs []int64
		if len(m.Path) > 0 {
			pathRoomIDs = make([]int64, len(m.Path))
			for i, ext := range m.Path {
				pathRoomIDs[i] = roomIDs[ext]
			}
		}
		// Slice-2 sanity: a mob with BOTH a strict path AND a
		// wander_radius is almost certainly a builder mistake — only
		// one branch wins (path), and we'd rather surface the
		// inconsistency at boot than have the radius silently
		// ignored at runtime.
		if len(m.Path) > 0 && m.WanderRadius > 0 {
			return 0, fmt.Errorf("mob %q: cannot set both `path` and `wander_radius` "+
				"(path takes precedence; pick one)", m.ID)
		}
		tpl := creature.MobTemplate{
			ExternalID:    m.ID,
			ChallengeCode: 'A',
			Organization:  "solitary",
			WanderChance:  wander,
			WanderRadius:  m.WanderRadius,
			Path:          append([]string(nil), m.Path...),
			PathRoomIDs:   pathRoomIDs,
			GoldDice:      m.GoldDice,
			XPValue:       m.XPValue,
			ShortDesc:     m.Short,
			Core: creature.Core{
				Name:    m.Name,
				Size:    creature.SizeMedium,
				Type:    creature.TypeHumanoid,
				HPMax:   1,
				Defense: 10,
				Speed:   creature.Speed{BaseFt: 30},
				ReachFt: 5, FaceFt: 5, ThreatFt: 5,
			},
		}
		if m.Dialogue != nil {
			djson, err := buildDialogueJSON(m.Dialogue)
			if err != nil {
				return 0, fmt.Errorf("mob %q dialogue: %w", m.ID, err)
			}
			tpl.DialogueJSON = djson
		}
		created, err := templates.Create(ctx, tpl)
		if err != nil {
			return 0, fmt.Errorf("insert mob template %q: %w", m.ID, err)
		}
		// Stamp the §9 spawn anchor so the §19 Respawner can top up
		// this mob's population on AreaReset ticks. roomZones[m.Room]
		// is 0 for orphan rooms (validation already rejects those),
		// so a zero zone is a loader bug worth surfacing.
		zoneID := roomZones[m.Room]
		if zoneID == 0 {
			return 0, fmt.Errorf("mob %q in room %q: room has no zone", m.ID, m.Room)
		}
		if err := templates.SetSpawnAnchor(ctx, created.ID, zoneID, roomID); err != nil {
			return 0, fmt.Errorf("set spawn anchor for mob %q: %w", m.ID, err)
		}
		created.RespawnZoneResetID = zoneID
		created.HomeRoomID = roomID
		spawn := creature.NewInstanceFromTemplate(created, roomID, 0)
		if _, err := instances.Create(ctx, spawn); err != nil {
			return 0, fmt.Errorf("spawn mob instance %q: %w", m.ID, err)
		}
		if m.Shop != nil {
			if err := insertShop(ctx, shops, created.ID, m); err != nil {
				return 0, fmt.Errorf("insert shop for mob %q: %w", m.ID, err)
			}
		}
		if m.Banker != nil {
			if err := insertBanker(ctx, bankers, created.ID, m); err != nil {
				return 0, fmt.Errorf("insert banker for mob %q: %w", m.ID, err)
			}
		}
		if m.Trainer != nil {
			if err := insertTrainer(ctx, trainers, created.ID, m); err != nil {
				return 0, fmt.Errorf("insert trainer for mob %q: %w", m.ID, err)
			}
		}
		if m.WeaveTeacher != nil {
			if err := insertWeaveTeacher(ctx, weaveTeachers, created.ID, m); err != nil {
				return 0, fmt.Errorf("insert weave teacher for mob %q: %w", m.ID, err)
			}
		}
		if err := insertMobTriggers(ctx, triggers, created.ID, m); err != nil {
			return 0, err
		}
		inserted++
	}
	return inserted, nil
}

// insertMobTriggers materialises a Mob's `triggers:` block as
// triggers rows keyed by owner_kind='mob_template'. Validation has
// already confirmed event/action/payload shape.
func insertMobTriggers(ctx context.Context, triggers repo.TriggerRepo, mobTemplateID int64, m Mob) error {
	for i, td := range m.Triggers {
		payload, err := marshalTriggerPayload(td.Payload)
		if err != nil {
			return fmt.Errorf("mob %q trigger #%d payload: %w", m.ID, i+1, err)
		}
		if _, err := triggers.Create(ctx, repo.Trigger{
			OwnerKind: repo.TriggerOwnerMobTemplate,
			OwnerID:   mobTemplateID,
			Event:     repo.TriggerEvent(td.Event),
			Match:     td.Match,
			Action:    td.Action,
			Payload:   payload,
			Priority:  td.Priority,
		}); err != nil {
			return fmt.Errorf("insert trigger for mob %q: %w", m.ID, err)
		}
	}
	return nil
}

// buildDialogueJSON converts an authored DialogueDecl into the
// canonical dialogue.Tree, runs duplicate-id + Validate checks, and
// returns the compact JSON encoding ready for the
// `mob_templates.dialogue_json` column. Validation runs here so a
// bad tree fails LoadAndSync's transaction before the partial state
// lands in SQLite.
func buildDialogueJSON(d *DialogueDecl) ([]byte, error) {
	if err := checkDialogueDupes(d); err != nil {
		return nil, err
	}
	tree := decodeDialogueTree(d)
	if err := dialogue.Validate(tree); err != nil {
		return nil, err
	}
	out, err := json.Marshal(tree)
	if err != nil {
		return nil, fmt.Errorf("marshal dialogue: %w", err)
	}
	return out, nil
}

// checkDialogueDupes scans the authored Nodes slice for duplicate
// IDs. Required because decodeDialogueTree's slice-to-map step
// silently overwrites duplicates, hiding the typo from
// dialogue.Validate. Run before the slice is collapsed.
func checkDialogueDupes(d *DialogueDecl) error {
	if d == nil {
		return nil
	}
	seen := make(map[string]bool, len(d.Nodes))
	for _, n := range d.Nodes {
		if seen[n.ID] {
			return fmt.Errorf("duplicate dialogue node id %q", n.ID)
		}
		seen[n.ID] = true
	}
	return nil
}

// decodeDialogueTree maps the YAML authoring shape onto the runtime
// dialogue.Tree. Duplicate detection runs in checkDialogueDupes
// before this function, so by the time we collapse the slice into a
// map every key is unique.
func decodeDialogueTree(d *DialogueDecl) *dialogue.Tree {
	if d == nil {
		return nil
	}
	t := &dialogue.Tree{
		Root:  dialogue.NodeID(d.Root),
		Nodes: make(map[dialogue.NodeID]dialogue.Node, len(d.Nodes)),
	}
	for _, n := range d.Nodes {
		responses := make([]dialogue.Response, 0, len(n.Responses))
		for _, r := range n.Responses {
			effects := make([]dialogue.Effect, 0, len(r.Effects))
			for _, e := range r.Effects {
				effects = append(effects, dialogue.Effect{
					Kind: dialogue.EffectKind(e.Kind),
					Args: e.Args,
				})
			}
			responses = append(responses, dialogue.Response{
				Match:   r.Match,
				Reply:   r.Reply,
				Label:   r.Label,
				Next:    dialogue.NodeID(r.Next),
				Effects: effects,
				Show: dialogue.Show{
					RequireFlag: r.Show.RequireFlag,
					ForbidFlag:  r.Show.ForbidFlag,
				},
			})
		}
		t.Nodes[dialogue.NodeID(n.ID)] = dialogue.Node{
			ID:        dialogue.NodeID(n.ID),
			Prompt:    n.Prompt,
			Responses: responses,
		}
	}
	return t
}

// insertTrainer materializes one `trainer:` YAML block into a trainers
// row. Class id has already cleared validateTrainer (non-empty,
// external-id charset).
func insertTrainer(ctx context.Context, trainers repo.TrainerRepo, mobTemplateID int64, m Mob) error {
	cfg := repo.Trainer{MobTemplateID: mobTemplateID, ClassID: m.Trainer.Class}
	if _, err := trainers.Create(ctx, cfg); err != nil {
		return err
	}
	return nil
}

// insertWeaveTeacher materializes one `weave_teacher:` YAML block
// into a weave_teachers row (Phase E #28). The block has already
// cleared validateWeaveTeacher (range + power names).
func insertWeaveTeacher(ctx context.Context, teachers repo.WeaveTeacherRepo, mobTemplateID int64, m Mob) error {
	var aff creature.PowerSet
	for _, p := range m.WeaveTeacher.AffinityFilter {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "air":
			aff |= 1 << creature.PowerAir
		case "earth":
			aff |= 1 << creature.PowerEarth
		case "fire":
			aff |= 1 << creature.PowerFire
		case "water":
			aff |= 1 << creature.PowerWater
		case "spirit":
			aff |= 1 << creature.PowerSpirit
		}
	}
	cfg := repo.WeaveTeacher{
		MobTemplateID:  mobTemplateID,
		MaxLevelTaught: int8(m.WeaveTeacher.MaxLevelTaught),
		AffinityFilter: aff,
	}
	if _, err := teachers.Create(ctx, cfg); err != nil {
		return err
	}
	return nil
}

// insertBanker materializes one `banker:` YAML block into a bankers
// row. Defaults: hours unset → always-open (OpenHour == CloseHour ==
// 0 is the always-open sentinel, same convention as shops).
func insertBanker(ctx context.Context, bankers repo.BankerRepo, mobTemplateID int64, m Mob) error {
	cfg := repo.Banker{MobTemplateID: mobTemplateID}
	if v := m.Banker.OpenHour; v != nil {
		cfg.OpenHour = *v
	}
	if v := m.Banker.CloseHour; v != nil {
		cfg.CloseHour = *v
	}
	if _, err := bankers.Create(ctx, cfg); err != nil {
		return err
	}
	return nil
}

// insertShop materializes one `shop:` YAML block into a shops row plus
// its shop_stock lines. Defaults: SellMarkup=1.0, BuyMarkdown=0.5,
// RestockIntervalS=3600, hours unset → always-open. Validation has
// already cleared types and item refs.
func insertShop(ctx context.Context, shops repo.ShopRepo, mobTemplateID int64, m Mob) error {
	cfg := repo.Shop{
		MobTemplateID:    mobTemplateID,
		SellMarkup:       1.0,
		BuyMarkdown:      0.5,
		RestockIntervalS: 3600,
	}
	for _, t := range m.Shop.BuyTypes {
		cfg.BuyTypes = append(cfg.BuyTypes, repo.ItemType(t))
	}
	if v := m.Shop.SellMarkup; v != nil {
		cfg.SellMarkup = *v
	}
	if v := m.Shop.BuyMarkdown; v != nil {
		cfg.BuyMarkdown = *v
	}
	if v := m.Shop.OpenHour; v != nil {
		cfg.OpenHour = *v
	}
	if v := m.Shop.CloseHour; v != nil {
		cfg.CloseHour = *v
	}
	if v := m.Shop.RestockIntervalS; v != nil {
		cfg.RestockIntervalS = *v
	}
	created, err := shops.Create(ctx, cfg)
	if err != nil {
		return err
	}
	for _, line := range m.Shop.Stock {
		if err := shops.UpsertStock(ctx, repo.ShopStockRow{
			ShopID:         created.ID,
			ItemExternalID: line.Item,
			Qty:            line.Qty,
			QtyMax:         line.QtyMax,
		}); err != nil {
			return fmt.Errorf("stock %q: %w", line.Item, err)
		}
	}
	return nil
}
