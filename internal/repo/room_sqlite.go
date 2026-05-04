package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type SQLiteRoomRepo struct {
	db *sql.DB
}

func NewSQLiteRoomRepo(db *sql.DB) *SQLiteRoomRepo {
	return &SQLiteRoomRepo{db: db}
}

const roomSelectCols = `id, external_id, zone_id, name, short_desc, long_desc, ` +
	`indoors, nopvp, noteleport, dark, silent, peaceful, nomap, ` +
	`sector, light_level, coord_x, coord_y, coord_z, extra_descs_json, created_at`

func (r *SQLiteRoomRepo) FindByID(ctx context.Context, id int64) (Room, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+roomSelectCols+` FROM rooms WHERE id = ?`, id)
	return scanRoom(row)
}

func (r *SQLiteRoomRepo) CountByZone(ctx context.Context, zoneID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM rooms WHERE zone_id = ?`, zoneID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count rooms by zone: %w", err)
	}
	return n, nil
}

func (r *SQLiteRoomRepo) FindByExternalID(ctx context.Context, externalID string) (Room, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+roomSelectCols+` FROM rooms WHERE external_id = ?`, externalID)
	return scanRoom(row)
}

func (r *SQLiteRoomRepo) Create(ctx context.Context, room Room) (Room, error) {
	if room.ExternalID == "" {
		return Room{}, ErrInvalidExternalID
	}
	if room.CreatedAt.IsZero() {
		room.CreatedAt = time.Now().UTC()
	}
	if room.Sector == "" {
		room.Sector = SectorCity
	}

	extraJSON, err := marshalExtraDescs(room.ExtraDescs)
	if err != nil {
		return Room{}, fmt.Errorf("marshal extra_descs: %w", err)
	}

	insertCols := `external_id, zone_id, name, short_desc, long_desc, ` +
		`indoors, nopvp, noteleport, dark, silent, peaceful, nomap, ` +
		`sector, light_level, coord_x, coord_y, coord_z, extra_descs_json, created_at`
	insertVals := []any{
		room.ExternalID, room.ZoneID, room.Name, room.ShortDesc, room.LongDesc,
		boolToInt(room.Flags.Indoors), boolToInt(room.Flags.NoPVP),
		boolToInt(room.Flags.NoTeleport), boolToInt(room.Flags.Dark),
		boolToInt(room.Flags.Silent), boolToInt(room.Flags.Peaceful),
		boolToInt(room.Flags.NoMap),
		string(room.Sector), room.LightLevel, room.CoordX, room.CoordY, room.CoordZ,
		extraJSON, room.CreatedAt,
	}

	if room.ID != 0 {
		args := append([]any{room.ID}, insertVals...)
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO rooms(id, `+insertCols+`) VALUES (`+placeholders(len(args))+`)`,
			args...,
		)
		if err != nil {
			return Room{}, mapRoomInsertErr(err)
		}
		return room, nil
	}

	res, err := r.db.ExecContext(ctx,
		`INSERT INTO rooms(`+insertCols+`) VALUES (`+placeholders(len(insertVals))+`)`,
		insertVals...,
	)
	if err != nil {
		return Room{}, mapRoomInsertErr(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Room{}, fmt.Errorf("last insert id: %w", err)
	}
	room.ID = id
	return room, nil
}

func scanRoom(row *sql.Row) (Room, error) {
	var (
		room                                                      Room
		indoors, nopvp, noteleport, dark, silent, peaceful, nomap int
		sector                                                    string
		extraJSON                                                 string
	)
	err := row.Scan(
		&room.ID, &room.ExternalID, &room.ZoneID, &room.Name, &room.ShortDesc, &room.LongDesc,
		&indoors, &nopvp, &noteleport, &dark, &silent, &peaceful, &nomap,
		&sector, &room.LightLevel, &room.CoordX, &room.CoordY, &room.CoordZ,
		&extraJSON, &room.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Room{}, ErrRoomNotFound
	}
	if err != nil {
		return Room{}, fmt.Errorf("scan room: %w", err)
	}
	room.Flags = RoomFlags{
		Indoors:    indoors != 0,
		NoPVP:      nopvp != 0,
		NoTeleport: noteleport != 0,
		Dark:       dark != 0,
		Silent:     silent != 0,
		Peaceful:   peaceful != 0,
		NoMap:      nomap != 0,
	}
	room.Sector = Sector(sector)
	room.ExtraDescs = unmarshalExtraDescs(extraJSON)
	return room, nil
}

func mapRoomInsertErr(err error) error {
	if isUniqueViolation(err) {
		return ErrDuplicateExternalID
	}
	return fmt.Errorf("insert room: %w", err)
}

// marshalExtraDescs returns the JSON object form of the keyword map.
// Keys are lowercased so look <Word> matches `word` in the map without
// rewriting the long-form text. Empty / nil maps marshal to "{}" so
// the column matches the schema default.
func marshalExtraDescs(m map[string]string) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	normalized := make(map[string]string, len(m))
	for k, v := range m {
		normalized[strings.ToLower(strings.TrimSpace(k))] = v
	}
	out, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// unmarshalExtraDescs decodes the extra_descs_json column. Returns nil
// for empty / "{}" / corrupt blobs so a single bad row can't brick a
// room; corrupt JSON logs a slog.Warn for the admin. No error return —
// ExtraDescs is non-critical ambient text and the caller has no other
// recovery action than to ignore it.
func unmarshalExtraDescs(raw string) map[string]string {
	if raw == "" || raw == "{}" {
		return nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		slog.Warn("unmarshal extra_descs: corrupt JSON, dropping",
			"raw_len", len(raw), "error", err)
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
