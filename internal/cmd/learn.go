package cmd

// learn is the spend verb for pending_skill_points (Phase E #24).
// Players type `learn` to see the menu of skills they can put ranks
// into, and `learn <skill> [n]` to invest. The pool was deposited by
// `train` (slice 4) on level-up; this verb is the symmetric spend
// path.
//
// V1 scope:
//   - Allowed list = union of class-skills (across all ClassLevels
//     keys) ∪ background skills. Cross-class picks are deferred,
//     mirroring the chargen V1 stance (chargen_features_followups.md).
//   - 1 pending point per rank. Cross-class half-rate is deferred.
//   - Per-skill cap = characterLevel + 3.
//   - No trainer NPC required — learn works anywhere.
//
// Refusals (skill not allowed, over-cap, over-budget, unknown id) do
// NOT mutate or audit. Successful spends write one admin_audit row.

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
			allowed := allowedSkillIDsFor(char, cat)

			if len(args) == 0 {
				return writeLearnMenu(s, char, allowed, cat)
			}

			// `learn info <skill|#>` — read-only descriptor.
			if strings.EqualFold(args[0], "info") || strings.EqualFold(args[0], "i") {
				if len(args) < 2 {
					return s.WriteString("{{Type 'learn info <skill>' for a skill description.}}::yellow\r\n")
				}
				return writeLearnInfo(s, args[1], allowed, cat)
			}

			// `learn <skill|#> [n]` — spend.
			id, ok := matchSkillToken(args[0], allowed, cat)
			if !ok {
				return s.WriteString("{{That skill is not available to you.}}::yellow\r\n")
			}
			n := 1
			if len(args) >= 2 {
				v, err := strconv.Atoi(args[1])
				if err != nil || v < 1 {
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
	allowed []string, cat *chargen.Catalog,
) error {
	if err := display.SectionHeader(s, "Skill Training"); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Skill points",
		strconv.Itoa(int(char.PendingSkillPoints)), 14); err != nil {
		return err
	}
	skillCap := skillRankCap(char)
	if err := display.FieldRow(s, "Per-skill cap",
		strconv.Itoa(skillCap), 14); err != nil {
		return err
	}
	if len(allowed) == 0 {
		return s.WriteString("\r\n  {{(no class or background skills available)}}::gray\r\n")
	}
	if err := display.Subsection(s, "Available skills"); err != nil {
		return err
	}
	bgSet := backgroundSkillSet(char, cat)
	for i, id := range allowed {
		sk, _ := cat.Skill(id)
		name := id
		ability := ""
		if sk != nil {
			name = sk.Name
			ability = sk.Ability
		}
		ranks := char.Skills[chargen.HashID(id)].Ranks
		tag := "[class]"
		if _, ok := bgSet[id]; ok {
			tag = "[bg]"
		}
		if err := s.WriteString(fmt.Sprintf(
			"  {{%2d)}}::gray {{%-22s}}::yellow|bold {{%-3s}}::gray  ranks={{%d}}::green|bold/%d %s\r\n",
			i+1, display.Defang(name, ""), display.Defang(ability, ""),
			ranks, skillCap, tag,
		)); err != nil {
			return err
		}
	}
	return s.WriteString(
		"\r\n  Usage: {{learn <skill> [n]}}::yellow|bold  ·  " +
			"{{learn info <skill>}}::yellow\r\n",
	)
}

// writeLearnInfo prints the descriptor for one skill. Read-only.
func writeLearnInfo(s *telnet.Session, token string, allowed []string,
	cat *chargen.Catalog,
) error {
	id, ok := matchSkillToken(token, allowed, cat)
	if !ok {
		return s.WriteString("{{That skill is not available to you.}}::yellow\r\n")
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
// success it writes a confirmation line and audits.
func commitLearn(c *telnet.Context, characters repo.CharacterRepo,
	audits repo.AdminAuditRepo, char repo.Character, cat *chargen.Catalog,
	skillID string, n int,
) error {
	s := c.Session
	skillCap := skillRankCap(char)
	key := chargen.HashID(skillID)
	cur := char.Skills[key].Ranks
	target := int(cur) + n
	if target > skillCap {
		return s.WriteString(fmt.Sprintf(
			"{{That would push %s past your cap of %d.}}::yellow\r\n",
			display.Defang(skillID, ""), skillCap))
	}
	cost := int32(n) // 1pt per rank, V1
	if cost > char.PendingSkillPoints {
		return s.WriteString(fmt.Sprintf(
			"{{Not enough skill points: need %d, have %d.}}::yellow\r\n",
			cost, char.PendingSkillPoints))
	}
	newPending := char.PendingSkillPoints - cost
	if err := characters.RecordSkillRank(c.Ctx, char.ID, key,
		int8(target), true, newPending); err != nil {
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

// allowedSkillIDsFor returns the union of class-skills (across every
// class in ClassLevels) and background skills, deduped, stable-sorted.
// Mirrors chargen_features.go::allowedSkillIDs but reads from the
// persisted Character rather than chargen draft state.
func allowedSkillIDsFor(ch repo.Character, cat *chargen.Catalog) []string {
	if cat == nil {
		return nil
	}
	seen := make(map[string]struct{}, 16)
	for k := range ch.ClassLevels {
		for _, cl := range cat.Classes() {
			if cl.Enum != k {
				continue
			}
			for _, id := range cl.ClassSkills {
				seen[id] = struct{}{}
			}
		}
	}
	for id := range backgroundSkillSet(ch, cat) {
		seen[id] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
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
// or string id, case-insensitive) against the allowed list. Returns
// the canonical catalog id on hit.
func matchSkillToken(token string, allowed []string, cat *chargen.Catalog) (string, bool) {
	if token == "" {
		return "", false
	}
	if n, err := strconv.Atoi(token); err == nil {
		if n >= 1 && n <= len(allowed) {
			return allowed[n-1], true
		}
		return "", false
	}
	t := strings.ToLower(token)
	for _, id := range allowed {
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

// skillRankCap is the per-skill ceiling for a class skill at the
// character's total level (sum of ClassLevels). d20 baseline:
// level + 3.
func skillRankCap(ch repo.Character) int {
	return characterLevel(ch) + 3
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
