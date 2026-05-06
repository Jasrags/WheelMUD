package mode

// First-level feats and skills (#15 slice 1). Two substeps slot in
// between identity and review:
//
//	identity → feat → skills → review
//
// `feat` picks one background-restricted 1st-level feat from
// catalog.FeatsForBackground; bg.BonusFeats are auto-merged on commit
// (they're prerequisites, not choices).
//
// `skills` allocates (max(1, class.SkillPoints + IntMod) × 4) ranks
// across the class skill list plus the background's bonus skills.
// All allocatable skills are treated as class skills here — V1 does
// not let the player buy ranks in cross-class skills (caps at half
// rate, costs double; deferred until level-up needs the same plumbing
// in §12). Per-skill cap is 4 ranks (level + 3 at level 1).
//
// Channeler branch (Source / Affinities / starting weaves) and
// equipment-bundle spawning are deferred — see
// chargen_features_followups.md.

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/telnet"
)

const (
	// classSkillRankCapL1 is level+3 evaluated at level 1 — the per-skill
	// rank ceiling for class skills at first level.
	classSkillRankCapL1 = 4
	// minFirstLevelSkillPoints is the d20 floor: even an Int 8 fighter
	// (skill_points=2, IntMod=-1) gets 1 point/level → 4 at first.
	minFirstLevelSkillPoints = 4
)

// catalogIDInt32 stably derives an int32 key from a catalog string id
// via FNV-32a. The Character schema keys feats and skills by int32
// (creature.Character.Feats / .Skills) — until those tables are
// authored as proper enums, hashing the catalog id keeps the chargen
// output stable across runs without a manual numbering table.
//
// FNV-32 has tiny collision probability over our ~30-skill / ~25-feat
// catalogs; if a collision ever shows up at boot the catalog loader
// can grow a duplicate-id-hash check.
func catalogIDInt32(id string) int32 {
	h := fnv.New32a()
	h.Write([]byte(id))
	return int32(h.Sum32())
}

// firstLevelSkillBudget returns (max(1, class.SkillPoints + IntMod)) × 4.
// The min-1-per-level rule applies *before* multiplication — a low-Int
// armsman gets 1 point/level → 4 at first, never 0.
func firstLevelSkillBudget(classPoints int, intMod int) int {
	per := classPoints + intMod
	if per < 1 {
		per = 1
	}
	total := per * 4
	if total < minFirstLevelSkillPoints {
		total = minFirstLevelSkillPoints
	}
	return total
}

// allowedSkillIDs is the union of class skills + background bonus
// skills, deduped and stable-sorted for menu rendering. These are
// the skills the player can spend ranks on at this step.
func (m *CharacterCreate) allowedSkillIDs() []string {
	cl, _ := m.catalog.Class(m.draft.ClassID)
	bg, _ := m.catalog.Background(m.draft.BackgroundID)
	if cl == nil || bg == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(cl.ClassSkills)+len(bg.BackgroundSkills))
	for _, id := range cl.ClassSkills {
		seen[id] = struct{}{}
	}
	for _, id := range bg.BackgroundSkills {
		seen[id] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// initFeatStepIfNeeded primes the feat substep on first entry. The
// pick stays empty so the menu prompts the player; bg.BonusFeats are
// applied at commit time (they're auto, not choices).
func (m *CharacterCreate) initFeatStepIfNeeded() {
	// Nothing to stamp — the menu reads bg.BonusFeats and lists the
	// background-feat options each render. Kept for symmetry with
	// initIdentityIfNeeded / initSkillsStepIfNeeded.
	_ = m
}

func (m *CharacterCreate) writeFeatMenu(s *telnet.Session) error {
	bg, _ := m.catalog.Background(m.draft.BackgroundID)
	if bg == nil {
		return writeError(s, "Internal catalog error; background missing.")
	}
	if err := writeStepHeader(s, chargenStepFeat); err != nil {
		return err
	}
	feats := m.catalog.FeatsForBackground(bg.ID)
	var b strings.Builder
	b.WriteString("{{First-level feat:}}::yellow|bold\r\n")
	if len(bg.BonusFeats) > 0 {
		fmt.Fprintf(&b,
			"  {{Bonus feats (auto):}}::yellow|bold %s\r\n",
			defangChargenField(strings.Join(
				featNames(m.catalog, bg.BonusFeats), ", ")))
	}
	if len(feats) == 0 {
		b.WriteString(
			"  {{(no background-restricted feats — type [D]one to continue)}}::gray\r\n")
	} else {
		b.WriteString("  {{Choices:}}::yellow|bold\r\n")
		for i, f := range feats {
			if f.ID == m.draft.SelectedFeatID {
				fmt.Fprintf(&b,
					"  {{*}}::green|bold {{%2d)}}::gray {{%-22s}}::yellow|bold %s\r\n",
					i+1, defangChargenField(f.Name),
					defangChargenField(featSummary(f)))
			} else {
				fmt.Fprintf(&b,
					"    {{%2d)}}::gray {{%-22s}}::yellow|bold %s\r\n",
					i+1, defangChargenField(f.Name),
					defangChargenField(featSummary(f)))
			}
		}
	}
	if m.draft.SelectedFeatID != "" {
		if f, ok := m.catalog.Feat(m.draft.SelectedFeatID); ok && f != nil {
			fmt.Fprintf(&b, "  {{Selected:}}::green|bold {{%s}}::green\r\n",
				defangChargenField(f.Name))
		}
	}
	b.WriteString("\r\n  Pick a number  ·  {{[I]}}::yellow nfo <#>  ·  {{[D]}}::green|bold one  ·  {{[B]}}::yellow ack to hub\r\n")
	return s.WriteString(b.String())
}

// featSummary renders a one-line hint for the picker row. Today the
// catalog feat type carries no compact summary field; fall back to
// the feat id (its yaml token) so power users still have a stable
// label they can pass to `info <id>`.
func featSummary(f *chargen.Feat) string {
	return fmt.Sprintf("(%s)", f.ID)
}

// featNames maps a slice of feat ids to display names via the catalog,
// falling back to the id when the entry is missing. Used to render
// bg.BonusFeats inline so the player sees what they're getting for
// free.
func featNames(cat *chargen.Catalog, ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if f, ok := cat.Feat(id); ok {
			out = append(out, f.Name)
			continue
		}
		out = append(out, id)
	}
	return out
}

