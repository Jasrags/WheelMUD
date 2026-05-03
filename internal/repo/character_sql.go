package repo

import (
	"database/sql"
	"time"
)

// Column lists for the characters table. Ordering is load-bearing —
// charCoreColumns and charCoreValues / charCoreScanDest must stay
// in lock-step, same contract as coreColumns in creature_sql.go.
//
// Differs from coreColumns (used by mob_templates) on three axes:
//
//  1. Adds hp_current / subdual / conditions / position_flags /
//     affects_json — characters are always "live" creatures, so
//     these instance-style fields live on the row too.
//  2. Splits out as a separate constant rather than reusing
//     coreColumns + extras, because the column ORDER differs from
//     0008 and embedding a different sequence with the same name
//     would silently corrupt scans.
//  3. Excludes mob-template-only fields (challenge_code, behavior,
//     shadow_*, loot, dialogue_tree, shopkeeper, etc.).
const charCoreColumns = `size, type, gender, alignment,
		str_cur, str_max, str_inh,
		dex_cur, dex_max, dex_inh,
		con_cur, con_max, con_inh,
		int_cur, int_max, int_inh,
		wis_cur, wis_max, wis_inh,
		cha_cur, cha_max, cha_inh,
		hp_current, hp_max, subdual, hit_dice,
		defense, save_fort, save_ref, save_will, init_mod, bab,
		speed_base_ft, speed_climb_ft, speed_fly_ft,
		speed_fly_maneuver, speed_swim_ft, speed_burrow_ft,
		reach_ft, face_ft, threat_ft,
		conditions, position_flags, specials,
		dr_json, resists_json, affects_json`

// charPlayerColumns covers everything migration 0009 added beyond
// the Core stat block — race / class / wealth / reputation / idle
// timers / bound room / play-time / JSON-encoded catalogs.
const charPlayerColumns = `race, background, class_levels_json,
		xp, feats_json, skills_json, practice_points, class_features_json,
		height_cm, weight_kg, age, handedness,
		fame, infamy, infamy_share,
		coin_cp, bank_cp,
		encumbrance, fatigue_until, position, idle_since,
		bound_room_id, played_seconds, last_login,
		quest_log_json, dialogue_state_json, equipment_json, inventory_json,
		channel_settings_json,
		auth_level`

// charCoreValues returns the bound-parameter slice for the Core
// columns in INSERT order. Caller marshals the JSON columns once
// and passes the strings.
func charCoreValues(c Character, drJSON, resistsJSON, affectsJSON string) []any {
	core := c.Core
	return []any{
		core.Size, core.Type, core.Gender, core.Alignment,
		core.Abilities.Str.Current, core.Abilities.Str.Max, core.Abilities.Str.Inherent,
		core.Abilities.Dex.Current, core.Abilities.Dex.Max, core.Abilities.Dex.Inherent,
		core.Abilities.Con.Current, core.Abilities.Con.Max, core.Abilities.Con.Inherent,
		core.Abilities.Int.Current, core.Abilities.Int.Max, core.Abilities.Int.Inherent,
		core.Abilities.Wis.Current, core.Abilities.Wis.Max, core.Abilities.Wis.Inherent,
		core.Abilities.Cha.Current, core.Abilities.Cha.Max, core.Abilities.Cha.Inherent,
		core.HPCurrent, core.HPMax, core.Subdual, core.HitDice,
		core.Defense, core.Saves.Fort, core.Saves.Ref, core.Saves.Will, core.InitMod, core.BAB,
		core.Speed.BaseFt, core.Speed.ClimbFt, core.Speed.FlyFt,
		core.Speed.FlyManeuver, core.Speed.SwimFt, core.Speed.BurrowFt,
		core.ReachFt, core.FaceFt, core.ThreatFt,
		core.Conditions, core.Position, core.Specials,
		drJSON, resistsJSON, affectsJSON,
	}
}

