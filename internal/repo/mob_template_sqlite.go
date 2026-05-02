package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

type SQLiteMobTemplateRepo struct {
	db *sql.DB
}

func NewSQLiteMobTemplateRepo(db *sql.DB) *SQLiteMobTemplateRepo {
	return &SQLiteMobTemplateRepo{db: db}
}

// templateExtraColumns covers everything beyond the shared Core
// block — challenge code, behavior, loot/dialogue, Shadowspawn
// fields, descriptions. Order matches 0008_create_creatures.sql.
const templateExtraColumns = `challenge_code, organization, behavior_flags,
		loot_table_id, gold_dice, dialogue_tree_id, shopkeeper_json,
		corpse_decay_ticks, respawn_zone_reset_id,
		shadow_link_myrddraal_id, taint_immune, fade_link_master_ticks,
		short_desc, long_desc,
		natural_attacks_json, special_attacks_json, traits_json,
		advancement_json, climate_json, terrain_json, trigger_scripts_json`

func (r *SQLiteMobTemplateRepo) Create(ctx context.Context, t creature.MobTemplate) (creature.MobTemplate, error) {
	if t.ExternalID == "" {
		return creature.MobTemplate{}, ErrInvalidExternalID
	}

	drJSON, err := marshalJSONSlice(t.Core.DR)
	if err != nil {
		return creature.MobTemplate{}, err
	}
	resistsJSON, err := marshalJSONSlice(t.Core.Resists)
	if err != nil {
		return creature.MobTemplate{}, err
	}
	naturalJSON, err := marshalJSONSlice(t.NaturalAttacks)
	if err != nil {
		return creature.MobTemplate{}, err
	}
	specialJSON, err := marshalJSONSlice(t.SpecialAttacks)
	if err != nil {
		return creature.MobTemplate{}, err
	}
	traitsJSON, err := marshalJSONSlice(t.Traits)
	if err != nil {
		return creature.MobTemplate{}, err
	}
	advancementJSON, err := marshalJSONSlice(t.Advancement)
	if err != nil {
		return creature.MobTemplate{}, err
	}
	climateJSON, err := marshalJSONSlice(t.Climate)
	if err != nil {
		return creature.MobTemplate{}, err
	}
	terrainJSON, err := marshalJSONSlice(t.Terrain)
	if err != nil {
		return creature.MobTemplate{}, err
	}
	scriptsJSON, err := marshalJSONSlice(t.TriggerScripts)
	if err != nil {
		return creature.MobTemplate{}, err
	}

	var shopJSON sql.NullString
	if t.ShopkeeperConfig != nil {
		s, err := jsonMarshalString(t.ShopkeeperConfig)
		if err != nil {
			return creature.MobTemplate{}, err
		}
		shopJSON = sql.NullString{String: s, Valid: true}
	}

	challengeCode := string(t.ChallengeCode)
	if challengeCode == "" {
		challengeCode = "A"
	}

	args := []any{t.ExternalID, t.Core.Name, strings.ToLower(t.Core.Name)}
	args = append(args, coreValues(t.Core, drJSON, resistsJSON)...)
	args = append(args,
		challengeCode, t.Organization, t.BehaviorFlags,
		t.LootTableID, t.GoldDice, t.DialogueTreeID, shopJSON,
		t.CorpseDecayTicks, t.RespawnZoneResetID,
		t.ShadowLinkMyrddraalID, boolToInt(t.TaintImmune), int64(t.FadeOnLinkMasterTimer),
		t.ShortDesc, t.LongDesc,
		naturalJSON, specialJSON, traitsJSON,
		advancementJSON, climateJSON, terrainJSON, scriptsJSON,
		time.Now().UTC(),
	)

	query := fmt.Sprintf(
		`INSERT INTO mob_templates(external_id, name, name_lower, %s, %s, created_at)
		 VALUES (%s)`,
		coreColumns, templateExtraColumns, placeholders(len(args)),
	)
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		if isUniqueViolation(err) {
			return creature.MobTemplate{}, ErrDuplicateExternalID
		}
		return creature.MobTemplate{}, fmt.Errorf("insert mob_template: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return creature.MobTemplate{}, fmt.Errorf("last insert id: %w", err)
	}
	t.ID = id
	t.Core.ID = id
	return t, nil
}

