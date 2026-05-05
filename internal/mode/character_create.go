package mode

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
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
	chargenStepFeat
	chargenStepSkills
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

	// First-level feat (#15). bg.BonusFeats are auto-merged at commit
	// time and don't live in the draft.
	SelectedFeatID string

	// First-level skill ranks (#15). Keys are catalog skill ids; values
	// are ranks in [0..classSkillRankCapL1]. SkillBudget caps the sum
	// of ranks. SkillsInit guards the on-entry initialization so that
	// re-entering via `back` from review preserves prior allocations.
	SkillRanks  map[string]int8
	SkillBudget int8
	SkillsInit  bool
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
	case chargenStepFeat:
		return "Feat (pick <id|#> | info <id|#> | done) [back/cancel]: "
	case chargenStepSkills:
		return "Skills (rank <id|#> <n> | reset | done) [back/cancel]: "
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
	if err := s.WriteString("\r\n{{Create a character.}}::cyan|bold\r\n"); err != nil {
		return err
	}
	if m.catalog != nil {
		if err := s.WriteString(
			"Type {{back}}::yellow to revisit the previous step or {{cancel}}::yellow to abort.\r\n",
		); err != nil {
			return err
		}
	}
	return nil
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
		return writeError(s, err.Error())
	}
	c, err := m.repo.Create(ctx, repo.Character{
		AccountID: s.AccountID,
		Name:      name,
		AuthLevel: repo.AuthLevelPlayer,
	})
	switch {
	case errors.Is(err, repo.ErrDuplicateCharacterName):
		return writeError(s, "Character name already taken. Choose another.")
	case err != nil:
		return writeError(s, "Character creation failed. Try again later.")
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
		return s.WriteString(
			"{{Cancelled. Restarting character creation.}}::yellow\r\n")
	case "back":
		if m.step == chargenStepName {
			return writeError(s, "Already at the first step.")
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
	case chargenStepFeat:
		return m.applyFeat(s, trimmed)
	case chargenStepSkills:
		return m.applySkills(s, trimmed)
	case chargenStepReview:
		return m.applyReview(ctx, s, trimmed)
	}
	return nil
}

