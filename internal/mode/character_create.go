package mode

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

const (
	minCharacterNameLen = 3
	maxCharacterNameLen = 24
)

// reservedCharacterNames are forbidden case-insensitively because they
// collide with character-select keywords; an unrouteable character name
// is a permanent footgun (mirrors the "new" reservation in account
// create). The chargen control verbs (`back`, `cancel`) are also
// reserved so a character name can never shadow flow control.
var reservedCharacterNames = map[string]bool{
	"create": true,
	"quit":   true,
	"back":   true,
	"cancel": true,
}

// chargenStep enumerates the multi-step CharacterCreate substeps. The
// scaffold (#11) only wires name → race → background → class → review;
// abilities (#12), identity (#14), and feats/skills/equipment (#15)
// slot in as additional steps without restructuring this enum — they
// just bump the constants and the next-step table below.
type chargenStep int

const (
	chargenStepName chargenStep = iota
	chargenStepRace
	chargenStepBackground
	chargenStepClass
	chargenStepAbilities
	chargenStepIdentity
	chargenStepReview
	chargenStepDone
)

// Point-buy V1 (#12). Standard d20 cost table; budget is 25 points
// per docs/PLAN.md. Min/max bound the assignable starting score —
// racial / inherent bonuses (#14) are layered on later, not here.
const (
	pointBuyBudget   = 25
	pointBuyMinScore = 8
	pointBuyMaxScore = 18
)

// pointBuyCosts is indexed by score - pointBuyMinScore. The non-linear
// jumps (15→16: +2, 16→17: +3, 17→18: +3) make 17/18 expensive on
// purpose — a 25-point budget cannot afford an 18 and three 14s.
var pointBuyCosts = [...]int{
	0,  // 8
	1,  // 9
	2,  // 10
	3,  // 11
	4,  // 12
	5,  // 13
	6,  // 14
	8,  // 15
	10, // 16
	13, // 17
	16, // 18
}

// abilityKeys is the canonical ordering for menus and indexing into
// chargenDraft.Abilities. Matches the book's Str/Dex/Con/Int/Wis/Cha
// presentation order.
var abilityKeys = [...]string{"str", "dex", "con", "int", "wis", "cha"}

// chargenDraft is the in-progress character being built across the
// multi-step CharacterCreate flow. Each substep mutates one field;
// the review step submits it to CharacterRepo.Create.
//
// Future steps (#12 abilities, #14 identity, #15 feats/skills/equipment)
// add fields here without changing the surrounding plumbing.
type chargenDraft struct {
	Name         string
	Race         string // "human" | "ogier"
	BackgroundID string
	ClassID      string

	// Abilities are indexed by abilityKeys (Str=0..Cha=5). Zero means
	// "step not visited yet"; the abilities substep initializes every
	// slot to pointBuyMinScore on first entry so a partially-filled
	// draft never serializes a 0.
	Abilities    [6]int8
	AbilitiesSet bool

	// Identity (#14). Defaults are stamped on first entry into the
	// identity substep — height/weight roll Table 6-1 against race +
	// background HeightModIn, the rest pick conservative defaults
	// the player can override.
	Gender      creature.Gender
	Age         int16
	Handedness  creature.Hand
	Alignment   creature.Posture
	HeightCm    int16
	WeightKg    int16
	IdentitySet bool
}

// CharacterCreate prompts for a new character. With a chargen catalog
// wired (SetCatalog) it walks a substep stack: name → race →
// background → class → review/confirm. Without a catalog it falls
// back to the legacy single-name flow so dev / test fixtures stay
// simple.
//
// The substep state machine is internal (a `step` enum) rather than
// one Mode per substep: the user-visible behavior — `back` to step
// down, `cancel` to abort — is the same either way, and the enum
// keeps draft state contiguous so #12-#15 can extend it without
// restructuring the mode stack.
type CharacterCreate struct {
	repo    repo.CharacterRepo
	catalog *chargen.Catalog
	game    telnet.Mode
	motd    MOTDFunc
	shown   bool

	step  chargenStep
	draft chargenDraft

	// rng drives the height/weight rolls in the identity substep.
	// Tests inject a deterministic source via SetRNG; production paths
	// fall through to a time-seeded default on first use.
	rng *rand.Rand
}