func (m *CharacterCreate) applyFeat(s *telnet.Session, input string) error {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return m.writeFeatMenu(s)
	}
	bg, _ := m.catalog.Background(m.draft.BackgroundID)
	if bg == nil {
		return writeError(s, "Internal catalog error; background missing.")
	}
	feats := m.catalog.FeatsForBackground(bg.ID)

	// Info shorthand handled first so it doesn't shadow a numeric
	// pick. stripInfoVerb accepts "info <id|#>" and "i <id|#>".
	if rest, ok := stripInfoVerb(input); ok {
		if rest == "" {
			return writeError(s, "Type 'i <#>' or 'info <id>' for details.")
		}
		idx := pickFromList(rest, len(feats), func(i int) string { return feats[i].ID })
		if idx < 0 {
			return writeError(s, "Unknown feat. Type the id or list number.")
		}
		return m.writeFeatInfo(s, feats[idx])
	}

	verb := strings.ToLower(fields[0])
	rest := strings.Join(fields[1:], " ")

	switch verb {
	case "show":
		return m.writeFeatMenu(s)
	case "d", "done", "next":
		// Empty pick is OK if the background offers no feats — the
		// player still gets bg.BonusFeats.
		if m.draft.SelectedFeatID == "" && len(feats) > 0 {
			return writeError(s, "Pick a feat first, or 'pick <id|#>'.")
		}
		m.step = chargenStepHub
		return m.writeHub(s)
	case "pick":
		idx := pickFromList(rest, len(feats), func(i int) string { return feats[i].ID })
		if idx < 0 {
			return writeError(s, "Unknown feat. Type the id or list number.")
		}
		m.draft.SelectedFeatID = feats[idx].ID
		return m.writeFeatMenu(s)
	}

	// Bare "<id|#>" is also accepted as a pick — saves a verb.
	if idx := pickFromList(verb, len(feats), func(i int) string { return feats[i].ID }); idx >= 0 {
		m.draft.SelectedFeatID = feats[idx].ID
		return m.writeFeatMenu(s)
	}
	return writeError(s, "Pick a number, or 'i <#>' for info, or 'done'.")
}

func (m *CharacterCreate) writeFeatInfo(s *telnet.Session, f *chargen.Feat) error {
	if err := s.WriteString(fmt.Sprintf(
		"{{%s (%s)}}::cyan|bold\r\n",
		defangChargenField(f.Name), defangChargenField(f.ID),
	)); err != nil {
		return err
	}
	if err := writeFieldRow(s, "Type", featTypeLabel(f)); err != nil {
		return err
	}
	if f.Background && len(f.Backgrounds) > 0 {
		labels := make([]string, 0, len(f.Backgrounds))
		for _, id := range f.Backgrounds {
			labels = append(labels, backgroundDisplayName(m.catalog, id))
		}
		if err := writeFieldRow(s, "Available to",
			strings.Join(labels, ", ")); err != nil {
			return err
		}
	}
	if f.Description != "" {
		if err := s.WriteString("\r\n"); err != nil {
			return err
		}
		if err := s.WriteWrapped(strings.TrimRight(f.Description, "\n")); err != nil {
			return err
		}
		if err := s.WriteString("\r\n"); err != nil {
			return err
		}
	}
	return writeRule(s)
}

