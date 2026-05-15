package cmd

// learn is the spend verb for pending_skill_points (Phase E #24).
// Players type `learn` to see the menu of skills they can put ranks
// into, and `learn <skill> [n]` to invest. The pool was deposited by
// `train` (slice 4) on level-up; this verb is the symmetric spend
// path.
//
// Scope:
//   - Class skills (union of class-skills across all ClassLevels keys)
//     and background skills cost 1 pending point per rank, cap =
//     characterLevel + 3, persisted with IsClassSkill=true.
//   - Cross-class skills (any other catalog skill) cost 2 pending
//     points per rank, cap = (characterLevel + 3) / 2 (floor),
//     persisted with IsClassSkill=false. d20 baseline.
//   - No trainer NPC required — learn works anywhere.
//
// Refusals (over-cap, over-budget, unknown id) do NOT mutate or audit.
// Successful spends write one admin_audit row.

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// maxLearnRanksPerCall bounds the player-supplied `n` ranks argument
// so the int32 cost multiplication can never overflow regardless of
// platform `int` width. The per-skill cap (≤ 23 at level 20) is the
// real ceiling; this is defense-in-depth.
const maxLearnRanksPerCall = 1000

// NewLearn builds the `learn` verb.
func NewLearn(characters repo.CharacterRepo, cat *chargen.Catalog,
	audits repo.AdminAuditRepo,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo,
	weaveTeachers repo.WeaveTeacherRepo,
) *telnet.Command {
	return &telnet.Command{
		Name: "learn",
		Help: "Learn — spend pending skill points on class/background skills",
		Long: "Usage: learn                       show skill menu\n" +
			"       learn <skill> [n]           put n ranks (default 1) into <skill>\n" +
			"       learn info <skill>          show description for <skill>\n" +
			"       learn weave                 show channeler weave menu\n" +
			"       learn weave <id>            spend a pending weave on <id>\n" +
			"       learn weave info <id>       show description for a weave",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			args := c.Args

			// `learn weave …` delegates to the channeler-spend path
			// in learn_weave.go. That helper does its own char lookup
			// + Channeling gate.
			if len(args) >= 1 && strings.EqualFold(args[0], "weave") {
				return runLearnWeave(c, characters, cat, audits, mobs, templates, weaveTeachers)
			}

			if cat == nil {
				return s.WriteString("{{Skill catalog unavailable.}}::red\r\n")
			}
			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("learn: char lookup", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't find your records.}}::red\r\n")
			}
			all := allLearnableSkillIDs(cat)

			if len(args) == 0 {
				return writeLearnMenu(s, char, all, cat)
			}

			// `learn info <skill|#>` — read-only descriptor.
			if strings.EqualFold(args[0], "info") || strings.EqualFold(args[0], "i") {
				if len(args) < 2 {
					return s.WriteString("{{Type 'learn info <skill>' for a skill description.}}::yellow\r\n")
				}
				return writeLearnInfo(s, args[1], char, all, cat)
			}

			// `learn <skill|#> [n]` — spend.
			id, ok := matchSkillToken(args[0], all, cat)
			if !ok {
				return s.WriteString("{{No such skill.}}::yellow\r\n")
			}
			n := 1
			if len(args) >= 2 {
				v, err := strconv.Atoi(args[1])
				// Upper bound is well above any plausible cap (class
				// cap at L20 = 23) and keeps `int32(n)*costPerRank`
				// far from int32 overflow on every platform.
				if err != nil || v < 1 || v > maxLearnRanksPerCall {
					return s.WriteString("{{Type a positive number of ranks.}}::yellow\r\n")
				}
				n = v
			}
			return commitLearn(c, characters, audits, char, cat, id, n)
		},
	}
}

