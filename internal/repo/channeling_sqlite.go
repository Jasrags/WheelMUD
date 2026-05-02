package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

type SQLiteChannelingRepo struct {
	db *sql.DB
}

func NewSQLiteChannelingRepo(db *sql.DB) *SQLiteChannelingRepo {
	return &SQLiteChannelingRepo{db: db}
}

func (r *SQLiteChannelingRepo) Upsert(ctx context.Context, kind OwnerKind, ownerID int64, c creature.Channeling) error {
	if !kind.Valid() {
		return ErrInvalidOwnerKind
	}
	talentsJSON, err := marshalJSONSlice(c.Talents)
	if err != nil {
		return err
	}
	weavesJSON, err := marshalJSONSlice(c.WeavesKnown)
	if err != nil {
		return err
	}
	slotsJSON, err := jsonMarshalString(c.Slots)
	if err != nil {
		return err
	}

	var embracedSince sql.NullTime
	if !c.EmbracedSince.IsZero() {
		embracedSince = sql.NullTime{Time: c.EmbracedSince, Valid: true}
	}

	// SQLite's ON CONFLICT replace path needs the unique key
	// (owner_kind, owner_id), declared in 0008.
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO channeling(
			owner_kind, owner_id,
			gender_source, channeler_type, affinities,
			talents_json, weaves_known_json, slots_json,
			embraced, embraced_since, madness, stilled,
			bonded_warder_id, bonded_aes_sedai_id,
			held_angreal_id, held_saangreal_id, circle_id,
			aes_sedai_oaths, ageless, damane_collar_to
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(owner_kind, owner_id) DO UPDATE SET
			gender_source       = excluded.gender_source,
			channeler_type      = excluded.channeler_type,
			affinities          = excluded.affinities,
			talents_json        = excluded.talents_json,
			weaves_known_json   = excluded.weaves_known_json,
			slots_json          = excluded.slots_json,
			embraced            = excluded.embraced,
			embraced_since      = excluded.embraced_since,
			madness             = excluded.madness,
			stilled             = excluded.stilled,
			bonded_warder_id    = excluded.bonded_warder_id,
			bonded_aes_sedai_id = excluded.bonded_aes_sedai_id,
			held_angreal_id     = excluded.held_angreal_id,
			held_saangreal_id   = excluded.held_saangreal_id,
			circle_id           = excluded.circle_id,
			aes_sedai_oaths     = excluded.aes_sedai_oaths,
			ageless             = excluded.ageless,
			damane_collar_to    = excluded.damane_collar_to`,
		kind, ownerID,
		c.GenderSource, c.ChannelerType, c.Affinities,
		talentsJSON, weavesJSON, slotsJSON,
		boolToInt(c.Embraced), embracedSince, c.Madness, boolToInt(c.Stilled),
		c.BondedWarderID, c.BondedAesSedaiID,
		c.HeldAngrealID, c.HeldSaangrealID, c.CircleID,
		c.AesSedaiOaths, boolToInt(c.Ageless), c.DamaneCollarTo,
	)
	if err != nil {
		return fmt.Errorf("upsert channeling: %w", err)
	}
	return nil
}

func (r *SQLiteChannelingRepo) GetByOwner(ctx context.Context, kind OwnerKind, ownerID int64) (creature.Channeling, error) {
	if !kind.Valid() {
		return creature.Channeling{}, ErrInvalidOwnerKind
	}
	row := r.db.QueryRowContext(ctx,
		`SELECT gender_source, channeler_type, affinities,
			talents_json, weaves_known_json, slots_json,
			embraced, embraced_since, madness, stilled,
			bonded_warder_id, bonded_aes_sedai_id,
			held_angreal_id, held_saangreal_id, circle_id,
			aes_sedai_oaths, ageless, damane_collar_to
		 FROM channeling WHERE owner_kind = ? AND owner_id = ?`,
		kind, ownerID,
	)
	var (
		c                                 creature.Channeling
		talentsJSON, weavesJSON, slotsJSON string
		embraced, stilled, ageless         int
		embracedSince                      sql.NullTime
	)
	err := row.Scan(
		&c.GenderSource, &c.ChannelerType, &c.Affinities,
		&talentsJSON, &weavesJSON, &slotsJSON,
		&embraced, &embracedSince, &c.Madness, &stilled,
		&c.BondedWarderID, &c.BondedAesSedaiID,
		&c.HeldAngrealID, &c.HeldSaangrealID, &c.CircleID,
		&c.AesSedaiOaths, &ageless, &c.DamaneCollarTo,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return creature.Channeling{}, ErrChannelingNotFound
	}
	if err != nil {
		return creature.Channeling{}, fmt.Errorf("scan channeling: %w", err)
	}
	c.Embraced = embraced != 0
	c.Stilled = stilled != 0
	c.Ageless = ageless != 0
	if embracedSince.Valid {
		c.EmbracedSince = embracedSince.Time
	}
	if err := unmarshalJSONSlice(talentsJSON, &c.Talents); err != nil {
		return creature.Channeling{}, err
	}
	if err := unmarshalJSONSlice(weavesJSON, &c.WeavesKnown); err != nil {
		return creature.Channeling{}, err
	}
	if err := jsonUnmarshalString(slotsJSON, &c.Slots); err != nil {
		return creature.Channeling{}, err
	}
	return c, nil
}

func (r *SQLiteChannelingRepo) DeleteByOwner(ctx context.Context, kind OwnerKind, ownerID int64) error {
	if !kind.Valid() {
		return ErrInvalidOwnerKind
	}
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM channeling WHERE owner_kind = ? AND owner_id = ?`,
		kind, ownerID,
	)
	if err != nil {
		return fmt.Errorf("delete channeling: %w", err)
	}
	return checkRowsAffected(res, ErrChannelingNotFound)
}