func NewCharacterCreate(characters repo.CharacterRepo, game telnet.Mode) *CharacterCreate {
	return &CharacterCreate{repo: characters, game: game}
}

// SetRNG injects a deterministic random source for the identity step's
// height/weight rolls. Tests use this to assert exact values; nil
// keeps the default time-seeded source.
func (m *CharacterCreate) SetRNG(r *rand.Rand) { m.rng = r }

func (m *CharacterCreate) randSource() *rand.Rand {
	if m.rng == nil {
		m.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return m.rng
}

// SetMOTD wires the MOTD hook fired by promoteToGame after the
// character is created. nil disables it.
func (m *CharacterCreate) SetMOTD(f MOTDFunc) { m.motd = f }

// SetCatalog enables the multi-step chargen flow. Passing nil keeps
// the legacy single-name flow.
func (m *CharacterCreate) SetCatalog(c *chargen.Catalog) { m.catalog = c }

func (m *CharacterCreate) Prompt(_ context.Context, _ *telnet.Session) string {
	if m.catalog == nil {
		return "Choose a character name: "
	}
	switch m.step {
	case chargenStepName:
		return "Choose a character name: "
	case chargenStepRace:
		return "Race (human / ogier) [back/cancel]: "
	case chargenStepBackground:
		return "Background (id, # or 'info <id|#>') [back/cancel]: "
	case chargenStepClass:
		return "Class (id, # or 'info <id|#>') [back/cancel]: "
	case chargenStepAbilities:
		return "Abilities (set <abil> <n> | reset | done) [back/cancel]: "
	case chargenStepIdentity:
		return "Identity (gender/age/handed/align/roll | done) [back/cancel]: "
	case chargenStepReview:
		return "Confirm? (yes / back / cancel): "
	}
	return "> "
}

func (m *CharacterCreate) OnEnter(s *telnet.Session) error {
	if m.shown {
		return nil
	}
	m.shown = true
	header := "\r\nCreate a character.\r\n"
	if m.catalog != nil {
		header += "Type 'back' to revisit the previous step or 'cancel' to abort.\r\n"
	}
	return s.WriteRaw([]byte(header))
}

func (m *CharacterCreate) OnExit(_ *telnet.Session) error { return nil }

func (m *CharacterCreate) Handle(ctx context.Context, s *telnet.Session, line string) error {
	if m.catalog == nil {
		return m.handleLegacy(ctx, s, line)
	}
	return m.handleMulti(ctx, s, line)
}

// handleLegacy preserves the original single-prompt flow for callers
// that don't wire a chargen catalog (every existing test, plus dev
// fixtures with no data/chargen on disk).
func (m *CharacterCreate) handleLegacy(ctx context.Context, s *telnet.Session, line string) error {
	name := strings.TrimSpace(line)
	if err := validateCharacterName(name); err != nil {
		return s.WriteRaw([]byte(err.Error() + "\r\n"))
	}
	c, err := m.repo.Create(ctx, repo.Character{
		AccountID: s.AccountID,
		Name:      name,
		AuthLevel: repo.AuthLevelPlayer,
	})
	switch {
	case errors.Is(err, repo.ErrDuplicateCharacterName):
		return s.WriteRaw([]byte("Character name already taken. Choose another.\r\n"))
	case err != nil:
		return s.WriteRaw([]byte("Character creation failed. Try again later.\r\n"))
	}
	return promoteToGame(ctx, s, c, m.repo, m.motd, m.game)
}

// handleMulti drives the substep state machine. `back` pops to the
// previous step (no-op at step zero); `cancel` resets the flow to
// the name step with an empty draft.
func (m *CharacterCreate) handleMulti(ctx context.Context, s *telnet.Session, line string) error {
	trimmed := strings.TrimSpace(line)
	switch strings.ToLower(trimmed) {
	case "":
		return nil
	case "cancel":
		m.draft = chargenDraft{}
		m.step = chargenStepName
		return s.WriteRaw([]byte("Cancelled. Restarting character creation.\r\n"))
	case "back":
		if m.step == chargenStepName {
			return s.WriteRaw([]byte("Already at the first step.\r\n"))
		}
		m.step--
		return nil
	}

	switch m.step {
	case chargenStepName:
		return m.applyName(s, trimmed)
	case chargenStepRace:
		return m.applyRace(s, trimmed)
	case chargenStepBackground:
		return m.applyBackground(s, trimmed)
	case chargenStepClass:
		return m.applyClass(s, trimmed)
	case chargenStepAbilities:
		return m.applyAbilities(s, trimmed)
	case chargenStepIdentity:
		return m.applyIdentity(s, trimmed)
	case chargenStepReview:
		return m.applyReview(ctx, s, trimmed)
	}
	return nil
}

func (m *CharacterCreate) applyName(s *telnet.Session, input string) error {
	if err := validateCharacterName(input); err != nil {
		return s.WriteRaw([]byte(err.Error() + "\r\n"))
	}
	m.draft.Name = input
	m.step = chargenStepRace
	return nil
}

func (m *CharacterCreate) applyRace(s *telnet.Session, input string) error {
	race := strings.ToLower(input)
	switch race {
	case "human", "ogier":
	default:
		return s.WriteRaw([]byte("Race must be 'human' or 'ogier'.\r\n"))
	}
	m.draft.Race = race
	m.step = chargenStepBackground
	return m.writeBackgroundMenu(s)
}

func (m *CharacterCreate) writeBackgroundMenu(s *telnet.Session) error {
	bgs := m.catalog.BackgroundsForRace(m.draft.Race)
	if len(bgs) == 0 {
		// Catalog mis-seeded for this race; back the user up so they
		// don't end up stuck.
		m.step = chargenStepRace
		return s.WriteRaw([]byte("No backgrounds available for that race; pick another race.\r\n"))
	}
	var b strings.Builder
	b.WriteString("Backgrounds:\r\n")
	for i, bg := range bgs {
		fmt.Fprintf(&b, "  %2d. %-16s %-22s %s\r\n",
			i+1, bg.ID, bg.Name, backgroundSummary(bg))
	}
	b.WriteString("Type 'info <id|#>' for full details.\r\n")
	return s.WriteRaw([]byte(b.String()))
}

// backgroundSummary renders the one-line menu hint for a background:
// home language plus the count of bonus feats / skills / equipment
// options. Keeps the menu narrow enough for an 80-col terminal.
func backgroundSummary(bg *chargen.Background) string {
	return fmt.Sprintf("%s (%d feats, %d skills, %d outfits)",
		bg.HomeLanguage, len(bg.BonusFeats), len(bg.BackgroundSkills),
		len(bg.EquipmentOptions))
}

func (m *CharacterCreate) applyBackground(s *telnet.Session, input string) error {
	bgs := m.catalog.BackgroundsForRace(m.draft.Race)
	if rest, ok := stripInfoVerb(input); ok {
		idx := pickFromList(rest, len(bgs), func(i int) string { return bgs[i].ID })
		if idx < 0 {
			return s.WriteRaw([]byte("Unknown background. Type the id or list number.\r\n"))
		}
		return m.writeBackgroundInfo(s, bgs[idx])
	}
	bg := pickFromList(input, len(bgs), func(i int) string { return bgs[i].ID })
	if bg < 0 {
		return s.WriteRaw([]byte("Unknown background. Type the id or list number, or 'info <id|#>'.\r\n"))
	}
	m.draft.BackgroundID = bgs[bg].ID
	m.step = chargenStepClass
	return m.writeClassMenu(s)
}

// writeBackgroundInfo renders the full descriptor block: languages,
// bonus feats, background skills, height mod, restrictions, and the
// equipment-option bundles. The player can then pick by id/number, or
// 'info' another option, or 'back' / 'cancel'.
func (m *CharacterCreate) writeBackgroundInfo(s *telnet.Session, bg *chargen.Background) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)\r\n", bg.Name, bg.ID)
	fmt.Fprintf(&b, "  Home language:    %s\r\n", bg.HomeLanguage)
	if len(bg.BonusLanguages) > 0 {
		fmt.Fprintf(&b, "  Bonus languages:  %s\r\n", strings.Join(bg.BonusLanguages, ", "))
	}
	if len(bg.BonusFeats) > 0 {
		fmt.Fprintf(&b, "  Bonus feats:      %s\r\n", strings.Join(bg.BonusFeats, ", "))
	}
	if len(bg.BackgroundSkills) > 0 {
		fmt.Fprintf(&b, "  Background skills:%s\r\n", " "+strings.Join(bg.BackgroundSkills, ", "))
	}
	if bg.RequiredSkill != "" {
		fmt.Fprintf(&b, "  Required skill:   %s\r\n", bg.RequiredSkill)
	}
	if bg.SkillRestriction != "" {
		fmt.Fprintf(&b, "  Skill restriction:%s\r\n", " "+bg.SkillRestriction)
	}
	if bg.WeaponRestriction != "" {
		fmt.Fprintf(&b, "  Weapon restriction:%s\r\n", " "+bg.WeaponRestriction)
	}
	if bg.HeightModIn != 0 {
		fmt.Fprintf(&b, "  Height mod:       %+d in\r\n", bg.HeightModIn)
	}
	if len(bg.EquipmentOptions) > 0 {
		b.WriteString("  Equipment options:\r\n")
		for i, eo := range bg.EquipmentOptions {
			fmt.Fprintf(&b, "    %d. %s\r\n", i+1, eo.Label)
		}
	}
	if bg.Description != "" {
		b.WriteString("\r\n")
		b.WriteString(strings.TrimRight(bg.Description, "\n"))
		b.WriteString("\r\n")
	}
	return s.WriteRaw([]byte(b.String()))
}