// writeLearnMenu renders the spendable-skill list. Reuses the
// internal/display helpers (SectionHeader / Subsection / FieldRow) so
// the look matches `score` and the chargen review.
func writeLearnMenu(s *telnet.Session, char repo.Character,
	all []string, cat *chargen.Catalog,
) error {
	if err := display.SectionHeader(s, "Skill Training"); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Skill points",
		strconv.Itoa(int(char.PendingSkillPoints)), 14); err != nil {
		return err
	}
	classCap := classSkillRankCap(char)
	crossCap := crossClassSkillRankCap(char)
	if err := display.FieldRow(s, "Per-skill cap",
		fmt.Sprintf("%d (class) / %d (cross)", classCap, crossCap), 14); err != nil {
		return err
	}
	if len(all) == 0 {
		return s.WriteString("\r\n  {{(no skills in catalog)}}::gray\r\n")
	}
	if err := display.Subsection(s, "Available skills"); err != nil {
		return err
	}
	classSet := classSkillSet(char, cat)
	bgSet := backgroundSkillSet(char, cat)
	for i, id := range all {
		sk, _ := cat.Skill(id)
		name := id
		ability := ""
		if sk != nil {
			name = sk.Name
			ability = sk.Ability
		}
		ranks := char.Skills[chargen.HashID(id)].Ranks
		var tag string
		var rankCap int
		switch {
		case isClassOrBackground(id, classSet, bgSet):
			rankCap = classCap
			if _, ok := bgSet[id]; ok {
				tag = "[bg]"
			} else {
				tag = "[class]"
			}
		default:
			rankCap = crossCap
			tag = "[cross]"
		}
		if err := s.WriteString(fmt.Sprintf(
			"  {{%2d)}}::gray {{%-22s}}::yellow|bold {{%-3s}}::gray  ranks={{%d}}::green|bold/%d %s\r\n",
			i+1, display.Defang(name, ""), display.Defang(ability, ""),
			ranks, rankCap, tag,
		)); err != nil {
			return err
		}
	}
	return s.WriteString(
		"\r\n  Usage: {{learn <skill> [n]}}::yellow|bold  ·  " +
			"{{learn info <skill>}}::yellow\r\n",
	)
}

// writeLearnInfo prints the descriptor for one skill plus the per-
// character cost and cap for that bucket. Read-only.
func writeLearnInfo(s *telnet.Session, token string, char repo.Character,
	all []string, cat *chargen.Catalog,
) error {
	id, ok := matchSkillToken(token, all, cat)
	if !ok {
		return s.WriteString("{{No such skill.}}::yellow\r\n")
	}
	sk, _ := cat.Skill(id)
	if sk == nil {
		return s.WriteString("{{No description on file.}}::yellow\r\n")
	}
	if err := s.WriteString(fmt.Sprintf(
		"{{%s (%s)}}::cyan|bold\r\n",
		display.Defang(sk.Name, ""), display.Defang(sk.ID, ""),
	)); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Key ability", abilityFullName(sk.Ability), 14); err != nil {
		return err
	}
	isClass := isClassOrBackgroundSkill(char, cat, id)
	cost, rankCap := skillCostAndCap(char, isClass)
	bucket := "cross-class"
	if isClass {
		bucket = "class"
	}
	if err := display.FieldRow(s, "Cost / rank",
		fmt.Sprintf("%d pt (%s)", cost, bucket), 14); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Rank cap",
		strconv.Itoa(rankCap), 14); err != nil {
		return err
	}
	if sk.Description != "" {
		if err := s.WriteString("\r\n"); err != nil {
			return err
		}
		if err := s.WriteWrapped(strings.TrimRight(sk.Description, "\n")); err != nil {
			return err
		}
		if err := s.WriteString("\r\n"); err != nil {
			return err
		}
	}
	return display.Rule(s)
}

// commitLearn enforces cap + budget then calls RecordSkillRank. On
// success it writes a confirmation line and audits. Class-skill picks
// cost 1 pending point per rank at cap level+3; cross-class picks
// cost 2 pending points per rank at cap (level+3)/2.
func commitLearn(c *telnet.Context, characters repo.CharacterRepo,
	audits repo.AdminAuditRepo, char repo.Character, cat *chargen.Catalog,
	skillID string, n int,
) error {
	s := c.Session
	isClass := isClassOrBackgroundSkill(char, cat, skillID)
	costPerRank, skillCap := skillCostAndCap(char, isClass)
	key := chargen.HashID(skillID)
	cur := char.Skills[key].Ranks
	target := int(cur) + n
	if target > skillCap {
		return s.WriteString(fmt.Sprintf(
			"{{That would push %s past your cap of %d.}}::yellow\r\n",
			display.Defang(skillID, ""), skillCap))
	}
	cost := int32(n) * costPerRank
	if cost > char.PendingSkillPoints {
		return s.WriteString(fmt.Sprintf(
			"{{Not enough skill points: need %d, have %d.}}::yellow\r\n",
			cost, char.PendingSkillPoints))
	}
	newPending := char.PendingSkillPoints - cost
	if err := characters.RecordSkillRank(c.Ctx, char.ID, key,
		int8(target), isClass, newPending); err != nil {
		slog.Error("learn: record skill rank",
			"char", char.ID, "skill", skillID, "error", err)
		return s.WriteString("{{The lesson slips away as you reach for it.}}::red\r\n")
	}
	audit.Record(c.Ctx, audits, s, "learn", skillID,
		fmt.Sprintf("ranks=%d", target))

	sk, _ := cat.Skill(skillID)
	name := skillID
	if sk != nil {
		name = sk.Name
	}
	pts := "skill points remain"
	if newPending == 1 {
		pts = "skill point remains"
	}
	return s.WriteString(fmt.Sprintf(
		"{{You drill %s. (%d → %d)}}::green|bold  %d %s.\r\n",
		display.Defang(name, ""),
		cur, target, newPending, pts))
}

