package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SQLiteRoomRepo struct {
	db *sql.DB
}

func NewSQLiteRoomRepo(db *sql.DB) *SQLiteRoomRepo {
	return &SQLiteRoomRepo{db: db}
}

const roomSelectCols = `id, external_id, name, short_desc, long_desc, ` +
	`indoors, nopvp, noteleport, dark, silent, peaceful, ` +
	`sector, light_level, coord_x, coord_y, coord_z, extra_descs_json, created_at`

func (r *SQLiteRoomRepo) FindByID(ctx context.Context, id int64) (Room, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+roomSelectCols+` FROM rooms WHERE id = ?`, id)
	return scanRoom(row)
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

	insertCols := `external_id, name, short_desc, long_desc, ` +
		`indoors, nopvp, noteleport, dark, silent, peaceful, ` +
		`sector, light_level, coord_x, coord_y, coord_z, extra_descs_json, created_at`
	insertVals := []any{
		room.ExternalID, room.Name, room.ShortDesc, room.LongDesc,
		boolToInt(room.Flags.Indoors), boolToInt(room.Flags.NoPVP),
		boolToInt(room.Flags.NoTeleport), boolToInt(room.Flags.Dark),
		boolToInt(room.Flags.Silent), boolToInt(room.Flags.Peaceful),
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
		room                                               Room
		indoors, nopvp, noteleport, dark, silent, peaceful int
		sector                                             string
		extraJSON                                          string
	)
	err := row.Scan(
		&room.ID, &room.ExternalID, &room.Name, &room.ShortDesc, &room.LongDesc,
		&indoors, &nopvp, &noteleport, &dark, &silent, &peaceful,
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
	}
	room.Sector = Sector(sector)
	descs, err := unmarshalExtraDescs(extraJSON)
	if err != nil {
		return Room{}, fmt.Errorf("scan room extra_descs (id=%d): %w", room.ID, err)
	}
	room.ExtraDescs = descs
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

func unmarshalExtraDescs(raw string) (map[string]string, error) {
	if raw == "" || raw == "{}" {
		return nil, nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