// stripInfoVerb returns the trailing argument when input begins with
// "info " (case-insensitive), and reports whether the prefix matched.
// "info" alone with no argument returns ("", true) so callers can
// surface a usage hint.
func stripInfoVerb(input string) (string, bool) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "", false
	}
	if !strings.EqualFold(fields[0], "info") && !strings.EqualFold(fields[0], "details") {
		return "", false
	}
	if len(fields) < 2 {
		return "", true
	}
	return strings.Join(fields[1:], " "), true
}

func (m *CharacterCreate) writeClassMenu(s *telnet.Session) error {
	cls := m.catalog.ClassesForRace(m.draft.Race)
	if len(cls) == 0 {
		m.step = chargenStepBackground
		return s.WriteRaw([]byte("No classes available for that race; revise your background.\r\n"))
	}
	var b strings.Builder
	b.WriteString("Classes:\r\n")
	for i, cl := range cls {
		fmt.Fprintf(&b, "  %2d. %-16s %-18s %s\r\n",
			i+1, cl.ID, cl.Name, classSummary(cl))
	}
	b.WriteString("Type 'info <id|#>' for full details.\r\n")
	return s.WriteRaw([]byte(b.String()))
}

// classSummary renders the one-line menu hint: hit die, BAB
// progression, and (when relevant) the channeler tag.
func classSummary(cl *chargen.Class) string {
	tag := ""
	if cl.Channeler {
		tag = " channeler"
	}
	return fmt.Sprintf("d%d HD, %s BAB%s", cl.HitDie, cl.BAB, tag)
}

