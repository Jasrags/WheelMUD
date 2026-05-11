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

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/dialogue"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// LoadAndSync reads YAML zone folders from src, validates the world,
// and populates the rooms / exits / items / mobs tables. The insert
// path short-circuits when the world tables already have rows (boot-
// time only — pick up YAML changes by wiping the DB), but the YAML
// is always parsed + validated so the returned LoadedWorld carries
// the in-memory recipes ZoneResetter consumes.
//
// All inserts happen in a single transaction so a partial failure
// rolls back to an empty world rather than leaving the DB half-loaded.
//
// The "already loaded?" probe and the subsequent insert are NOT
// atomic. This is safe today because the loader runs once per process
// at boot and the project ships a single server binary. If LoadAndSync
// is ever invoked concurrently (e.g. exposed as an admin endpoint or
// run from two boot paths against a shared DB) it must be wrapped in
// an application-level mutex or rewritten to do the probe + load
// inside one transaction.
func LoadAndSync(ctx context.Context, db *sql.DB, src fs.FS) (LoadedWorld, error) {
	world, err := parseWorld(src)
	if err != nil {
		return LoadedWorld{}, fmt.Errorf("world: parse: %w", err)
	}
	if err := validate(world); err != nil {
		return LoadedWorld{}, fmt.Errorf("world: validate: %w", err)
	}

	already, err := worldAlreadyLoaded(ctx, db)
	if err != nil {
		return LoadedWorld{}, fmt.Errorf("world: probe existing rows: %w", err)
	}
	if !already {
		if err := insertWorld(ctx, db, world); err != nil {
			return LoadedWorld{}, fmt.Errorf("world: insert: %w", err)
		}
		slog.Info("world: load complete",
			"zones", len(world.Zones),
			"rooms", len(world.Rooms),
			"items", len(world.Items),
			"mobs", len(world.Mobs))
	} else {
		slog.Info("world: already loaded, skipping insert",
			"zones", len(world.Zones),
			"rooms", len(world.Rooms),
			"items", len(world.Items),
			"mobs", len(world.Mobs))
	}

	itemSpecs, err := buildItemSpecs(world)
	if err != nil {
		return LoadedWorld{}, fmt.Errorf("world: build item specs: %w", err)
	}
	return LoadedWorld{ItemSpecsByZone: itemSpecs}, nil
}

