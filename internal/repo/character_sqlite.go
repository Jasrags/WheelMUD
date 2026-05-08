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
		jsons.channelSettings, jsons.channeling)...)

	// Atomic first-character bootstrap: replace the trailing
	// auth_level placeholder with a CASE expression that consults
	// COUNT(*) at INSERT time. SQLite holds its writer mutex for the
	// entirety of an INSERT statement, so two parallel inserts can't
	// both observe COUNT=0. RETURNING auth_level reads back whichever
	// branch fired so the returned Character reflects the actual
	// stored value.
	//
	// args is laid out [non-auth-cols ..., c.AuthLevel]; the CASE
	// consumes the trailing ? as the ELSE branch. Total placeholder
	// count remains len(args). NOTE: auth_level MUST stay the very
	// last element of charPlayerColumns / charPlayerValues /
	// charPlayerScanDest for this alignment to hold; new columns
	// belong before it. last_news_seen is currently the immediate
	// predecessor — adding another column means slotting it
	// strictly between last_news_seen and auth_level.
	authPlaceholder := fmt.Sprintf(
		`CASE WHEN (SELECT COUNT(*) FROM characters)=0 THEN %d ELSE ? END`,
		AuthLevelAdmin,
	)
	bodyPh := placeholders(len(args) - 1) // all simple ?'s except auth_level
	query := fmt.Sprintf(
		`INSERT INTO characters(
			account_id, name, name_lower, created_at, current_room_id,
			%s,
			%s
		) VALUES (%s, %s)
		 RETURNING id, auth_level`,
		charCoreColumns, charPlayerColumns, bodyPh, authPlaceholder,
	)

	var (
		id        int64
		authLevel uint8
	)
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&id, &authLevel); err != nil {
		if isUniqueViolation(err) {
			return Character{}, ErrDuplicateCharacterName
		}
		return Character{}, fmt.Errorf("insert character: %w", err)
	}
	c.ID = id
	c.AuthLevel = authLevel
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