func (m *CharacterCreate) applyClass(s *telnet.Session, input string) error {
	cls := m.catalog.ClassesForRace(m.draft.Race)
	if rest, ok := stripInfoVerb(input); ok {
		idx := pickFromList(rest, len(cls), func(i int) string { return cls[i].ID })
		if idx < 0 {
			return s.WriteRaw([]byte("Unknown class. Type the id or list number.\r\n"))
		}
		return m.writeClassInfo(s, cls[idx])
	}
	idx := pickFromList(input, len(cls), func(i int) string { return cls[i].ID })
	if idx < 0 {
		return s.WriteRaw([]byte("Unknown class. Type the id or list number, or 'info <id|#>'.\r\n"))
	}
	m.draft.ClassID = cls[idx].ID
	m.step = chargenStepAbilities
	m.initAbilitiesIfNeeded()
	return m.writeAbilitiesMenu(s)
}

// writeClassInfo renders the full descriptor block: HD / BAB / saves,
// per-level skill points, key abilities, channeler source, class
// skills, and the narrative blurb.
func (m *CharacterCreate) writeClassInfo(s *telnet.Session, cl *chargen.Class) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)\r\n", cl.Name, cl.ID)
	fmt.Fprintf(&b, "  Hit die:        d%d\r\n", cl.HitDie)
	fmt.Fprintf(&b, "  BAB:            %s\r\n", cl.BAB)
	fmt.Fprintf(&b, "  Saves:          fort=%s ref=%s will=%s\r\n",
		cl.SaveFort, cl.SaveRef, cl.SaveWill)
	fmt.Fprintf(&b, "  Skill points:   %d/lvl (×4 at 1st)\r\n", cl.SkillPoints)
	if len(cl.KeyAbilities) > 0 {
		fmt.Fprintf(&b, "  Key abilities:  %s\r\n", strings.Join(cl.KeyAbilities, ", "))
	}
	if cl.Channeler {
		fmt.Fprintf(&b, "  Channeler:      yes (source: %s)\r\n", cl.ChannelSource)
	}
	if len(cl.ClassSkills) > 0 {
		fmt.Fprintf(&b, "  Class skills:   %s\r\n", strings.Join(cl.ClassSkills, ", "))
	}
	if cl.Description != "" {
		b.WriteString("\r\n")
		b.WriteString(strings.TrimRight(cl.Description, "\n"))
		b.WriteString("\r\n")
	}
	return s.WriteRaw([]byte(b.String()))
}

