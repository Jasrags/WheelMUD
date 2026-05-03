package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
)

type SQLiteCharacterRepo struct {
	db *sql.DB
}

func NewSQLiteCharacterRepo(db *sql.DB) *SQLiteCharacterRepo {
	return &SQLiteCharacterRepo{db: db}
}

func (r *SQLiteCharacterRepo) Create(ctx context.Context, c Character) (Character, error) {
	c.NameLower = strings.ToLower(c.Name)
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	if c.CurrentRoomID == 0 {
		c.CurrentRoomID = StarterRoomID
	}
	if c.BoundRoomID == 0 {
		c.BoundRoomID = StarterRoomID
	}

	jsons, err := marshalCharacterJSON(c)
	if err != nil {
		return Character{}, err
	}

	args := []any{
		c.AccountID, c.Name, c.NameLower, c.CreatedAt, c.CurrentRoomID,
	}
	args = append(args, charCoreValues(c, jsons.dr, jsons.resists, jsons.affects)...)
	args = append(args, charPlayerValues(c,
		jsons.classLevels, jsons.feats, jsons.skills, jsons.classFeatures,
		jsons.questLog, jsons.dialogueState, jsons.equipment, jsons.inventory,
		jsons.channelSettings)...)

	query := fmt.Sprintf(
		`INSERT INTO characters(
			account_id, name, name_lower, created_at, current_room_id,
			%s,
			%s
		) VALUES (%s)`,
		charCoreColumns, charPlayerColumns, placeholders(len(args)),
	)
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		if isUniqueViolation(err) {
			return Character{}, ErrDuplicateCharacterName
		}
		return Character{}, fmt.Errorf("insert character: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Character{}, fmt.Errorf("last insert id: %w", err)
	}
	c.ID = id
	return c, nil
}

func (r *SQLiteCharacterRepo) FindByName(ctx context.Context, name string) (Character, error) {
	row := r.db.QueryRowContext(ctx, characterSelect+` WHERE name_lower = ?`, strings.ToLower(name))
	c, err := scanCharacter(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Character{}, ErrCharacterNotFound
	}
	return c, err
}