// allLearnableSkillIDs returns every catalog skill id, sorted, for
// the spend menu. Cross-class picks are routed to a higher cost and
// halved cap by commitLearn; the menu does not pre-filter.
func allLearnableSkillIDs(cat *chargen.Catalog) []string {
	if cat == nil {
		return nil
	}
	skills := cat.Skills()
	out := make([]string, 0, len(skills))
	for _, sk := range skills {
		if sk == nil {
			continue
		}
		out = append(out, sk.ID)
	}
	sort.Strings(out)
	return out
}

// classSkillSet returns the union of class-skills across every class
// in the character's ClassLevels map, as a set keyed by skill id.
func classSkillSet(ch repo.Character, cat *chargen.Catalog) map[string]struct{} {
	out := map[string]struct{}{}
	if cat == nil {
		return out
	}
	for k := range ch.ClassLevels {
		for _, cl := range cat.Classes() {
			if cl.Enum != k {
				continue
			}
			for _, id := range cl.ClassSkills {
				out[id] = struct{}{}
			}
		}
	}
	return out
}

// isClassOrBackground returns true if skill id is in either bucket.
func isClassOrBackground(id string, classSet, bgSet map[string]struct{}) bool {
	if _, ok := classSet[id]; ok {
		return true
	}
	_, ok := bgSet[id]
	return ok
}

// isClassOrBackgroundSkill is the per-character bucket check used at
// commit time. Returns true for class skills (any ClassLevels entry)
// and background skills; false for cross-class.
func isClassOrBackgroundSkill(ch repo.Character, cat *chargen.Catalog, id string) bool {
	return isClassOrBackground(id, classSkillSet(ch, cat), backgroundSkillSet(ch, cat))
}

// backgroundSkillSet returns the character's background skill ids as
// a set. Lookup tolerates nil/missing background.
func backgroundSkillSet(ch repo.Character, cat *chargen.Catalog) map[string]struct{} {
	out := map[string]struct{}{}
	if cat == nil {
		return out
	}
	for _, bg := range cat.Backgrounds() {
		if bg.Enum != ch.Background {
			continue
		}
		for _, id := range bg.BackgroundSkills {
			out[id] = struct{}{}
		}
		return out
	}
	return out
}

// matchSkillToken resolves a player-typed token (numeric menu index
// or string id / display name, case-insensitive) against the full
// learnable list. Returns the canonical catalog id on hit.
func matchSkillToken(token string, all []string, cat *chargen.Catalog) (string, bool) {
	if token == "" {
		return "", false
	}
	if n, err := strconv.Atoi(token); err == nil {
		if n >= 1 && n <= len(all) {
			return all[n-1], true
		}
		return "", false
	}
	t := strings.ToLower(token)
	for _, id := range all {
		if strings.EqualFold(id, t) {
			return id, true
		}
		sk, _ := cat.Skill(id)
		if sk != nil && strings.EqualFold(sk.Name, t) {
			return id, true
		}
	}
	return "", false
}

// classSkillRankCap is the per-skill ceiling for a class or background
// skill at the character's total level (sum of ClassLevels). d20
// baseline: level + 3.
func classSkillRankCap(ch repo.Character) int {
	return characterLevel(ch) + 3
}

// crossClassSkillRankCap is the per-skill ceiling for a cross-class
// skill. d20 baseline: floor((level + 3) / 2).
func crossClassSkillRankCap(ch repo.Character) int {
	return (characterLevel(ch) + 3) / 2
}

// skillCostAndCap returns the per-rank pending-point cost and per-
// skill rank ceiling for the character's chosen bucket. isClass true
// → (1, level+3); false → (2, (level+3)/2).
func skillCostAndCap(ch repo.Character, isClass bool) (int32, int) {
	if isClass {
		return 1, classSkillRankCap(ch)
	}
	return 2, crossClassSkillRankCap(ch)
}

// abilityFullName expands the YAML 3-letter ability token. Mirrors
// internal/mode/chargen_features.go::abilityDisplayName so the info
// view reads the same in both surfaces.
func abilityFullName(token string) string {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "":
		return "—"
	case "str":
		return "Strength"
	case "dex":
		return "Dexterity"
	case "con":
		return "Constitution"
	case "int":
		return "Intelligence"
	case "wis":
		return "Wisdom"
	case "cha":
		return "Charisma"
	}
	return token
}