// initAbilitiesIfNeeded primes every slot to the point-buy floor on
// first entry into the abilities step. Idempotent: re-entering via
// `back` from review preserves the draft's prior assignments.
func (m *CharacterCreate) initAbilitiesIfNeeded() {
	if m.draft.AbilitiesSet {
		return
	}
	for i := range m.draft.Abilities {
		m.draft.Abilities[i] = pointBuyMinScore
	}
	m.draft.AbilitiesSet = true
}

// pointBuyCost returns the cumulative cost of `score` measured from
// the floor (8 = 0 points). Out-of-range scores return -1.
func pointBuyCost(score int) int {
	if score < pointBuyMinScore || score > pointBuyMaxScore {
		return -1
	}
	return pointBuyCosts[score-pointBuyMinScore]
}

// pointBuySpent sums the cost of the current draft assignments.
func (m *CharacterCreate) pointBuySpent() int {
	total := 0
	for _, sc := range m.draft.Abilities {
		total += pointBuyCost(int(sc))
	}
	return total
}

func (m *CharacterCreate) writeAbilitiesMenu(s *telnet.Session) error {
	spent := m.pointBuySpent()
	var b strings.Builder
	b.WriteString("Point-buy ability scores:\r\n")
	for i, key := range abilityKeys {
		score := int(m.draft.Abilities[i])
		mod := abilityModifier(score)
		fmt.Fprintf(&b, "  %s %2d (mod %+d, cost %d)\r\n",
			strings.ToUpper(key), score, mod, pointBuyCost(score))
	}
	fmt.Fprintf(&b, "  budget %d / spent %d / remaining %d\r\n",
		pointBuyBudget, spent, pointBuyBudget-spent)
	b.WriteString("  set <abil> <n>   change one score (8..18)\r\n")
	b.WriteString("  reset            send all scores back to 8\r\n")
	b.WriteString("  done             accept and continue\r\n")
	return s.WriteRaw([]byte(b.String()))
}

// abilityModifier mirrors creature.AbilityModifier (floor((s-10)/2))
// without taking a runtime dep on creature internals — keeps the
// chargen step pure-arithmetic.
func abilityModifier(score int) int {
	// Go integer division truncates toward zero, but for scores in the
	// point-buy range (8..18) the result matches floor.
	return (score - 10) / 2
}