// charCoreScanDest returns pointers in the same order as
// charCoreColumns. JSON columns scan into the supplied string
// pointers; caller unmarshals them after rows.Scan.
func charCoreScanDest(c *Character, drJSON, resistsJSON, affectsJSON *string) []any {
	core := &c.Core
	return []any{
		&core.Size, &core.Type, &core.Gender, &core.Alignment,
		&core.Abilities.Str.Current, &core.Abilities.Str.Max, &core.Abilities.Str.Inherent,
		&core.Abilities.Dex.Current, &core.Abilities.Dex.Max, &core.Abilities.Dex.Inherent,
		&core.Abilities.Con.Current, &core.Abilities.Con.Max, &core.Abilities.Con.Inherent,
		&core.Abilities.Int.Current, &core.Abilities.Int.Max, &core.Abilities.Int.Inherent,
		&core.Abilities.Wis.Current, &core.Abilities.Wis.Max, &core.Abilities.Wis.Inherent,
		&core.Abilities.Cha.Current, &core.Abilities.Cha.Max, &core.Abilities.Cha.Inherent,
		&core.HPCurrent, &core.HPMax, &core.Subdual, &core.HitDice,
		&core.Defense, &core.Saves.Fort, &core.Saves.Ref, &core.Saves.Will, &core.InitMod, &core.BAB,
		&core.Speed.BaseFt, &core.Speed.ClimbFt, &core.Speed.FlyFt,
		&core.Speed.FlyManeuver, &core.Speed.SwimFt, &core.Speed.BurrowFt,
		&core.ReachFt, &core.FaceFt, &core.ThreatFt,
		&core.Conditions, &core.Position, &core.Specials,
		drJSON, resistsJSON, affectsJSON,
	}
}

// charPlayerValues returns the bound-parameter slice for the
// player-only columns. Time fields use sql.NullTime so an unset
// FatigueUntil / IdleSince / LastLogin maps to NULL rather than
// the zero time.
func charPlayerValues(c Character, classLevelsJSON, featsJSON, skillsJSON, classFeaturesJSON,
	questLogJSON, dialogueStateJSON, equipmentJSON, inventoryJSON, channelSettingsJSON string,
) []any {
	return []any{
		c.Race, c.Background, classLevelsJSON,
		c.XP, featsJSON, skillsJSON, c.PracticePoints, classFeaturesJSON,
		c.HeightCm, c.WeightKg, c.Age, c.Handedness,
		c.Fame, c.Infamy, c.InfamyShare,
		int64(c.Coin), int64(c.BankBalance),
		c.Encumbrance, nullTime(c.FatigueUntil), c.Position, nullTime(c.IdleSince),
		c.BoundRoomID, c.PlayedSeconds, nullTime(c.LastLogin),
		questLogJSON, dialogueStateJSON, equipmentJSON, inventoryJSON,
		channelSettingsJSON,
		c.AuthLevel,
	}
}

// charPlayerScanDest returns pointers in the same order as
// charPlayerColumns. Caller unmarshals JSON strings + assigns the
// nullable time slots back into the Character after Scan.
func charPlayerScanDest(c *Character,
	classLevelsJSON, featsJSON, skillsJSON, classFeaturesJSON *string,
	coinCP, bankCP *int64,
	fatigueUntil, idleSince, lastLogin *sql.NullTime,
	questLogJSON, dialogueStateJSON, equipmentJSON, inventoryJSON, channelSettingsJSON *string,
) []any {
	return []any{
		&c.Race, &c.Background, classLevelsJSON,
		&c.XP, featsJSON, skillsJSON, &c.PracticePoints, classFeaturesJSON,
		&c.HeightCm, &c.WeightKg, &c.Age, &c.Handedness,
		&c.Fame, &c.Infamy, &c.InfamyShare,
		coinCP, bankCP,
		&c.Encumbrance, fatigueUntil, &c.Position, idleSince,
		&c.BoundRoomID, &c.PlayedSeconds, lastLogin,
		questLogJSON, dialogueStateJSON, equipmentJSON, inventoryJSON,
		channelSettingsJSON,
		&c.AuthLevel,
	}
}

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}