func (r *SQLiteCharacterRepo) GetByID(ctx context.Context, id int64) (Character, error) {
	row := r.db.QueryRowContext(ctx, characterSelect+` WHERE id = ?`, id)
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

func (r *SQLiteCharacterRepo) RecordEquipment(ctx context.Context, id int64, eq creature.Equipment) error {
	js, err := jsonMarshalString(eq)
	if err != nil {
		return fmt.Errorf("marshal equipment: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters SET equipment_json = ? WHERE id = ?`,
		js, id,
	)
	if err != nil {
		return fmt.Errorf("record equipment: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *SQLiteCharacterRepo) RecordSkillRank(ctx context.Context, id int64,
	skillID int32, newRanks int8, isClassSkill bool, newPending int32) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin skill rank tx: %w", err)
	}
	defer tx.Rollback()

	var skillsJSON string
	if err := tx.QueryRowContext(ctx,
		`SELECT skills_json FROM characters WHERE id = ?`, id,
	).Scan(&skillsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCharacterNotFound
		}
		return fmt.Errorf("read skills: %w", err)
	}

	skills := map[int32]creature.SkillRanks{}
	if err := jsonUnmarshalString(skillsJSON, &skills); err != nil {
		return fmt.Errorf("unmarshal skills: %w", err)
	}
	skills[skillID] = creature.SkillRanks{
		Ranks:        newRanks,
		IsClassSkill: isClassSkill,
	}
	js, err := jsonMarshalString(skills)
	if err != nil {
		return fmt.Errorf("marshal skills: %w", err)
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE characters
		   SET skills_json = ?, pending_skill_points = ?
		 WHERE id = ?`,
		js, newPending, id,
	)
	if err != nil {
		return fmt.Errorf("record skill rank: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCharacterNotFound
	}
	return tx.Commit()
}

func (r *SQLiteCharacterRepo) RecordFeatPick(ctx context.Context, id int64,
	featID int32, newPending int32) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin feat pick tx: %w", err)
	}
	defer tx.Rollback()

	var featsJSON string
	if err := tx.QueryRowContext(ctx,
		`SELECT feats_json FROM characters WHERE id = ?`, id,
	).Scan(&featsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCharacterNotFound
		}
		return fmt.Errorf("read feats: %w", err)
	}

	var feats []int32
	if err := unmarshalJSONSlice(featsJSON, &feats); err != nil {
		return fmt.Errorf("unmarshal feats: %w", err)
	}
	feats = append(feats, featID)
	js, err := marshalJSONSlice(feats)
	if err != nil {
		return fmt.Errorf("marshal feats: %w", err)
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE characters
		   SET feats_json = ?, pending_feats = ?
		 WHERE id = ?`,
		js, newPending, id,
	)
	if err != nil {
		return fmt.Errorf("record feat pick: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCharacterNotFound
	}
	return tx.Commit()
}

func (r *SQLiteCharacterRepo) RecordAbilityBump(ctx context.Context, id int64,
	ability AbilityKey, newScore int8, newPending int32) error {
	col, ok := abilityCurColumn(ability)
	if !ok {
		return fmt.Errorf("record ability bump: unknown ability %d", ability)
	}
	// Column name is from a fixed allow-list — no injection risk.
	q := fmt.Sprintf(
		`UPDATE characters SET %s = ?, pending_ability_bumps = ? WHERE id = ?`,
		col,
	)
	res, err := r.db.ExecContext(ctx, q, newScore, newPending, id)
	if err != nil {
		return fmt.Errorf("record ability bump: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *SQLiteCharacterRepo) RecordWeavePick(ctx context.Context, id int64,
	weaveID string, newPending int32) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin weave pick tx: %w", err)
	}
	defer tx.Rollback()

	var channelingNS sql.NullString
	if err := tx.QueryRowContext(ctx,
		`SELECT channeling_json FROM characters WHERE id = ?`, id,
	).Scan(&channelingNS); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCharacterNotFound
		}
		return fmt.Errorf("read channeling: %w", err)
	}
	raw := ""
	if channelingNS.Valid {
		raw = channelingNS.String
	}

	var ch *creature.Channeling
	if err := jsonUnmarshalString(raw, &ch); err != nil {
		return fmt.Errorf("unmarshal channeling: %w", err)
	}
	if ch == nil {
		return ErrNotChanneler
	}
	ch.WeavesKnownIDs = append(ch.WeavesKnownIDs, weaveID)
	js, err := jsonMarshalString(ch)
	if err != nil {
		return fmt.Errorf("marshal channeling: %w", err)
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE characters
		   SET channeling_json = ?, pending_weaves = ?
		 WHERE id = ?`,
		js, newPending, id,
	)
	if err != nil {
		return fmt.Errorf("record weave pick: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCharacterNotFound
	}
	return tx.Commit()
}

// abilityCurColumn maps the AbilityKey enum to the str/dex/con/int/
// wis/cha _cur column name. Returns ("", false) on unknown enum.
func abilityCurColumn(a AbilityKey) (string, bool) {
	switch a {
	case AbilityStr:
		return "str_cur", true
	case AbilityDex:
		return "dex_cur", true
	case AbilityCon:
		return "con_cur", true
	case AbilityInt:
		return "int_cur", true
	case AbilityWis:
		return "wis_cur", true
	case AbilityCha:
		return "cha_cur", true
	}
	return "", false
}

func (r *SQLiteCharacterRepo) RecordCoin(ctx context.Context, id int64, coin, bank currency.Amount, expectedVersion int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters
		 SET coin_cp = ?, bank_cp = ?, coin_version = coin_version + 1
		 WHERE id = ? AND coin_version = ?`,
		int64(coin), int64(bank), id, expectedVersion,
	)
	if err != nil {
		return fmt.Errorf("record coin: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("record coin rows: %w", err)
	}
	if n > 0 {
		return nil
	}
	// Zero rows affected. Distinguish "row missing" from "version
	// moved on" so the caller can react differently — a missing
	// character row is a programmer bug, a version conflict is a
	// race the verb can refuse cleanly.
	var dummy int
	err = r.db.QueryRowContext(ctx,
		`SELECT 1 FROM characters WHERE id = ?`, id).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCharacterNotFound
	}
	if err != nil {
		return fmt.Errorf("record coin existence check: %w", err)
	}
	return ErrCoinConflict
}

