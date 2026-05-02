package repo

import (
	"encoding/json"
	"fmt"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// Helpers shared by mob_template_sqlite.go and mob_instance_sqlite.go
// for marshalling Core stat-block fields and JSON columns to/from
// SQLite rows. Kept private to the package — repos are the only
// boundary that cares about column shape.

// jsonMarshalString is encoding/json.Marshal returning a string,
// for the JSON-blob columns on mob_templates / mob_instances /
// channeling. Used for non-slice values (ShopConfig pointer,
// equipment struct).
func jsonMarshalString(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}
	return string(b), nil
}

func jsonUnmarshalString(s string, dst any) error {
	if s == "" || s == "null" {
		return nil
	}
	if err := json.Unmarshal([]byte(s), dst); err != nil {
		return fmt.Errorf("unmarshal json: %w", err)
	}
	return nil
}

// marshalJSON returns "[]"/"{}" for empty values rather than "null"
// so SQLite NOT NULL DEFAULT columns stay consistent.
func marshalJSONSlice[T any](v []T) (string, error) {
	if len(v) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}
	return string(b), nil
}

func unmarshalJSONSlice[T any](s string, dst *[]T) error {
	if s == "" || s == "[]" || s == "null" {
		*dst = nil
		return nil
	}
	if err := json.Unmarshal([]byte(s), dst); err != nil {
		return fmt.Errorf("unmarshal json: %w", err)
	}
	return nil
}

// coreColumns is the column list for every Core field on
// mob_templates. Matches 0008_create_creatures.sql column order so
// scanCoreFromTemplate can read SELECT * positionally without a
// brittle map. Excludes id/external_id/name/name_lower/created_at/
// short_desc/long_desc which the caller binds itself.
const coreColumns = `size, type, gender, alignment,
		str_cur, str_max, str_inh,
		dex_cur, dex_max, dex_inh,
		con_cur, con_max, con_inh,
		int_cur, int_max, int_inh,
		wis_cur, wis_max, wis_inh,
		cha_cur, cha_max, cha_inh,
		hp_max, hit_dice,
		defense, save_fort, save_ref, save_will, init_mod, bab,
		speed_base_ft, speed_climb_ft, speed_fly_ft,
		speed_fly_maneuver, speed_swim_ft, speed_burrow_ft,
		reach_ft, face_ft, threat_ft,
		specials,
		dr_json, resists_json`

// coreValues returns the bound-parameter slice in the same order
// as coreColumns for an INSERT.
func coreValues(c creature.Core, drJSON, resistsJSON string) []any {
	return []any{
		c.Size, c.Type, c.Gender, c.Alignment,
		c.Abilities.Str.Current, c.Abilities.Str.Max, c.Abilities.Str.Inherent,
		c.Abilities.Dex.Current, c.Abilities.Dex.Max, c.Abilities.Dex.Inherent,
		c.Abilities.Con.Current, c.Abilities.Con.Max, c.Abilities.Con.Inherent,
		c.Abilities.Int.Current, c.Abilities.Int.Max, c.Abilities.Int.Inherent,
		c.Abilities.Wis.Current, c.Abilities.Wis.Max, c.Abilities.Wis.Inherent,
		c.Abilities.Cha.Current, c.Abilities.Cha.Max, c.Abilities.Cha.Inherent,
		c.HPMax, c.HitDice,
		c.Defense, c.Saves.Fort, c.Saves.Ref, c.Saves.Will, c.InitMod, c.BAB,
		c.Speed.BaseFt, c.Speed.ClimbFt, c.Speed.FlyFt,
		c.Speed.FlyManeuver, c.Speed.SwimFt, c.Speed.BurrowFt,
		c.ReachFt, c.FaceFt, c.ThreatFt,
		c.Specials,
		drJSON, resistsJSON,
	}
}

// coreScanDest returns a pointer slice in the same order as
// coreColumns, plus output slots for the JSON strings the caller
// will unmarshal after the row scan. Caller passes the returned
// pointers to rows.Scan.
func coreScanDest(c *creature.Core, drJSON, resistsJSON *string) []any {
	return []any{
		&c.Size, &c.Type, &c.Gender, &c.Alignment,
		&c.Abilities.Str.Current, &c.Abilities.Str.Max, &c.Abilities.Str.Inherent,
		&c.Abilities.Dex.Current, &c.Abilities.Dex.Max, &c.Abilities.Dex.Inherent,
		&c.Abilities.Con.Current, &c.Abilities.Con.Max, &c.Abilities.Con.Inherent,
		&c.Abilities.Int.Current, &c.Abilities.Int.Max, &c.Abilities.Int.Inherent,
		&c.Abilities.Wis.Current, &c.Abilities.Wis.Max, &c.Abilities.Wis.Inherent,
		&c.Abilities.Cha.Current, &c.Abilities.Cha.Max, &c.Abilities.Cha.Inherent,
		&c.HPMax, &c.HitDice,
		&c.Defense, &c.Saves.Fort, &c.Saves.Ref, &c.Saves.Will, &c.InitMod, &c.BAB,
		&c.Speed.BaseFt, &c.Speed.ClimbFt, &c.Speed.FlyFt,
		&c.Speed.FlyManeuver, &c.Speed.SwimFt, &c.Speed.BurrowFt,
		&c.ReachFt, &c.FaceFt, &c.ThreatFt,
		&c.Specials,
		drJSON, resistsJSON,
	}
}