// applyAbilities parses one of:
//
//	show / blank line   → re-render the menu
//	set <abil> <n>      → assign one score (also accepts "<abil> <n>")
//	reset               → all back to 8
//	done                → advance to review
func (m *CharacterCreate) applyAbilities(s *telnet.Session, input string) error {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return m.writeAbilitiesMenu(s)
	}
	verb := strings.ToLower(fields[0])

	switch verb {
	case "show":
		return m.writeAbilitiesMenu(s)
	case "reset":
		for i := range m.draft.Abilities {
			m.draft.Abilities[i] = pointBuyMinScore
		}
		return m.writeAbilitiesMenu(s)
	case "done", "next":
		if m.pointBuySpent() > pointBuyBudget {
			return s.WriteRaw([]byte("You're over budget; reduce a score or 'reset'.\r\n"))
		}
		m.step = chargenStepIdentity
		m.initIdentityIfNeeded()
		return m.writeIdentityMenu(s)
	}

	// Allow either "set <abil> <n>" or "<abil> <n>".
	abil, scoreStr := "", ""
	switch {
	case verb == "set" && len(fields) == 3:
		abil, scoreStr = strings.ToLower(fields[1]), fields[2]
	case len(fields) == 2:
		abil, scoreStr = verb, fields[1]
	default:
		return s.WriteRaw([]byte("Usage: set <abil> <n>  |  reset  |  done\r\n"))
	}

	idx := abilityIndex(abil)
	if idx < 0 {
		return s.WriteRaw([]byte("Ability must be one of str, dex, con, int, wis, cha.\r\n"))
	}
	score, err := strconv.Atoi(scoreStr)
	if err != nil || score < pointBuyMinScore || score > pointBuyMaxScore {
		return s.WriteRaw([]byte(fmt.Sprintf(
			"Score must be an integer in [%d..%d].\r\n",
			pointBuyMinScore, pointBuyMaxScore)))
	}

	// Try the assignment; reject if it would push us over budget.
	prev := m.draft.Abilities[idx]
	m.draft.Abilities[idx] = int8(score)
	if m.pointBuySpent() > pointBuyBudget {
		m.draft.Abilities[idx] = prev
		return s.WriteRaw([]byte(fmt.Sprintf(
			"Not enough points (would cost %d, %d remaining).\r\n",
			pointBuyCost(score)-pointBuyCost(int(prev)),
			pointBuyBudget-m.pointBuySpent())))
	}
	return m.writeAbilitiesMenu(s)
}

// buildAbilities renders the draft's [6]int8 into the
// creature.Abilities triple. Current = Max = the assigned score;
// Inherent stays 0 (ter'angreal/racial floors aren't applied here).
func (m *CharacterCreate) buildAbilities() creature.Abilities {
	score := func(i int) creature.AbilityScore {
		v := m.draft.Abilities[i]
		return creature.AbilityScore{Current: v, Max: v}
	}
	return creature.Abilities{
		Str: score(0),
		Dex: score(1),
		Con: score(2),
		Int: score(3),
		Wis: score(4),
		Cha: score(5),
	}
}

// abilityIndex maps a 3-letter ability token (case-insensitive) to
// its abilityKeys index, or -1 on miss.
func abilityIndex(token string) int {
	t := strings.ToLower(token)
	for i, k := range abilityKeys {
		if k == t {
			return i
		}
	}
	return -1
}

func (m *CharacterCreate) writeReview(s *telnet.Session) error {
	bg, _ := m.catalog.Background(m.draft.BackgroundID)
	cl, _ := m.catalog.Class(m.draft.ClassID)
	var b strings.Builder
	b.WriteString("Review:\r\n")
	fmt.Fprintf(&b, "  Name:       %s\r\n", m.draft.Name)
	fmt.Fprintf(&b, "  Race:       %s\r\n", m.draft.Race)
	if bg != nil {
		fmt.Fprintf(&b, "  Background: %s (%s)\r\n", bg.Name, bg.ID)
	}
	if cl != nil {
		fmt.Fprintf(&b, "  Class:      %s (%s)\r\n", cl.Name, cl.ID)
	}
	b.WriteString("  Abilities:  ")
	for i, key := range abilityKeys {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%s %d", strings.ToUpper(key), m.draft.Abilities[i])
	}
	b.WriteString("\r\n")
	if m.draft.IdentitySet {
		fmt.Fprintf(&b, "  Gender:     %s\r\n", genderLabel(m.draft.Gender))
		fmt.Fprintf(&b, "  Age:        %d\r\n", m.draft.Age)
		fmt.Fprintf(&b, "  Height:     %s\r\n", renderHeight(m.draft.HeightCm))
		fmt.Fprintf(&b, "  Weight:     %s\r\n", renderWeight(m.draft.WeightKg))
		fmt.Fprintf(&b, "  Handed:     %s\r\n", handLabel(m.draft.Handedness))
		fmt.Fprintf(&b, "  Alignment:  %s\r\n", postureLabel(m.draft.Alignment))
	}
	return s.WriteRaw([]byte(b.String()))
}