func (r *SQLiteCharacterRepo) RecordXP(ctx context.Context, id int64, xp int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters SET xp = ? WHERE id = ?`,
		xp, id,
	)
	if err != nil {
		return fmt.Errorf("record xp: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *SQLiteCharacterRepo) RecordXPDebt(ctx context.Context, id int64, debt int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters SET xp_debt = ? WHERE id = ?`,
		debt, id,
	)
	if err != nil {
		return fmt.Errorf("record xp debt: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *SQLiteCharacterRepo) RecordPromptTemplate(ctx context.Context, id int64, tmpl string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters SET prompt_template = ? WHERE id = ?`,
		tmpl, id,
	)
	if err != nil {
		return fmt.Errorf("record prompt template: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *SQLiteCharacterRepo) RecordLevelUp(ctx context.Context, id int64, f LevelUpFields) error {
	js, err := jsonMarshalString(f.ClassLevels)
	if err != nil {
		return fmt.Errorf("marshal class levels: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters
		   SET hp_current = ?, hp_max = ?, bab = ?,
		       save_fort = ?, save_ref = ?, save_will = ?,
		       class_levels_json = ?,
		       pending_feats          = pending_feats          + ?,
		       pending_skill_points   = pending_skill_points   + ?,
		       pending_ability_bumps  = pending_ability_bumps  + ?,
		       pending_weaves         = pending_weaves         + ?
		 WHERE id = ?`,
		f.HPCurrent, f.HPMax, f.BAB,
		f.Saves.Fort, f.Saves.Ref, f.Saves.Will,
		js,
		f.PendingFeatsDelta, f.PendingSkillPointsDelta,
		f.PendingAbilityBumpsDelta, f.PendingWeavesDelta,
		id,
	)
	if err != nil {
		return fmt.Errorf("record level up: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *SQLiteCharacterRepo) RecordPvP(ctx context.Context, id int64, on bool) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters SET pvp = ? WHERE id = ?`,
		boolToInt(on), id,
	)
	if err != nil {
		return fmt.Errorf("record pvp: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *SQLiteCharacterRepo) RecordAffects(ctx context.Context, id int64, affects []creature.Affect) error {
	js, err := marshalJSONSlice(affects)
	if err != nil {
		return fmt.Errorf("marshal affects: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters SET affects_json = ? WHERE id = ?`,
		js, id,
	)
	if err != nil {
		return fmt.Errorf("record affects: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *SQLiteCharacterRepo) RecordChanneling(ctx context.Context, id int64, c *creature.Channeling) error {
	js, err := jsonMarshalString(c)
	if err != nil {
		return fmt.Errorf("marshal channeling: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters SET channeling_json = ? WHERE id = ?`,
		js, id,
	)
	if err != nil {
		return fmt.Errorf("record channeling: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

func (r *SQLiteCharacterRepo) MarkNewsSeen(ctx context.Context, id int64, when time.Time) error {
	if when.IsZero() {
		// Defensive: a zero time would store the "never seen" sentinel
		// and unread every entry on next login. Refuse rather than
		// regress the watermark.
		return nil
	}
	// max(stored, ?) clamp at the SQL level — stale entries can't
	// regress the watermark even under racing reads.
	res, err := r.db.ExecContext(ctx,
		`UPDATE characters
		    SET last_news_seen = MAX(last_news_seen, ?)
		  WHERE id = ?`,
		when.Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("mark news seen: %w", err)
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

func (r *SQLiteCharacterRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM characters WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete character: %w", err)
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
		c            Character
		lastPlayed   sql.NullTime
		j            characterJSON
		coinCP       int64
		bankCP       int64
		fatigue      sql.NullTime
		idle         sql.NullTime
		login        sql.NullTime
		newsSeenSecs int64
		// channeling_json (migration 0033) was added with
		// NOT NULL DEFAULT 'null', but rows imported / migrated from
		// pre-0033 DBs in the wild can still hold an actual SQL NULL.
		// Scan into a nullable so a rogue NULL doesn't take the whole
		// account offline at login (postAuth.ListByAccount). Empty /
		// invalid values fold back to "" which jsonUnmarshalString
		// treats as a no-op (Channeling stays nil — correct for
		// non-channeler classes and the implicit pre-0033 default).
		channelingNS sql.NullString
		pvpInt       int
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
		&j.channelSettings, &channelingNS,
		&newsSeenSecs, &pvpInt,
		&c.PendingFeats, &c.PendingSkillPoints, &c.PendingAbilityBumps, &c.PendingWeaves,
		&c.XPDebt)...)

	if err := s.Scan(dest...); err != nil {
		return Character{}, err
	}
	if channelingNS.Valid {
		j.channeling = channelingNS.String
	}
	// Defense-in-depth: a corrupt row with auth_level outside the
	// known enum range would silently amplify privilege when stamped
	// onto the session in postauth. Reject explicitly.
	if c.AuthLevel > AuthLevelMax {
		return Character{}, fmt.Errorf("scan character: invalid auth_level %d", c.AuthLevel)
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
	if newsSeenSecs > 0 {
		c.LastNewsSeen = time.Unix(newsSeenSecs, 0).UTC()
	}
	c.PvP = pvpInt != 0
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
	channeling                                    string
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
	if j.channeling, err = jsonMarshalString(c.Channeling); err != nil {
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
	if err := jsonUnmarshalString(j.channeling, &c.Channeling); err != nil {
		return err
	}
	return nil
}