// worldAlreadyLoaded probes whether either of the two top-level world
// tables has rows. Today rooms + zones are inserted in the same
// transaction so they're always either both empty or both populated;
// covering both is cheap insurance against a future change that
// splits the insert path (e.g. zone reset rules loaded after a hot
// reload). Without this, a half-loaded DB with a populated zones
// table but no rooms would re-enter the loader and fail with a
// duplicate-zone error mid-insert.
func worldAlreadyLoaded(ctx context.Context, db *sql.DB) (bool, error) {
	row := db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM rooms) OR EXISTS(SELECT 1 FROM zones)`)
	var exists int
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists != 0, nil
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

// insertWorld writes the parsed world into the DB inside a single
// transaction. The starter room is forced to id=1 so the
// repo.StarterRoomID constant stays valid; everything else
// auto-increments.
func insertWorld(ctx context.Context, db *sql.DB, w *World) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	zoneIDs, err := insertZones(ctx, tx, w.Zones)
	if err != nil {
		return err
	}
	roomIDs, err := insertRooms(ctx, tx, w.Rooms, zoneIDs)
	if err != nil {
		return err
	}
	if err := insertExits(ctx, tx, w.Rooms, roomIDs); err != nil {
		return err
	}
	if err := insertItems(ctx, tx, w.Items, roomIDs); err != nil {
		return err
	}
	roomZones := make(map[string]int64, len(w.Rooms))
	for _, r := range w.Rooms {
		if zid, ok := zoneIDs[r.ZoneExternalID]; ok {
			roomZones[r.ID] = zid
		}
	}
	if err := insertMobs(ctx, tx, w.Mobs, roomIDs, roomZones); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
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
func insertZones(ctx context.Context, tx *sql.Tx, zones []Zone) (map[string]int64, error) {
	out := make(map[string]int64, len(zones))
	for _, z := range zones {
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
			return nil, fmt.Errorf("insert zone %q: %w", z.ID, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("last insert id for zone %q: %w", z.ID, err)
		}
		out[z.ID] = id
	}
	return out, nil
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
func insertRooms(ctx context.Context, tx *sql.Tx, rooms []Room, zoneIDs map[string]int64) (map[string]int64, error) {
	out := make(map[string]int64, len(rooms))
	triggers := repo.NewSQLiteTriggerRepo(tx)

	resolveZone := func(r Room) (int64, error) {
		id, ok := zoneIDs[r.ZoneExternalID]
		if !ok {
			return 0, fmt.Errorf("room %q references unknown zone %q", r.ID, r.ZoneExternalID)
		}
		return id, nil
	}

	// Validation has already established exactly one starter exists.
	var starterIdx int
	for i, r := range rooms {
		if r.Starter {
			starterIdx = i
			break
		}
	}

	starter := rooms[starterIdx]
	starterZoneID, err := resolveZone(starter)
	if err != nil {
		return nil, err
	}
	starterCols, starterVals := roomInsertValues(starter, starterZoneID)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO rooms(id, `+starterCols+`) VALUES (?, `+repo.Placeholders(len(starterVals))+`)`,
		append([]any{repo.StarterRoomID}, starterVals...)...,
	); err != nil {
		return nil, fmt.Errorf("insert starter room %q: %w", starter.ID, err)
	}
	out[starter.ID] = repo.StarterRoomID
	if err := insertRoomTriggers(ctx, triggers, repo.StarterRoomID, starter); err != nil {
		return nil, err
	}

	for i, r := range rooms {
		if i == starterIdx {
			continue
		}
		zoneID, err := resolveZone(r)
		if err != nil {
			return nil, err
		}
		cols, vals := roomInsertValues(r, zoneID)
		res, err := tx.ExecContext(ctx,
			`INSERT INTO rooms(`+cols+`) VALUES (`+repo.Placeholders(len(vals))+`)`,
			vals...,
		)
		if err != nil {
			return nil, fmt.Errorf("insert room %q: %w", r.ID, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("last insert id for room %q: %w", r.ID, err)
		}
		out[r.ID] = id
		if err := insertRoomTriggers(ctx, triggers, id, r); err != nil {
			return nil, err
		}
	}
	return out, nil
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
		indoors, nopvp, noteleport, dark, silent, peaceful, nomap,
		sector, light_level, coord_x, coord_y, coord_z, coords_auto,
		extra_descs_json, zone_id`
	vals := []any{
		r.ID, r.Name, r.Short, r.Long,
		repo.BoolToInt(r.Flags.Indoors), repo.BoolToInt(r.Flags.NoPVP),
		repo.BoolToInt(r.Flags.NoTeleport), repo.BoolToInt(r.Flags.Dark),
		repo.BoolToInt(r.Flags.Silent), repo.BoolToInt(r.Flags.Peaceful),
		repo.BoolToInt(r.Flags.NoMap),
		sector, light, x, y, z, coordsAuto, extraJSON,
		zoneID,
	}
	return cols, vals
}

func insertExits(ctx context.Context, tx *sql.Tx, rooms []Room, roomIDs map[string]int64) error {
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
			ex := r.Exits[dir]
			to, ok := roomIDs[ex.To]
			if !ok {
				// validate() already caught this, but defensively.
				return fmt.Errorf("exit from %q dir %q targets unknown room %q", r.ID, dir, ex.To)
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
				return fmt.Errorf("insert exit %q->%q: %w", r.ID, dir, err)
			}
		}
	}
	return nil
}

func insertItems(ctx context.Context, tx *sql.Tx, items []Item, roomIDs map[string]int64) error {
	for _, it := range items {
		roomID := roomIDs[it.Room]
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
			return fmt.Errorf("insert item %q: %w", it.ID, err)
		}
		stats, err := convertItemStats(it)
		if err != nil {
			return fmt.Errorf("insert item %q: %w", it.ID, err)
		}
		statsJSON, err := encodeItemStatsJSON(stats)
		if err != nil {
			return fmt.Errorf("insert item %q: %w", it.ID, err)
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
			return fmt.Errorf("insert item %q: %w", it.ID, err)
		}
	}
	return nil
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
func insertMobs(ctx context.Context, tx *sql.Tx, mobs []Mob, roomIDs, roomZones map[string]int64) error {
	templates := repo.NewSQLiteMobTemplateRepo(tx)
	instances := repo.NewSQLiteMobInstanceRepo(tx)
	shops := repo.NewSQLiteShopRepo(tx)
	bankers := repo.NewSQLiteBankerRepo(tx)
	trainers := repo.NewSQLiteTrainerRepo(tx)
	weaveTeachers := repo.NewSQLiteWeaveTeacherRepo(tx)
	triggers := repo.NewSQLiteTriggerRepo(tx)

	for _, m := range mobs {
		if m.XPValue < 0 {
			return fmt.Errorf("mob %q: xp_value must be >= 0, got %d "+
				"(0 = fall back to challenge_code table)", m.ID, m.XPValue)
		}
		roomID := roomIDs[m.Room]
		wander := creature.DefaultWanderChance
		if m.WanderChance != nil {
			wander = *m.WanderChance
		}
		tpl := creature.MobTemplate{
			ExternalID:    m.ID,
			ChallengeCode: 'A',
			Organization:  "solitary",
			WanderChance:  wander,
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
				return fmt.Errorf("mob %q dialogue: %w", m.ID, err)
			}
			tpl.DialogueJSON = djson
		}
		created, err := templates.Create(ctx, tpl)
		if err != nil {
			return fmt.Errorf("insert mob template %q: %w", m.ID, err)
		}
		// Stamp the §9 spawn anchor so the §19 Respawner can top up
		// this mob's population on AreaReset ticks. roomZones[m.Room]
		// is 0 for orphan rooms (validation already rejects those),
		// so a zero zone is a loader bug worth surfacing.
		zoneID := roomZones[m.Room]
		if zoneID == 0 {
			return fmt.Errorf("mob %q in room %q: room has no zone", m.ID, m.Room)
		}
		if err := templates.SetSpawnAnchor(ctx, created.ID, zoneID, roomID); err != nil {
			return fmt.Errorf("set spawn anchor for mob %q: %w", m.ID, err)
		}
		created.RespawnZoneResetID = zoneID
		created.HomeRoomID = roomID
		spawn := creature.NewInstanceFromTemplate(created, roomID, 0)
		if _, err := instances.Create(ctx, spawn); err != nil {
			return fmt.Errorf("spawn mob instance %q: %w", m.ID, err)
		}
		if m.Shop != nil {
			if err := insertShop(ctx, shops, created.ID, m); err != nil {
				return fmt.Errorf("insert shop for mob %q: %w", m.ID, err)
			}
		}
		if m.Banker != nil {
			if err := insertBanker(ctx, bankers, created.ID, m); err != nil {
				return fmt.Errorf("insert banker for mob %q: %w", m.ID, err)
			}
		}
		if m.Trainer != nil {
			if err := insertTrainer(ctx, trainers, created.ID, m); err != nil {
				return fmt.Errorf("insert trainer for mob %q: %w", m.ID, err)
			}
		}
		if m.WeaveTeacher != nil {
			if err := insertWeaveTeacher(ctx, weaveTeachers, created.ID, m); err != nil {
				return fmt.Errorf("insert weave teacher for mob %q: %w", m.ID, err)
			}
		}
		if err := insertMobTriggers(ctx, triggers, created.ID, m); err != nil {
			return err
		}
	}
	return nil
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