func (r *SQLiteMobTemplateRepo) GetByID(ctx context.Context, id int64) (creature.MobTemplate, error) {
	return r.queryOne(ctx, "id = ?", id)
}

func (r *SQLiteMobTemplateRepo) GetByExternalID(ctx context.Context, externalID string) (creature.MobTemplate, error) {
	return r.queryOne(ctx, "external_id = ?", externalID)
}

func (r *SQLiteMobTemplateRepo) queryOne(ctx context.Context, where string, arg any) (creature.MobTemplate, error) {
	query := fmt.Sprintf(
		`SELECT id, external_id, name, %s, %s
		 FROM mob_templates WHERE %s`,
		coreColumns, templateExtraColumns, where,
	)
	var (
		t                                                      creature.MobTemplate
		drJSON, resistsJSON                                    string
		challengeCode                                          string
		shopJSON                                               sql.NullString
		taintImmune                                            int
		fadeTicks                                              int64
		naturalJSON, specialJSON, traitsJSON                   string
		advancementJSON, climateJSON, terrainJSON, scriptsJSON string
	)
	dest := []any{&t.ID, &t.ExternalID, &t.Core.Name}
	dest = append(dest, coreScanDest(&t.Core, &drJSON, &resistsJSON)...)
	dest = append(dest,
		&challengeCode, &t.Organization, &t.BehaviorFlags,
		&t.LootTableID, &t.GoldDice, &t.DialogueTreeID, &shopJSON,
		&t.CorpseDecayTicks, &t.RespawnZoneResetID,
		&t.ShadowLinkMyrddraalID, &taintImmune, &fadeTicks,
		&t.ShortDesc, &t.LongDesc,
		&naturalJSON, &specialJSON, &traitsJSON,
		&advancementJSON, &climateJSON, &terrainJSON, &scriptsJSON,
	)

	err := r.db.QueryRowContext(ctx, query, arg).Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return creature.MobTemplate{}, ErrTemplateNotFound
	}
	if err != nil {
		return creature.MobTemplate{}, fmt.Errorf("scan mob_template: %w", err)
	}

	if challengeCode != "" {
		t.ChallengeCode = creature.ChallengeCode(rune(challengeCode[0]))
	}
	t.TaintImmune = taintImmune != 0
	t.FadeOnLinkMasterTimer = time.Duration(fadeTicks)
	t.Core.ID = t.ID

	if err := unmarshalJSONSlice(drJSON, &t.Core.DR); err != nil {
		return creature.MobTemplate{}, err
	}
	if err := unmarshalJSONSlice(resistsJSON, &t.Core.Resists); err != nil {
		return creature.MobTemplate{}, err
	}
	if shopJSON.Valid {
		var sc creature.ShopConfig
		if err := jsonUnmarshalString(shopJSON.String, &sc); err != nil {
			return creature.MobTemplate{}, err
		}
		t.ShopkeeperConfig = &sc
	}
	if err := unmarshalJSONSlice(naturalJSON, &t.NaturalAttacks); err != nil {
		return creature.MobTemplate{}, err
	}
	if err := unmarshalJSONSlice(specialJSON, &t.SpecialAttacks); err != nil {
		return creature.MobTemplate{}, err
	}
	if err := unmarshalJSONSlice(traitsJSON, &t.Traits); err != nil {
		return creature.MobTemplate{}, err
	}
	if err := unmarshalJSONSlice(advancementJSON, &t.Advancement); err != nil {
		return creature.MobTemplate{}, err
	}
	if err := unmarshalJSONSlice(climateJSON, &t.Climate); err != nil {
		return creature.MobTemplate{}, err
	}
	if err := unmarshalJSONSlice(terrainJSON, &t.Terrain); err != nil {
		return creature.MobTemplate{}, err
	}
	if err := unmarshalJSONSlice(scriptsJSON, &t.TriggerScripts); err != nil {
		return creature.MobTemplate{}, err
	}
	return t, nil
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ", ")
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