func (m *CharacterCreate) applyName(s *telnet.Session, input string) error {
	if err := validateCharacterName(input); err != nil {
		return writeError(s, err.Error())
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
		return writeError(s, "Race must be 'human' or 'ogier'.")
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
		return writeError(s,
			"No backgrounds available for that race; pick another race.")
	}
	if err := writeStepHeader(s, chargenStepBackground); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("{{Backgrounds:}}::yellow|bold\r\n")
	for i, bg := range bgs {
		fmt.Fprintf(&b,
			"  {{%2d.}}::gray {{%-16s}}::yellow|bold %-22s %s\r\n",
			i+1,
			defangChargenField(bg.ID),
			defangChargenField(bg.Name),
			defangChargenField(backgroundSummary(bg)))
	}
	b.WriteString("Type {{info <id|#>}}::yellow for full details.\r\n")
	return s.WriteString(b.String())
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
			return writeError(s, "Unknown background. Type the id or list number.")
		}
		return m.writeBackgroundInfo(s, bgs[idx])
	}
	bg := pickFromList(input, len(bgs), func(i int) string { return bgs[i].ID })
	if bg < 0 {
		return writeError(s,
			"Unknown background. Type the id or list number, or 'info <id|#>'.")
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
	if err := s.WriteString(fmt.Sprintf(
		"{{%s (%s)}}::cyan|bold\r\n",
		defangChargenField(bg.Name), defangChargenField(bg.ID),
	)); err != nil {
		return err
	}
	if err := writeFieldRow(s, "Home language", bg.HomeLanguage); err != nil {
		return err
	}
	if len(bg.BonusLanguages) > 0 {
		if err := writeFieldRow(s, "Bonus languages",
			strings.Join(bg.BonusLanguages, ", ")); err != nil {
			return err
		}
	}
	if len(bg.BonusFeats) > 0 {
		if err := writeFieldRow(s, "Bonus feats",
			strings.Join(bg.BonusFeats, ", ")); err != nil {
			return err
		}
	}
	if len(bg.BackgroundSkills) > 0 {
		if err := writeFieldRow(s, "Background skills",
			strings.Join(bg.BackgroundSkills, ", ")); err != nil {
			return err
		}
	}
	if bg.RequiredSkill != "" {
		if err := writeFieldRow(s, "Required skill", bg.RequiredSkill); err != nil {
			return err
		}
	}
	if bg.SkillRestriction != "" {
		if err := writeFieldRow(s, "Skill restriction", bg.SkillRestriction); err != nil {
			return err
		}
	}
	if bg.WeaponRestriction != "" {
		if err := writeFieldRow(s, "Weapon restriction", bg.WeaponRestriction); err != nil {
			return err
		}
	}
	if bg.HeightModIn != 0 {
		if err := writeFieldRow(s, "Height mod",
			fmt.Sprintf("%+d in", bg.HeightModIn)); err != nil {
			return err
		}
	}
	if len(bg.EquipmentOptions) > 0 {
		if err := s.WriteString(
			"  {{Equipment options:}}::yellow|bold\r\n"); err != nil {
			return err
		}
		var b strings.Builder
		for i, eo := range bg.EquipmentOptions {
			fmt.Fprintf(&b, "    {{%d.}}::gray %s\r\n",
				i+1, defangChargenField(eo.Label))
		}
		if err := s.WriteString(b.String()); err != nil {
			return err
		}
	}
	if bg.Description != "" {
		if err := s.WriteString("\r\n"); err != nil {
			return err
		}
		// Description is builder-authored prose: pass through cfmt +
		// width-aware wrap. NOT defanged — builders may intentionally
		// colour the prose, mirroring look.go's LongDesc handling.
		if err := s.WriteWrapped(strings.TrimRight(bg.Description, "\n")); err != nil {
			return err
		}
		if err := s.WriteString("\r\n"); err != nil {
			return err
		}
	}
	return writeRule(s)
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
		return writeError(s,
			"No classes available for that race; revise your background.")
	}
	if err := writeStepHeader(s, chargenStepClass); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("{{Classes:}}::yellow|bold\r\n")
	for i, cl := range cls {
		fmt.Fprintf(&b,
			"  {{%2d.}}::gray {{%-16s}}::yellow|bold %-18s %s\r\n",
			i+1,
			defangChargenField(cl.ID),
			defangChargenField(cl.Name),
			defangChargenField(classSummary(cl)))
	}
	b.WriteString("Type {{info <id|#>}}::yellow for full details.\r\n")
	return s.WriteString(b.String())
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
			return writeError(s, "Unknown class. Type the id or list number.")
		}
		return m.writeClassInfo(s, cls[idx])
	}
	idx := pickFromList(input, len(cls), func(i int) string { return cls[i].ID })
	if idx < 0 {
		return writeError(s,
			"Unknown class. Type the id or list number, or 'info <id|#>'.")
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
	if err := s.WriteString(fmt.Sprintf(
		"{{%s (%s)}}::cyan|bold\r\n",
		defangChargenField(cl.Name), defangChargenField(cl.ID),
	)); err != nil {
		return err
	}
	if err := writeFieldRow(s, "Hit die", fmt.Sprintf("d%d", cl.HitDie)); err != nil {
		return err
	}
	if err := writeFieldRow(s, "BAB", string(cl.BAB)); err != nil {
		return err
	}
	if err := writeFieldRow(s, "Saves", fmt.Sprintf(
		"fort=%s ref=%s will=%s", cl.SaveFort, cl.SaveRef, cl.SaveWill,
	)); err != nil {
		return err
	}
	if err := writeFieldRow(s, "Skill points",
		fmt.Sprintf("%d/lvl (×4 at 1st)", cl.SkillPoints)); err != nil {
		return err
	}
	if len(cl.KeyAbilities) > 0 {
		if err := writeFieldRow(s, "Key abilities",
			strings.Join(cl.KeyAbilities, ", ")); err != nil {
			return err
		}
	}
	if cl.Channeler {
		// Channeler accent: cyan-bold draws the eye to the most
		// distinctive class trait without claiming saidin/saidar
		// blue/white that are reserved for in-world weave colouring
		// (see skills/mud/ui-expert/references/theme-and-cfmt-vocabulary.md).
		if err := s.WriteString(fmt.Sprintf(
			"  {{%-*s}}::yellow|bold {{yes}}::cyan|bold (source: %s) — channeler\r\n",
			chargenLabelGutter, "Channeler:",
			defangChargenField(cl.ChannelSource),
		)); err != nil {
			return err
		}
	}
	if len(cl.ClassSkills) > 0 {
		if err := writeFieldRow(s, "Class skills",
			strings.Join(cl.ClassSkills, ", ")); err != nil {
			return err
		}
	}
	if cl.Description != "" {
		if err := s.WriteString("\r\n"); err != nil {
			return err
		}
		if err := s.WriteWrapped(strings.TrimRight(cl.Description, "\n")); err != nil {
			return err
		}
		if err := s.WriteString("\r\n"); err != nil {
			return err
		}
	}
	return writeRule(s)
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
	if err := writeStepHeader(s, chargenStepAbilities); err != nil {
		return err
	}
	spent := m.pointBuySpent()
	remaining := pointBuyBudget - spent
	var b strings.Builder
	b.WriteString("{{Point-buy ability scores:}}::yellow|bold\r\n")
	for i, key := range abilityKeys {
		score := int(m.draft.Abilities[i])
		mod := abilityModifier(score)
		fmt.Fprintf(&b,
			"  {{%s}}::yellow|bold {{%2d}}::yellow (mod %+d, cost %d)\r\n",
			strings.ToUpper(key), score, mod, pointBuyCost(score))
	}
	// Remaining-points colour rule: green ≥1, yellow at 0, red <0.
	// The current code rejects on assignment, so the red branch only
	// renders on re-entry from `back` — keep it defensively.
	remTag := "green|bold"
	switch {
	case remaining < 0:
		remTag = "red|bold"
	case remaining == 0:
		remTag = "yellow|bold"
	}
	fmt.Fprintf(&b,
		"  Budget {{%d}}::yellow|bold · Spent {{%d}}::yellow · Remaining {{%d}}::%s\r\n",
		pointBuyBudget, spent, remaining, remTag)
	b.WriteString("  {{set}}::yellow <abil> <n>   change one score (8..18)\r\n")
	b.WriteString("  {{reset}}::yellow            send all scores back to 8\r\n")
	b.WriteString("  {{done}}::green|bold             accept and continue\r\n")
	return s.WriteString(b.String())
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
			return writeError(s, "You're over budget; reduce a score or 'reset'.")
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
		return writeError(s, "Usage: set <abil> <n>  |  reset  |  done")
	}

	idx := abilityIndex(abil)
	if idx < 0 {
		return writeError(s,
			"Ability must be one of str, dex, con, int, wis, cha.")
	}
	score, err := strconv.Atoi(scoreStr)
	if err != nil || score < pointBuyMinScore || score > pointBuyMaxScore {
		return writeError(s, fmt.Sprintf(
			"Score must be an integer in [%d..%d].",
			pointBuyMinScore, pointBuyMaxScore))
	}

	// Try the assignment; reject if it would push us over budget.
	prev := m.draft.Abilities[idx]
	m.draft.Abilities[idx] = int8(score)
	if m.pointBuySpent() > pointBuyBudget {
		m.draft.Abilities[idx] = prev
		return writeError(s, fmt.Sprintf(
			"Not enough points (would cost %d, %d remaining).",
			pointBuyCost(score)-pointBuyCost(int(prev)),
			pointBuyBudget-m.pointBuySpent()))
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
	if err := writeStepHeader(s, chargenStepReview); err != nil {
		return err
	}

	// Identity group ───────────────────────────────────────────────
	if err := s.WriteString("\r\n{{Identity}}::cyan|bold\r\n"); err != nil {
		return err
	}
	if err := writeFieldRow(s, "Name", m.draft.Name); err != nil {
		return err
	}
	if err := writeFieldRow(s, "Race", m.draft.Race); err != nil {
		return err
	}
	if m.draft.IdentitySet {
		if err := writeFieldRow(s, "Gender", genderLabel(m.draft.Gender)); err != nil {
			return err
		}
		if err := writeFieldRow(s, "Age", fmt.Sprintf("%d", m.draft.Age)); err != nil {
			return err
		}
		if err := writeFieldRow(s, "Height", renderHeight(m.draft.HeightCm)); err != nil {
			return err
		}
		if err := writeFieldRow(s, "Weight", renderWeight(m.draft.WeightKg)); err != nil {
			return err
		}
		if err := writeFieldRow(s, "Handed", handLabel(m.draft.Handedness)); err != nil {
			return err
		}
		if err := writeFieldRow(s, "Alignment", postureLabel(m.draft.Alignment)); err != nil {
			return err
		}
	}

	// Build group ──────────────────────────────────────────────────
	if err := s.WriteString("\r\n{{Build}}::cyan|bold\r\n"); err != nil {
		return err
	}
	if bg != nil {
		if err := writeFieldRow(s, "Background",
			fmt.Sprintf("%s (%s)", bg.Name, bg.ID)); err != nil {
			return err
		}
	}
	if cl != nil {
		if err := writeFieldRow(s, "Class",
			fmt.Sprintf("%s (%s)", cl.Name, cl.ID)); err != nil {
			return err
		}
	}
	var ab strings.Builder
	for i, key := range abilityKeys {
		if i > 0 {
			ab.WriteString("  ")
		}
		fmt.Fprintf(&ab, "%s %d", strings.ToUpper(key), m.draft.Abilities[i])
	}
	if err := writeFieldRow(s, "Abilities", ab.String()); err != nil {
		return err
	}

	// Loadout group ────────────────────────────────────────────────
	loadoutOpen := false
	openLoadout := func() error {
		if loadoutOpen {
			return nil
		}
		loadoutOpen = true
		return s.WriteString("\r\n{{Loadout}}::cyan|bold\r\n")
	}
	if bg != nil {
		feats := append([]string{}, bg.BonusFeats...)
		if m.draft.SelectedFeatID != "" {
			feats = append(feats, m.draft.SelectedFeatID)
		}
		if len(feats) > 0 {
			if err := openLoadout(); err != nil {
				return err
			}
			if err := writeFieldRow(s, "Feats",
				strings.Join(featNames(m.catalog, feats), ", ")); err != nil {
				return err
			}
		}
	}
	if len(m.draft.SkillRanks) > 0 {
		if err := openLoadout(); err != nil {
			return err
		}
		ids := make([]string, 0, len(m.draft.SkillRanks))
		for id := range m.draft.SkillRanks {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		parts := make([]string, 0, len(ids))
		for _, id := range ids {
			name := id
			if sk, ok := m.catalog.Skill(id); ok {
				name = sk.Name
			}
			parts = append(parts, fmt.Sprintf("%s %d", name, m.draft.SkillRanks[id]))
		}
		if err := writeFieldRow(s, "Skills", strings.Join(parts, ", ")); err != nil {
			return err
		}
	}

	if err := writeRule(s); err != nil {
		return err
	}
	return s.WriteString(
		"  Type {{yes}}::green|bold to confirm, " +
			"{{back}}::yellow to revise, " +
			"{{cancel}}::red to abort.\r\n")
}

func (m *CharacterCreate) applyReview(ctx context.Context, s *telnet.Session, input string) error {
	if !strings.EqualFold(input, "yes") && !strings.EqualFold(input, "y") {
		return writeError(s,
			"Type 'yes' to confirm, 'back' to revise, or 'cancel' to abort.")
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
		return writeError(s, "Internal catalog error; please start over.")
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
		Feats:       m.buildFeatIDs(),
		Skills:      m.buildSkills(),
	})
	switch {
	case errors.Is(err, repo.ErrDuplicateCharacterName):
		// Take the user back to the name step rather than dropping
		// every other selection — they only need to retype the name.
		m.step = chargenStepName
		m.draft.Name = ""
		return writeError(s, "Character name already taken. Choose another.")
	case err != nil:
		return writeError(s, "Character creation failed. Try again later.")
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