func (r *SQLiteCharacterRepo) ListByAccount(ctx context.Context, accountID int64) ([]Character, error) {
	rows, err := r.db.QueryContext(ctx,
		characterSelect+` WHERE account_id = ? ORDER BY last_played_at DESC NULLS LAST, name_lower`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list characters: %w", err)
	}
	defer rows.Close()
	var out []Character
	for rows.Next() {
		c, err := scanCharacter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *SQLiteCharacterRepo) RecordPlay(ctx context.Context, id int64, when time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE characters SET last_played_at = ? WHERE id = ?`,
		when, id,
	)
	if err != nil {
		return fmt.Errorf("record play: %w", err)
	}
	return nil
}

func (r *SQLiteCharacterRepo) RecordRoom(ctx context.Context, id, roomID int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters SET current_room_id = ? WHERE id = ?`,
		roomID, id,
	)
	if err != nil {
		return fmt.Errorf("record room: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *SQLiteCharacterRepo) RecordCore(ctx context.Context, id int64, hp, subdual int32, cond creature.Condition, pos creature.PositionFlags) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters SET hp_current = ?, subdual = ?, conditions = ?, position_flags = ?
		 WHERE id = ?`,
		hp, subdual, cond, pos, id,
	)
	if err != nil {
		return fmt.Errorf("record core: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *SQLiteCharacterRepo) RecordInventory(ctx context.Context, id int64, ids []int64) error {
	js, err := marshalJSONSlice(ids)
	if err != nil {
		return fmt.Errorf("marshal inventory: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters SET inventory_json = ? WHERE id = ?`,
		js, id,
	)
	if err != nil {
		return fmt.Errorf("record inventory: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *SQLiteCharacterRepo) RecordCoin(ctx context.Context, id int64, coin, bank currency.Amount) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters SET coin_cp = ?, bank_cp = ? WHERE id = ?`,
		int64(coin), int64(bank), id,
	)
	if err != nil {
		return fmt.Errorf("record coin: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *SQLiteCharacterRepo) RecordChannelSettings(ctx context.Context, id int64, settings map[string]bool) error {
	js, err := jsonMarshalString(settings)
	if err != nil {
		return fmt.Errorf("marshal channel settings: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters SET channel_settings_json = ? WHERE id = ?`,
		js, id,
	)
	if err != nil {
		return fmt.Errorf("record channel settings: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

// characterSelect is the canonical SELECT used by FindByName /
// ListByAccount. Columns ordered: identity → CurrentRoomID → Core
// block → player block. scanCharacter mirrors the order.
var characterSelect = `SELECT id, account_id, name, name_lower, created_at, last_played_at, current_room_id, ` +
	charCoreColumns + `, ` + charPlayerColumns + ` FROM characters`

func scanCharacter(s scanner) (Character, error) {
	var (
		c          Character
		lastPlayed sql.NullTime
		j          characterJSON
		coinCP     int64
		bankCP     int64
		fatigue    sql.NullTime
		idle       sql.NullTime
		login      sql.NullTime
	)
	dest := []any{
		&c.ID, &c.AccountID, &c.Name, &c.NameLower, &c.CreatedAt, &lastPlayed, &c.CurrentRoomID,
	}
	dest = append(dest, charCoreScanDest(&c, &j.dr, &j.resists, &j.affects)...)
	dest = append(dest, charPlayerScanDest(&c,
		&j.classLevels, &j.feats, &j.skills, &j.classFeatures,
		&coinCP, &bankCP,
		&fatigue, &idle, &login,
		&j.questLog, &j.dialogueState, &j.equipment, &j.inventory,
		&j.channelSettings)...)

	if err := s.Scan(dest...); err != nil {
		return Character{}, err
	}
	if lastPlayed.Valid {
		t := lastPlayed.Time
		c.LastPlayedAt = &t
	}
	c.Coin = currency.Amount(coinCP)
	c.BankBalance = currency.Amount(bankCP)
	if fatigue.Valid {
		c.FatigueUntil = fatigue.Time
	}
	if idle.Valid {
		c.IdleSince = idle.Time
	}
	if login.Valid {
		c.LastLogin = login.Time
	}
	if err := j.unmarshalInto(&c); err != nil {
		return Character{}, err
	}
	return c, nil
}

// characterJSON bundles the eight JSON-encoded columns on the
// characters table (Core's three + player block's eight). Same
// pattern as templateJSON in creature_sql.go.
type characterJSON struct {
	dr, resists, affects                          string
	classLevels, feats, skills, classFeatures     string
	questLog, dialogueState, equipment, inventory string
	channelSettings                               string
}

func marshalCharacterJSON(c Character) (characterJSON, error) {
	var j characterJSON
	var err error
	if j.dr, err = marshalJSONSlice(c.Core.DR); err != nil {
		return j, err
	}
	if j.resists, err = marshalJSONSlice(c.Core.Resists); err != nil {
		return j, err
	}
	if j.affects, err = marshalJSONSlice(c.Core.Affects); err != nil {
		return j, err
	}
	if j.classLevels, err = jsonMarshalString(c.ClassLevels); err != nil {
		return j, err
	}
	if j.feats, err = marshalJSONSlice(c.Feats); err != nil {
		return j, err
	}
	if j.skills, err = jsonMarshalString(c.Skills); err != nil {
		return j, err
	}
	if j.classFeatures, err = marshalJSONSlice(c.ClassFeatures); err != nil {
		return j, err
	}
	if j.questLog, err = marshalJSONSlice(c.QuestLog); err != nil {
		return j, err
	}
	if j.dialogueState, err = jsonMarshalString(c.DialogueState); err != nil {
		return j, err
	}
	if j.equipment, err = jsonMarshalString(c.Equipment); err != nil {
		return j, err
	}
	if j.inventory, err = marshalJSONSlice(c.Inventory); err != nil {
		return j, err
	}
	if j.channelSettings, err = jsonMarshalString(c.ChannelSettings); err != nil {
		return j, err
	}
	return j, nil
}

func (j characterJSON) unmarshalInto(c *Character) error {
	if err := unmarshalJSONSlice(j.dr, &c.Core.DR); err != nil {
		return err
	}
	if err := unmarshalJSONSlice(j.resists, &c.Core.Resists); err != nil {
		return err
	}
	if err := unmarshalJSONSlice(j.affects, &c.Core.Affects); err != nil {
		return err
	}
	if err := jsonUnmarshalString(j.classLevels, &c.ClassLevels); err != nil {
		return err
	}
	if err := unmarshalJSONSlice(j.feats, &c.Feats); err != nil {
		return err
	}
	if err := jsonUnmarshalString(j.skills, &c.Skills); err != nil {
		return err
	}
	if err := unmarshalJSONSlice(j.classFeatures, &c.ClassFeatures); err != nil {
		return err
	}
	if err := unmarshalJSONSlice(j.questLog, &c.QuestLog); err != nil {
		return err
	}
	if err := jsonUnmarshalString(j.dialogueState, &c.DialogueState); err != nil {
		return err
	}
	if err := jsonUnmarshalString(j.equipment, &c.Equipment); err != nil {
		return err
	}
	if err := unmarshalJSONSlice(j.inventory, &c.Inventory); err != nil {
		return err
	}
	if err := jsonUnmarshalString(j.channelSettings, &c.ChannelSettings); err != nil {
		return err
	}
	return nil
}