// featTypeLabel reports whether a feat is background-restricted or
// generally available, in player-friendly language.
func featTypeLabel(f *chargen.Feat) string {
	if f.Background {
		return "Background feat"
	}
	return "General feat"
}

// backgroundDisplayName resolves a background id to its display
// name via the catalog, falling back to the id when not found
// (defensive — a YAML edit could leave a stale reference).
func backgroundDisplayName(cat *chargen.Catalog, id string) string {
	if cat == nil {
		return id
	}
	if bg, ok := cat.Background(id); ok && bg != nil {
		return bg.Name
	}
	return id
}

// initSkillsStepIfNeeded stamps the budget and zero-rank map on first
// entry. Idempotent so back→forward preserves the player's allocations.
func (m *CharacterCreate) initSkillsStepIfNeeded() {
	if m.draft.SkillsInit {
		return
	}
	cl, _ := m.catalog.Class(m.draft.ClassID)
	classPoints := 0
	if cl != nil {
		classPoints = cl.SkillPoints
	}
	intMod := abilityModifier(int(m.draft.Abilities[3]))
	m.draft.SkillBudget = int8(firstLevelSkillBudget(classPoints, intMod))
	m.draft.SkillRanks = map[string]int8{}
	m.draft.SkillsInit = true
}

// skillsSpent sums the ranks the player has assigned in the draft.
func (m *CharacterCreate) skillsSpent() int {
	total := 0
	for _, r := range m.draft.SkillRanks {
		total += int(r)
	}
	return total
}

func (m *CharacterCreate) writeSkillsMenu(s *telnet.Session) error {
	if err := writeStepHeader(s, chargenStepSkills); err != nil {
		return err
	}
	skills := m.allowedSkillIDs()
	var b strings.Builder
	b.WriteString(
		"{{Skill ranks}}::yellow|bold (class + background skills, cap 4 each):\r\n")
	for i, id := range skills {
		sk, _ := m.catalog.Skill(id)
		name := id
		ability := ""
		if sk != nil {
			name = sk.Name
			ability = sk.Ability
		}
		ranks := m.draft.SkillRanks[id]
		// Highlight skills that have a non-zero rank so the eye can
		// scan allocations at a glance.
		if ranks > 0 {
			fmt.Fprintf(&b,
				"  {{%2d)}}::gray {{%-22s}}::yellow|bold {{%-3s}}::gray ranks={{%d}}::green|bold\r\n",
				i+1, defangChargenField(name), defangChargenField(ability), ranks)
		} else {
			fmt.Fprintf(&b,
				"  {{%2d)}}::gray {{%-22s}}::yellow {{%-3s}}::gray ranks=%d\r\n",
				i+1, defangChargenField(name), defangChargenField(ability), ranks)
		}
	}
	spent := m.skillsSpent()
	remaining := int(m.draft.SkillBudget) - spent
	remTag := "green|bold"
	switch {
	case remaining < 0:
		remTag = "red|bold"
	case remaining == 0:
		remTag = "yellow|bold"
	}
	fmt.Fprintf(&b,
		"  Budget {{%d}}::yellow|bold · Spent {{%d}}::yellow · Remaining {{%d}}::%s\r\n",
		m.draft.SkillBudget, spent, remaining, remTag)
	b.WriteString("\r\n  Pick a number then {{+}}::green|bold or {{-}}::red|bold to adjust  ·  {{[R]}}::yellow eset  ·  {{[D]}}::green|bold one\r\n")
	return s.WriteString(b.String())
}

// applySkills accepts:
//
//	<n>+ / <n>-          shorthand: bump skill <n>'s rank up or down by 1
//	<n> +  / <n> -       same, with an explicit space
//	rank <id|#> <n>      power-user form: set ranks directly to 0..4
//	r / reset            zero every skill
//	d / done             return to the hub
//	show                 redraw the menu
func (m *CharacterCreate) applySkills(s *telnet.Session, input string) error {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return m.writeSkillsMenu(s)
	}
	skills := m.allowedSkillIDs()
	verb := strings.ToLower(fields[0])

	switch verb {
	case "show":
		return m.writeSkillsMenu(s)
	case "r", "reset":
		m.draft.SkillRanks = map[string]int8{}
		return m.writeSkillsMenu(s)
	case "d", "done", "next":
		m.step = chargenStepHub
		return m.writeHub(s)
	case "rank":
		if len(fields) != 3 {
			return writeError(s, "Usage: rank <id|#> <n>")
		}
		return m.applySkillRank(s, skills, fields[1], fields[2])
	}

	// "<n> +" / "<n> -" with a space.
	if len(fields) == 2 && (fields[1] == "+" || fields[1] == "-") {
		return m.applySkillBump(s, skills, fields[0], fields[1] == "+")
	}
	// "<n>+" / "<n>-" without a space.
	if len(fields) == 1 {
		tok := fields[0]
		if n := len(tok); n >= 2 && (tok[n-1] == '+' || tok[n-1] == '-') {
			return m.applySkillBump(s, skills, tok[:n-1], tok[n-1] == '+')
		}
	}
	return writeError(s,
		"Pick a number then '+' or '-', or 'rank <id|#> <n>' / 'reset' / 'done'.")
}

