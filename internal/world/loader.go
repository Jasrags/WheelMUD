package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// LoadAndSync reads YAML zone folders from src, validates the world,
// and populates the rooms / exits / items / mobs tables. It is a no-op
// if the world tables already have rows (boot-time only — pick up YAML
// changes by wiping the DB).
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
func LoadAndSync(ctx context.Context, db *sql.DB, src fs.FS) error {
	already, err := worldAlreadyLoaded(ctx, db)
	if err != nil {
		return fmt.Errorf("world: probe existing rows: %w", err)
	}
	if already {
		slog.Info("world: already loaded, skipping")
		return nil
	}

	world, err := parseWorld(src)
	if err != nil {
		return fmt.Errorf("world: parse: %w", err)
	}
	if err := validate(world); err != nil {
		return fmt.Errorf("world: validate: %w", err)
	}

	if err := insertWorld(ctx, db, world); err != nil {
		return fmt.Errorf("world: insert: %w", err)
	}
	slog.Info("world: load complete",
		"zones", len(world.Zones),
		"rooms", len(world.Rooms),
		"items", len(world.Items),
		"mobs", len(world.Mobs))
	return nil
}

// worldAlreadyLoaded probes whether the rooms table has any rows.
// Migrations 0006 wipes the tables; the loader runs once on first boot
// and is a no-op on every subsequent boot.
func worldAlreadyLoaded(ctx context.Context, db *sql.DB) (bool, error) {
	row := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM rooms)`)
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

	roomIDs, err := insertRooms(ctx, tx, w.Rooms)
	if err != nil {
		return err
	}
	if err := insertExits(ctx, tx, w.Rooms, roomIDs); err != nil {
		return err
	}
	if err := insertItems(ctx, tx, w.Items, roomIDs); err != nil {
		return err
	}
	if err := insertMobs(ctx, tx, w.Mobs, roomIDs); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// insertRooms inserts every room and returns a map from external_id ->
// int id. The starter room is inserted first with an explicit id=1 so
// repo.StarterRoomID stays accurate.
//
// Note: this writes raw SQL into *sql.Tx instead of going through
// repo.RoomRepo.Create. The repo Create takes *sql.DB, so calling it
// from inside a transaction is not possible without either a tx-aware
// variant of the interface or rewriting the loader to not be
// transactional. Atomicity across all four kinds matters more here
// than reuse, so the column list is duplicated. Keep the INSERT
// columns in sync with room_sqlite.go::Create if either changes.
func insertRooms(ctx context.Context, tx *sql.Tx, rooms []Room) (map[string]int64, error) {
	out := make(map[string]int64, len(rooms))

	// Validation has already established exactly one starter exists.
	var starterIdx int
	for i, r := range rooms {
		if r.Starter {
			starterIdx = i
			break
		}
	}

	starter := rooms[starterIdx]
	starterCols, starterVals := roomInsertValues(starter)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO rooms(id, `+starterCols+`) VALUES (?, `+placeholders(len(starterVals))+`)`,
		append([]any{repo.StarterRoomID}, starterVals...)...,
	); err != nil {
		return nil, fmt.Errorf("insert starter room %q: %w", starter.ID, err)
	}
	out[starter.ID] = repo.StarterRoomID

	for i, r := range rooms {
		if i == starterIdx {
			continue
		}
		cols, vals := roomInsertValues(r)
		res, err := tx.ExecContext(ctx,
			`INSERT INTO rooms(`+cols+`) VALUES (`+placeholders(len(vals))+`)`,
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
	}
	return out, nil
}

// roomInsertValues materializes the column list + values for one room
// row, applying defaults (sector=city, light=DefaultLightLevel for
// non-dark rooms) when the YAML left them blank.
func roomInsertValues(r Room) (string, []any) {
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
	var x, y, z int
	if r.Coords != nil {
		x, y, z = r.Coords.X, r.Coords.Y, r.Coords.Z
	}
	cols := `external_id, name, short_desc, long_desc,
		indoors, nopvp, noteleport, dark, silent, peaceful,
		sector, light_level, coord_x, coord_y, coord_z`
	vals := []any{
		r.ID, r.Name, r.Short, r.Long,
		boolInt(r.Flags.Indoors), boolInt(r.Flags.NoPVP),
		boolInt(r.Flags.NoTeleport), boolInt(r.Flags.Dark),
		boolInt(r.Flags.Silent), boolInt(r.Flags.Peaceful),
		sector, light, x, y, z,
	}
	return cols, vals
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, 0, n*2)
	for i := 0; i < n; i++ {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, '?')
	}
	return string(out)
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
			to, ok := roomIDs[r.Exits[dir]]
			if !ok {
				// validate() already caught this, but defensively.
				return fmt.Errorf("exit from %q dir %q targets unknown room %q", r.ID, dir, r.Exits[dir])
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO exits(from_room_id, to_room_id, direction) VALUES (?, ?, ?)`,
				from, to, dir,
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
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO items(external_id, name, name_lower, short_desc, room_id) VALUES (?, ?, ?, ?, ?)`,
			it.ID, it.Name, strings.ToLower(it.Name), it.Short, roomID,
		); err != nil {
			return fmt.Errorf("insert item %q: %w", it.ID, err)
		}
	}
	return nil
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
func insertMobs(ctx context.Context, tx *sql.Tx, mobs []Mob, roomIDs map[string]int64) error {
	templates := repo.NewSQLiteMobTemplateRepo(tx)
	instances := repo.NewSQLiteMobInstanceRepo(tx)

	for _, m := range mobs {
		roomID := roomIDs[m.Room]
		tpl := creature.MobTemplate{
			ExternalID:    m.ID,
			ChallengeCode: 'A',
			Organization:  "solitary",
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
		created, err := templates.Create(ctx, tpl)
		if err != nil {
			return fmt.Errorf("insert mob template %q: %w", m.ID, err)
		}
		spawn := creature.MobInstance{
			TemplateID: created.ID,
			Core: creature.Core{
				HPCurrent:     created.Core.HPMax,
				CurrentRoomID: roomID,
			},
		}
		if _, err := instances.Create(ctx, spawn); err != nil {
			return fmt.Errorf("spawn mob instance %q: %w", m.ID, err)
		}
	}
	return nil
}