func (m *CharacterCreate) applyReview(ctx context.Context, s *telnet.Session, input string) error {
	if !strings.EqualFold(input, "yes") && !strings.EqualFold(input, "y") {
		return s.WriteRaw([]byte("Type 'yes' to confirm, 'back' to revise, or 'cancel' to abort.\r\n"))
	}

	// Identity must be stamped before commit — otherwise GenderNone /
	// zero height/weight would persist. The flow forces the player
	// through chargenStepIdentity, but a future refactor that adds a
	// shortcut into review would silently corrupt the row without
	// this guard. Bump back to identity rather than committing.
	if !m.draft.IdentitySet {
		m.step = chargenStepIdentity
		m.initIdentityIfNeeded()
		return m.writeIdentityMenu(s)
	}

	bg, _ := m.catalog.Background(m.draft.BackgroundID)
	cl, _ := m.catalog.Class(m.draft.ClassID)
	if bg == nil || cl == nil {
		// Catalog drift — should be impossible since we just looked
		// these up. Defensive reset rather than panic.
		m.draft = chargenDraft{}
		m.step = chargenStepName
		return s.WriteRaw([]byte("Internal catalog error; please start over.\r\n"))
	}

	race := creature.RaceHuman
	if m.draft.Race == "ogier" {
		race = creature.RaceOgier
	}

	core := creature.Core{}
	core.Abilities = m.buildAbilities()
	core.Gender = m.draft.Gender
	core.Alignment = m.draft.Alignment

	c, err := m.repo.Create(ctx, repo.Character{
		AccountID:   s.AccountID,
		Name:        m.draft.Name,
		AuthLevel:   repo.AuthLevelPlayer,
		Race:        race,
		Background:  bg.Enum,
		ClassLevels: map[creature.Class]int8{cl.Enum: 1},
		Core:        core,
		HeightCm:    m.draft.HeightCm,
		WeightKg:    m.draft.WeightKg,
		Age:         m.draft.Age,
		Handedness:  m.draft.Handedness,
	})
	switch {
	case errors.Is(err, repo.ErrDuplicateCharacterName):
		// Take the user back to the name step rather than dropping
		// every other selection — they only need to retype the name.
		m.step = chargenStepName
		m.draft.Name = ""
		return s.WriteRaw([]byte("Character name already taken. Choose another.\r\n"))
	case err != nil:
		return s.WriteRaw([]byte("Character creation failed. Try again later.\r\n"))
	}

	m.step = chargenStepDone
	return promoteToGame(ctx, s, c, m.repo, m.motd, m.game)
}

// pickFromList resolves an input that's either a 1-based list number
// or an id (case-insensitive) against `n` items via id(i). Returns
// the matching index or -1 on miss.
func pickFromList(input string, n int, id func(int) string) int {
	if n == 0 {
		return -1
	}
	if num, err := strconv.Atoi(input); err == nil {
		if num >= 1 && num <= n {
			return num - 1
		}
		return -1
	}
	for i := 0; i < n; i++ {
		if strings.EqualFold(input, id(i)) {
			return i
		}
	}
	return -1
}

// validateCharacterName mirrors validateUsername — same charset and
// length rules, distinct reserved set. Kept separate from
// validateUsername so the two policies can drift independently as
// account vs. character UX evolves.
func validateCharacterName(name string) error {
	if name == "" {
		return errors.New("Character name cannot be empty")
	}
	n := len(name)
	if n < minCharacterNameLen {
		return errors.New("Character name too short (minimum 3 characters)")
	}
	if n > maxCharacterNameLen {
		return errors.New("Character name too long (maximum 24 characters)")
	}
	for _, r := range name {
		if r > unicode.MaxASCII {
			return errors.New("Character name may only contain ASCII letters, digits, _ or -")
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return errors.New("Character name may only contain letters, digits, _ or -")
		}
	}
	if reservedCharacterNames[strings.ToLower(name)] {
		return errors.New("Character name is reserved. Choose another")
	}
	return nil
}