// applySkillBump steps the rank for one skill up or down by 1 within
// [0..classSkillRankCapL1]. Over-budget bumps roll back, mirroring
// the rank verb's refusal behaviour.
func (m *CharacterCreate) applySkillBump(s *telnet.Session, skills []string, idTok string, up bool) error {
	idx := pickFromList(idTok, len(skills), func(i int) string { return skills[i] })
	if idx < 0 {
		return writeError(s, "Unknown skill. Type the id or list number.")
	}
	id := skills[idx]
	prev := m.draft.SkillRanks[id]
	next := prev
	if up {
		next++
	} else {
		next--
	}
	if next < 0 {
		return writeError(s, "Already at 0.")
	}
	if int(next) > classSkillRankCapL1 {
		return writeError(s, fmt.Sprintf("Cap is %d ranks per skill.", classSkillRankCapL1))
	}
	if next == 0 {
		delete(m.draft.SkillRanks, id)
	} else {
		m.draft.SkillRanks[id] = next
	}
	if m.skillsSpent() > int(m.draft.SkillBudget) {
		if prev == 0 {
			delete(m.draft.SkillRanks, id)
		} else {
			m.draft.SkillRanks[id] = prev
		}
		return writeError(s, fmt.Sprintf(
			"Not enough points (%d remaining).",
			int(m.draft.SkillBudget)-m.skillsSpent()))
	}
	return m.writeSkillsMenu(s)
}

func (m *CharacterCreate) applySkillRank(s *telnet.Session, skills []string, idTok, nTok string) error {
	idx := pickFromList(idTok, len(skills), func(i int) string { return skills[i] })
	if idx < 0 {
		return writeError(s, "Unknown skill. Type the id or list number.")
	}
	n, err := strconv.Atoi(nTok)
	if err != nil || n < 0 || n > classSkillRankCapL1 {
		return writeError(s, fmt.Sprintf(
			"Ranks must be an integer in [0..%d].", classSkillRankCapL1))
	}
	id := skills[idx]
	prev := m.draft.SkillRanks[id]
	m.draft.SkillRanks[id] = int8(n)
	if m.skillsSpent() > int(m.draft.SkillBudget) {
		// Roll back — over-budget assignments leave prior state intact
		// (mirrors the abilities point-buy refusal pattern).
		if prev == 0 {
			delete(m.draft.SkillRanks, id)
		} else {
			m.draft.SkillRanks[id] = prev
		}
		return writeError(s, fmt.Sprintf(
			"Not enough points (%d remaining).",
			int(m.draft.SkillBudget)-m.skillsSpent()))
	}
	if n == 0 {
		// Keep the map sparse so an empty rank doesn't render
		// alongside real allocations on the menu line.
		delete(m.draft.SkillRanks, id)
	}
	return m.writeSkillsMenu(s)
}

// buildFeatIDs merges the background's bonus feats (auto) with the
// player's selected 1st-level feat into the int32 list the Character
// schema persists. Duplicates are deduped (a player picking a feat
// they already have free is harmless).
func (m *CharacterCreate) buildFeatIDs() []int32 {
	bg, _ := m.catalog.Background(m.draft.BackgroundID)
	seen := map[string]struct{}{}
	if bg != nil {
		for _, id := range bg.BonusFeats {
			seen[id] = struct{}{}
		}
	}
	if m.draft.SelectedFeatID != "" {
		seen[m.draft.SelectedFeatID] = struct{}{}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]int32, len(ids))
	for i, id := range ids {
		out[i] = catalogIDInt32(id)
	}
	return out
}

// buildSkills converts the draft's id→ranks map into the int32-keyed
// SkillRanks map the Character schema persists. Every entry is
// flagged IsClassSkill=true — V1 only allows class + background
// skills, both of which are class-rate at level 1.
func (m *CharacterCreate) buildSkills() map[int32]creature.SkillRanks {
	if len(m.draft.SkillRanks) == 0 {
		return nil
	}
	out := make(map[int32]creature.SkillRanks, len(m.draft.SkillRanks))
	for id, r := range m.draft.SkillRanks {
		out[catalogIDInt32(id)] = creature.SkillRanks{
			Ranks:        r,
			IsClassSkill: true,
		}
	}
	return out
}
