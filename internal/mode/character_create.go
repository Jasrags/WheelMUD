package mode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/display"
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
// post-name/race flow is hub-driven: every substep returns to
// chargenStepHub when "done", and the hub auto-opens review once the
// draft is complete. The legacy linear ordering is preserved as the
// numeric order of the enum so existing tests and the chargen step
// banner continue to read 1..8.
type chargenStep int

const (
	chargenStepName chargenStep = iota
	chargenStepRace
	chargenStepHub
	chargenStepBackground
	chargenStepClass
	chargenStepAbilities
	chargenStepIdentity
	chargenStepFeat
	chargenStepSkills
	chargenStepChanneling
	chargenStepEquipment
	chargenStepReview
	chargenStepDone

	// Confirm sub-states pivot the hub into a [Y/N] prompt. The
	// pendingConfirm field on CharacterCreate records *which* confirm
	// is active so a single applyHubConfirm handler can answer both.
	chargenStepHubConfirm
)

// confirmKind tracks which destructive hub action is awaiting [Y/N].
type confirmKind int

const (
	confirmNone confirmKind = iota
	confirmRestart
	confirmCancel
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

	// Channeler branch (#15 slice 2). Only populated when
	// catalog.Class(ClassID).Channeler is true. Source defaults from
	// Gender on first entry; affinities + starting weaves are
	// player-picked. ChannelingInit guards the gender→source default.
	ChannelSource  creature.Source
	Affinities     creature.PowerSet
	StartingWeaves []string
	ChannelingInit bool

	// Starting-equipment bundle (#15 slice 3). Index into
	// bg.EquipmentOptions of the picked bundle, 1-based; 0 means the
	// player hasn't picked yet. The substep enforces "pick before
	// done" so the finaliseReview path always sees a positive index
	// (defensive: build path also rejects an out-of-range index).
	SelectedEquipmentOptionIdx int
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
	items   repo.ItemRepo
	catalog *chargen.Catalog
	game    telnet.Mode
	shown   bool

	step  chargenStep
	draft chargenDraft

	// rng drives the height/weight rolls in the identity substep.
	// Tests inject a deterministic source via SetRNG; production paths
	// fall through to a time-seeded default on first use.
	rng *rand.Rand

	// settings carries the §6 account-level AccountSettings the player
	// configured in the account menu before entering chargen.
	// PromptDefault stamps onto Character.PromptTemplate at finalize
	// time; ColorOverride and WidthOverride apply to the session via
	// applyAccountSettings just before promoteToGame so the very first
	// game-mode render uses the player's preferred values. The zero
	// value (an account that never visited the settings menu) is a
	// no-op — the per-character column stays empty and the session
	// keeps its TERM/NAWS-detected values.
	settings repo.AccountSettings

	// pendingConfirm is non-zero while the hub is awaiting a [Y/N]
	// answer for a destructive action ([R]estart wipes the draft;
	// [Q]uit exits chargen via onCancel or, when nil, restarts in
	// place). Cleared on either answer.
	pendingConfirm confirmKind

	// suppressAutoReview disables the hub's "all rows filled →
	// auto-open review" jump for exactly one render. Set when the
	// player picks [B]ack from the review screen so they can land on
	// the hub view to make further edits.
	suppressAutoReview bool

	// onCancel runs when the hub's [Q]uit confirm is answered Y. It
	// returns the player to whichever mode launched chargen (the
	// account menu, typically). nil — set when chargen is the
	// post-auth landing for a fresh account with no characters — falls
	// back to the legacy in-place "wipe draft, restart" behaviour.
	onCancel func(s *telnet.Session) error

	// pendingIdentityField is non-zero while the identity substep is
	// awaiting a value for a specific field (the player picked a
	// numbered row from the identity sub-hub). Set when the player
	// picks 1..5 from the identity menu, cleared as soon as the next
	// line is parsed (or rejected) for that field.
	pendingIdentityField identityField

	// channelingStage tracks which sub-screen the channeling substep
	// is currently rendering. Slice 5 splits the channeler picker
	// into two checklist stages — affinities first, then weaves —
	// each driven by numbered toggles. enterSubstep resets to
	// channelingStageAffinities; "done" advances the stage forward
	// and "prev" rolls it back.
	channelingStage channelingStage
}

// channelingStage is the per-screen sub-state for the slice-5
// channeling picker. Only meaningful while step == chargenStepChanneling.
type channelingStage int

const (
	channelingStageAffinities channelingStage = iota
	channelingStageWeaves
)

// identityField is the per-row sub-state for the slice-4 identity
// numbered sub-hub. Only meaningful while step == chargenStepIdentity.
type identityField int

const (
	identityFieldNone identityField = iota
	identityFieldGender
	identityFieldAge
	identityFieldHanded
	identityFieldAlign
)

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

// SetCatalog enables the multi-step chargen flow. Passing nil keeps
// the legacy single-name flow.
func (m *CharacterCreate) SetCatalog(c *chargen.Catalog) { m.catalog = c }

// SetItems wires the item repo so the equipment substep can spawn
// the picked starting-equipment bundle into the new character's
// inventory at finalize time. nil silently disables spawning so
// existing tests with no item repo wired still pass.
func (m *CharacterCreate) SetItems(r repo.ItemRepo) { m.items = r }

// SetSettings forwards the account-level AccountSettings so chargen
// can stamp PromptDefault onto the new character's prompt_template
// column and apply ColorOverride/WidthOverride to the session at
// promote time. Zero value is a no-op.
func (m *CharacterCreate) SetSettings(s repo.AccountSettings) { m.settings = s }

// SetOnCancel wires the action taken when the hub's [Q]uit confirm is
// answered Y. AccountMenu sets this to a closure that ReplaceMode's
// itself back onto the session so the player returns to where they
// came from. Leaving the hook nil keeps the legacy "wipe draft and
// loop on the name prompt" behaviour, which is the right fallback for
// the post-auth direct-to-chargen path on accounts with no
// characters.
func (m *CharacterCreate) SetOnCancel(f func(*telnet.Session) error) { m.onCancel = f }

func (m *CharacterCreate) Prompt(_ context.Context, _ *telnet.Session) string {
	if m.catalog == nil {
		return "Choose a character name: "
	}
	switch m.step {
	case chargenStepName:
		return "Choose a character name: "
	case chargenStepRace:
		return "> "
	case chargenStepHub:
		return "> "
	case chargenStepHubConfirm:
		return "[Y/N] "
	case chargenStepBackground:
		return "> "
	case chargenStepClass:
		return "> "
	case chargenStepAbilities:
		return "> "
	case chargenStepIdentity:
		return "Identity (gender/age/handed/align/roll | done) [back]: "
	case chargenStepFeat:
		return "> "
	case chargenStepSkills:
		return "> "
	case chargenStepChanneling:
		return "> "
	case chargenStepEquipment:
		return "> "
	case chargenStepReview:
		return "> "
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
		AccountID:      s.AccountID,
		Name:           name,
		AuthLevel:      repo.AuthLevelPlayer,
		PromptTemplate: m.settings.PromptDefault,
	})
	switch {
	case errors.Is(err, repo.ErrDuplicateCharacterName):
		return writeError(s, "Character name already taken. Choose another.")
	case err != nil:
		return writeError(s, "Character creation failed. Try again later.")
	}
	applyAccountSettings(s, m.settings)
	return promoteToGame(ctx, s, c, m.repo, m.game)
}

// handleMulti drives the substep state machine. The post-name/race
// flow is hub-driven: typing `back` from any substep (or from review)
// returns to the hub, where the player can pick a different row, hit
// [R]estart, or [Q]uit. `back` at the name step still errors out.
func (m *CharacterCreate) handleMulti(ctx context.Context, s *telnet.Session, line string) error {
	trimmed := strings.TrimSpace(line)
	lower := strings.ToLower(trimmed)

	// Confirm sub-state owns its own input — answer Y/N; anything else
	// re-prompts. Routed first so [Y/N] can't be hijacked by the verbs
	// below.
	if m.step == chargenStepHubConfirm {
		return m.applyHubConfirm(s, trimmed)
	}

	switch lower {
	case "":
		// Empty line on the hub or review re-renders so the player
		// can see the current state without losing context.
		switch m.step {
		case chargenStepHub:
			return m.writeHub(s)
		case chargenStepReview:
			return m.writeReview(s)
		}
		return nil
	case "back", "b":
		switch m.step {
		case chargenStepName:
			return writeError(s, "Already at the first step.")
		case chargenStepRace:
			m.step = chargenStepName
			return nil
		case chargenStepHub:
			// [B] from the hub is the same gesture as [Q]: confirm
			// exit-chargen.
			return m.beginCancelConfirm(s)
		case chargenStepReview:
			// Drop to hub for one render without re-auto-opening
			// review so the player can pick a different row to edit.
			m.suppressAutoReview = true
			m.step = chargenStepHub
			return m.writeHub(s)
		}
		// Any other substep returns to the hub.
		m.step = chargenStepHub
		return m.writeHub(s)
	case "cancel":
		// Legacy alias — route through the same confirm as [Q].
		return m.beginCancelConfirm(s)
	}

	switch m.step {
	case chargenStepName:
		return m.applyName(s, trimmed)
	case chargenStepRace:
		return m.applyRace(s, trimmed)
	case chargenStepHub:
		return m.applyHub(s, trimmed)
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
	case chargenStepChanneling:
		return m.applyChanneling(s, trimmed)
	case chargenStepEquipment:
		return m.applyEquipment(s, trimmed)
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
	return m.writeRaceMenu(s)
}

// raceRows is the slice-6 numbered race picker. The catalog only
// currently exposes Human and Ogier; future races slot in here
// without rewiring callers — applyRace consults this table for both
// the numeric pick and the bare-token pick.
var raceRows = [...]struct {
	id    string
	label string
	hint  string
}{
	{"human", "Human", "Versatile, mid-statured, every class available"},
	{"ogier", "Ogier", "Tall, deliberate, no channelers"},
}

func (m *CharacterCreate) writeRaceMenu(s *telnet.Session) error {
	if err := writeStepHeader(s, chargenStepRace); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("{{Race:}}::yellow|bold\r\n")
	for i, r := range raceRows {
		fmt.Fprintf(&b,
			"  {{%d)}}::gray {{%-8s}}::yellow|bold %s\r\n",
			i+1, defangChargenField(r.label), defangChargenField(r.hint))
	}
	b.WriteString("\r\n  Pick a number  ·  {{[B]}}::yellow ack to name\r\n")
	return s.WriteString(b.String())
}

func (m *CharacterCreate) applyRace(s *telnet.Session, input string) error {
	in := strings.ToLower(strings.TrimSpace(input))
	race := ""
	// Numeric pick.
	if idx, err := parsePositiveIndex(in, len(raceRows)); err == nil {
		race = raceRows[idx].id
	} else {
		// Bare token (human / ogier) — preserved so existing tests and
		// muscle memory keep working.
		for _, r := range raceRows {
			if in == r.id {
				race = r.id
				break
			}
		}
	}
	if race == "" {
		return writeError(s,
			"Pick a number, or type 'human' or 'ogier'.")
	}
	m.draft.Race = race
	m.step = chargenStepHub
	return m.writeHub(s)
}

func (m *CharacterCreate) writeBackgroundMenu(s *telnet.Session) error {
	bgs := m.catalog.BackgroundsForRace(m.draft.Race)
	if len(bgs) == 0 {
		// Catalog mis-seeded for this race; bounce the player back to
		// the hub rather than stranding them in a substep with no
		// choices.
		m.step = chargenStepHub
		if err := writeError(s,
			"No backgrounds available for that race."); err != nil {
			return err
		}
		return m.writeHub(s)
	}
	if err := writeStepHeader(s, chargenStepBackground); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("{{Backgrounds:}}::yellow|bold\r\n")
	for i, bg := range bgs {
		fmt.Fprintf(&b,
			"  {{%2d)}}::gray {{%-22s}}::yellow|bold %s\r\n",
			i+1,
			defangChargenField(bg.Name),
			defangChargenField(backgroundSummary(bg)))
	}
	b.WriteString("\r\n  Pick a number  {{[I]}}::yellow nfo <#>  {{[B]}}::yellow ack to hub\r\n")
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
	if m.draft.BackgroundID != bgs[bg].ID {
		// Background changed — equipment_options are bg-specific, so
		// drop a stale pick rather than carrying it across into a
		// different bundle table.
		m.draft.SelectedEquipmentOptionIdx = 0
	}
	m.draft.BackgroundID = bgs[bg].ID
	m.step = chargenStepHub
	return m.writeHub(s)
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
// the info verb (case-insensitive). Accepts the long form "info" /
// "details" and the single-letter shorthand "i" (matching the
// numbered-picker footer hint). "info" alone with no argument returns
// ("", true) so callers can surface a usage hint. The shorthand "i"
// requires a non-empty argument so a bare "i" doesn't accidentally
// trigger info — it also doubles as a one-letter token a bg id might
// start with, but bare-id picks resolve via pickFromList so the
// branch ordering still picks the right thing.
func stripInfoVerb(input string) (string, bool) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "", false
	}
	verb := strings.ToLower(fields[0])
	switch verb {
	case "info", "details":
		if len(fields) < 2 {
			return "", true
		}
		return strings.Join(fields[1:], " "), true
	case "i":
		if len(fields) < 2 {
			return "", false
		}
		return strings.Join(fields[1:], " "), true
	}
	return "", false
}

func (m *CharacterCreate) writeClassMenu(s *telnet.Session) error {
	cls := m.catalog.ClassesForRace(m.draft.Race)
	if len(cls) == 0 {
		m.step = chargenStepHub
		if err := writeError(s,
			"No classes available for that race."); err != nil {
			return err
		}
		return m.writeHub(s)
	}
	if err := writeStepHeader(s, chargenStepClass); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("{{Classes:}}::yellow|bold\r\n")
	for i, cl := range cls {
		fmt.Fprintf(&b,
			"  {{%2d)}}::gray {{%-18s}}::yellow|bold %s\r\n",
			i+1,
			defangChargenField(cl.Name),
			defangChargenField(classSummary(cl)))
	}
	b.WriteString("\r\n  Pick a number  {{[I]}}::yellow nfo <#>  {{[B]}}::yellow ack to hub\r\n")
	return s.WriteString(b.String())
}

// classSummary renders the one-line menu hint in plain English:
// toughness, fighting progression, and (when relevant) channeler.
// The d20 tokens (d10 / high BAB) are reserved for the info detail
// screen so the picker stays readable to a player who has never
// touched a d20 sheet.
func classSummary(cl *chargen.Class) string {
	parts := []string{hdLabel(cl.HitDie), babLabel(cl.BAB)}
	if cl.Channeler {
		parts = append(parts, "channeler")
	}
	return strings.Join(parts, " · ")
}

// hdLabel maps a hit-die value to a plain-English toughness label.
// d4=frail, d6=average, d8=hardy, d10=sturdy, d12=tough; unknown
// dice fall through to a stringified form so a future YAML edit
// renders something rather than nothing.
func hdLabel(d int) string {
	switch d {
	case 4:
		return "frail"
	case 6:
		return "average"
	case 8:
		return "hardy"
	case 10:
		return "sturdy"
	case 12:
		return "tough"
	}
	return fmt.Sprintf("d%d", d)
}

// babLabel maps a base-attack-bonus progression to a plain-English
// combat label. high=expert fighter, medium=trained fighter,
// low=novice fighter.
func babLabel(b chargen.BABProgression) string {
	switch b {
	case chargen.BABHigh:
		return "expert fighter"
	case chargen.BABMedium:
		return "trained fighter"
	case chargen.BABLow:
		return "novice fighter"
	}
	return string(b)
}

// saveLabel maps a save progression to a plain-English defensive
// label (used on the class info screen for Fortitude / Reflex /
// Will saves).
func saveLabel(s chargen.SaveProgression) string {
	switch s {
	case chargen.SaveHigh:
		return "expert"
	case chargen.SaveLow:
		return "novice"
	}
	return string(s)
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
	if m.draft.ClassID != cls[idx].ID {
		// Flipping class can flip channeler status. Clear any prior
		// channeling state so the hub re-derives the row from the new
		// class — n/a vs not-chosen — without carrying stale picks.
		m.draft.ChannelingInit = false
		m.draft.ChannelSource = 0
		m.draft.Affinities = 0
		m.draft.StartingWeaves = nil
	}
	m.draft.ClassID = cls[idx].ID
	m.step = chargenStepHub
	return m.writeHub(s)
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
	if err := writeFieldRow(s, "Toughness", fmt.Sprintf(
		"%s (d%d hit die per level)", hdLabel(cl.HitDie), cl.HitDie,
	)); err != nil {
		return err
	}
	if err := writeFieldRow(s, "Combat", fmt.Sprintf(
		"%s (%s BAB)", babLabel(cl.BAB), cl.BAB,
	)); err != nil {
		return err
	}
	if err := writeFieldRow(s, "Saves", fmt.Sprintf(
		"Fortitude %s · Reflex %s · Will %s",
		saveLabel(cl.SaveFort), saveLabel(cl.SaveRef), saveLabel(cl.SaveWill),
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

// abilityLabels mirrors abilityKeys ordering with the human-readable
// row label rendered in the picker.
var abilityLabels = [...]string{"Strength", "Dexterity", "Constitution", "Intelligence", "Wisdom", "Charisma"}

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
		// Highlight rows that are above the point-buy floor so the eye
		// can scan allocations at a glance — same pattern as the
		// skills picker.
		if score > pointBuyMinScore {
			fmt.Fprintf(&b,
				"  {{%2d)}}::gray {{%-12s}}::yellow|bold ({{%s}}::gray) score {{%2d}}::green|bold  cost %d  mod {{%+d}}::green\r\n",
				i+1, abilityLabels[i], strings.ToUpper(key),
				score, pointBuyCost(score), mod)
		} else {
			fmt.Fprintf(&b,
				"  {{%2d)}}::gray {{%-12s}}::yellow ({{%s}}::gray) score {{%2d}}::yellow  cost %d  mod {{%+d}}::gray\r\n",
				i+1, abilityLabels[i], strings.ToUpper(key),
				score, pointBuyCost(score), mod)
		}
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
	b.WriteString("\r\n  Pick a number then {{+}}::green|bold or {{-}}::red|bold to adjust  ·  {{[R]}}::yellow eset  ·  {{[D]}}::green|bold one\r\n")
	return s.WriteString(b.String())
}

// abilityModifier returns the d20 ability modifier floor((s-10)/2).
// Go integer division truncates toward zero, so we adjust for odd
// scores below 10 (e.g. 9 → -1, not 0) so racial/background-modified
// scores outside the point-buy 8..18 baseline still render correctly.
func abilityModifier(score int) int {
	diff := score - 10
	if diff < 0 && diff%2 != 0 {
		return (diff - 1) / 2
	}
	return diff / 2
}

// applyAbilities parses the abilities-substep input. Slice 3 makes
// the canonical form a numbered +/- adjuster (`1+`, `2 -`, …) so it
// matches the skills picker shape; the legacy `set <abil> <n>` /
// `<abil> <n>` forms keep working as a power-user fallback.
//
//	<n>+ / <n>- / <n> + / <n> -   bump score for ability row n by ±1
//	set <abil> <n>                set one score directly (8..18)
//	<abil> <n>                    same, without the verb
//	r / reset                     all back to 8
//	d / done                      return to the hub
//	show / blank line             re-render
func (m *CharacterCreate) applyAbilities(s *telnet.Session, input string) error {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return m.writeAbilitiesMenu(s)
	}
	verb := strings.ToLower(fields[0])

	switch verb {
	case "show":
		return m.writeAbilitiesMenu(s)
	case "r", "reset":
		for i := range m.draft.Abilities {
			m.draft.Abilities[i] = pointBuyMinScore
		}
		return m.writeAbilitiesMenu(s)
	case "d", "done", "next":
		if m.pointBuySpent() > pointBuyBudget {
			return writeError(s, "You're over budget; reduce a score or 'reset'.")
		}
		m.step = chargenStepHub
		return m.writeHub(s)
	}

	// "<n> +" / "<n> -" with a space.
	if len(fields) == 2 && (fields[1] == "+" || fields[1] == "-") {
		return m.applyAbilityBump(s, fields[0], fields[1] == "+")
	}
	// "<n>+" / "<n>-" without a space.
	if len(fields) == 1 {
		tok := fields[0]
		if n := len(tok); n >= 2 && (tok[n-1] == '+' || tok[n-1] == '-') {
			return m.applyAbilityBump(s, tok[:n-1], tok[n-1] == '+')
		}
	}

	// Allow either "set <abil> <n>" or "<abil> <n>".
	abil, scoreStr := "", ""
	switch {
	case verb == "set" && len(fields) == 3:
		abil, scoreStr = strings.ToLower(fields[1]), fields[2]
	case len(fields) == 2:
		abil, scoreStr = verb, fields[1]
	default:
		return writeError(s,
			"Pick a number then '+' or '-', or 'set <abil> <n>' / 'reset' / 'done'.")
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

// applyAbilityBump steps one ability's score up or down by 1 within
// [pointBuyMinScore..pointBuyMaxScore]. Over-budget bumps roll back,
// mirroring the set-verb refusal behaviour.
func (m *CharacterCreate) applyAbilityBump(s *telnet.Session, idTok string, up bool) error {
	num, err := strconv.Atoi(idTok)
	if err != nil || num < 1 || num > len(abilityKeys) {
		return writeError(s, fmt.Sprintf(
			"Pick a row number 1..%d.", len(abilityKeys)))
	}
	idx := num - 1
	prev := int(m.draft.Abilities[idx])
	next := prev
	if up {
		next++
	} else {
		next--
	}
	if next < pointBuyMinScore {
		return writeError(s, fmt.Sprintf(
			"Already at the floor (%d).", pointBuyMinScore))
	}
	if next > pointBuyMaxScore {
		return writeError(s, fmt.Sprintf(
			"Cap is %d.", pointBuyMaxScore))
	}
	m.draft.Abilities[idx] = int8(next)
	if m.pointBuySpent() > pointBuyBudget {
		m.draft.Abilities[idx] = int8(prev)
		return writeError(s, fmt.Sprintf(
			"Not enough points (would cost %d, %d remaining).",
			pointBuyCost(next)-pointBuyCost(prev),
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

// deriveLevel1Vitals computes HP/Defense/Saves for a freshly-created
// level-1 character. HP takes the class hit die at max (d20 level-1
// convention) plus Con mod, floored at 1. Defense is 10 + Dex mod;
// armor and class defense bonuses land with the equipment slice.
// Saves use d20 base progression (high=+2, low=+0) plus the ability
// mod (Fort=Con, Ref=Dex, Will=Wis).
func deriveLevel1Vitals(cl *chargen.Class, ab creature.Abilities) (hp int32, defense int16, saves creature.Saves) {
	conMod := abilityModifier(int(ab.Con.Current))
	dexMod := abilityModifier(int(ab.Dex.Current))
	wisMod := abilityModifier(int(ab.Wis.Current))

	hpVal := cl.HitDie + conMod
	if hpVal < 1 {
		hpVal = 1
	}
	hp = int32(hpVal)
	defense = int16(10 + dexMod)
	saves = creature.Saves{
		Fort: int16(baseSaveBonus(cl.SaveFort) + conMod),
		Ref:  int16(baseSaveBonus(cl.SaveRef) + dexMod),
		Will: int16(baseSaveBonus(cl.SaveWill) + wisMod),
	}
	return hp, defense, saves
}

// baseSaveBonus is the level-1 base for a high (+2) or low (+0) save
// progression. Anything else (catalog drift) falls back to 0.
func baseSaveBonus(p chargen.SaveProgression) int {
	if p == chargen.SaveHigh {
		return 2
	}
	return 0
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
	if bg != nil && m.draft.SelectedEquipmentOptionIdx > 0 &&
		m.draft.SelectedEquipmentOptionIdx <= len(bg.EquipmentOptions) {
		if err := openLoadout(); err != nil {
			return err
		}
		opt := bg.EquipmentOptions[m.draft.SelectedEquipmentOptionIdx-1]
		if err := writeFieldRow(s, "Equipment", opt.Label); err != nil {
			return err
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

	// Channeler picks. Only renders when the chosen class is a
	// channeler — the substep is silently skipped otherwise.
	if m.classIsChanneler() {
		if err := s.WriteString("\r\n{{Channeling}}::cyan|bold\r\n"); err != nil {
			return err
		}
		source := "saidar"
		if m.draft.ChannelSource == creature.SourceSaidin {
			source = "saidin"
		}
		if err := writeFieldRow(s, "Source", source); err != nil {
			return err
		}
		if picked := powerSetFlags(m.draft.Affinities); len(picked) > 0 {
			labels := make([]string, 0, len(picked))
			for _, p := range picked {
				labels = append(labels, powerNames[int(p)])
			}
			if err := writeFieldRow(s, "Affinities",
				strings.Join(labels, ", ")); err != nil {
				return err
			}
		}
		if len(m.draft.StartingWeaves) > 0 {
			weaveLabels := make([]string, 0, len(m.draft.StartingWeaves))
			for _, id := range m.draft.StartingWeaves {
				name := id
				if w, ok := m.catalog.Weave(id); ok {
					name = w.Name
				}
				weaveLabels = append(weaveLabels, name)
			}
			if err := writeFieldRow(s, "Weaves",
				strings.Join(weaveLabels, ", ")); err != nil {
				return err
			}
		}
	}

	if err := writeRule(s); err != nil {
		return err
	}
	if err := s.WriteString(
		"  {{1)}}::gray Confirm and finalise\r\n",
	); err != nil {
		return err
	}
	rows := m.hubRows()
	for _, r := range rows {
		if r.number == 0 {
			continue
		}
		if err := s.WriteString(fmt.Sprintf(
			"  {{%d)}}::gray Revise %s\r\n",
			r.number+1, display.Defang(r.label, ""),
		)); err != nil {
			return err
		}
	}
	return s.WriteString(
		"  Pick a number, or {{Y}}::green|bold to confirm / " +
			"{{B}}::yellow to revisit the build hub.\r\n")
}

// applyReview routes one line of input on the review screen. Numbered
// `1` (or `Y`/`yes`) finalises; `2..N` jumps to the corresponding hub
// row's substep so the player can revise. `B`/`back` and `cancel` are
// intercepted by handleMulti before they reach this handler.
func (m *CharacterCreate) applyReview(ctx context.Context, s *telnet.Session, input string) error {
	in := strings.TrimSpace(input)
	switch strings.ToLower(in) {
	case "y", "yes", "1":
		return m.finaliseReview(ctx, s)
	}
	rows := m.hubRows()
	idx, err := parsePositiveIndex(in, hubMaxNumber(rows)+1)
	if err != nil || idx < 1 {
		return writeError(s,
			"Pick a number from the list, or 'Y' to confirm.")
	}
	row := hubRowByNumber(rows, idx)
	if row == nil || !row.available {
		return writeError(s, "That row is not editable.")
	}
	return m.enterSubstep(s, row.target)
}

// finaliseReview commits the draft via CharacterRepo.Create and
// promotes the session into the game. Pulled out of applyReview so
// the numeric "1" and the textual "yes"/"y" both land on one
// implementation.
func (m *CharacterCreate) finaliseReview(ctx context.Context, s *telnet.Session) error {
	// Identity must be stamped before commit — otherwise GenderNone /
	// zero height/weight would persist. The hub gates auto-opening
	// review on draftComplete (which checks IdentitySet), so this is
	// defence in depth against a hypothetical jump-back path that
	// lands here without the row filled.
	if !m.draft.IdentitySet {
		return m.enterSubstep(s, chargenStepIdentity)
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
	hp, defense, saves := deriveLevel1Vitals(cl, core.Abilities)
	core.HPMax = hp
	core.HPCurrent = hp
	core.Defense = defense
	core.Saves = saves

	c, err := m.repo.Create(ctx, repo.Character{
		AccountID:      s.AccountID,
		Name:           m.draft.Name,
		AuthLevel:      repo.AuthLevelPlayer,
		Race:           race,
		Background:     bg.Enum,
		ClassLevels:    map[creature.Class]int8{cl.Enum: 1},
		Core:           core,
		HeightCm:       m.draft.HeightCm,
		WeightKg:       m.draft.WeightKg,
		Age:            m.draft.Age,
		Handedness:     m.draft.Handedness,
		Feats:          m.buildFeatIDs(),
		Skills:         m.buildSkills(),
		Channeling:     m.buildChanneling(),
		PromptTemplate: m.settings.PromptDefault,
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

	// Spawn the picked starting-equipment bundle and auto-equip the
	// outfit / armor / shield / primary weapon. Best-effort: a partial
	// failure here doesn't unwind the character row — the player can
	// recover via a future GM tool / reroll. A nil items repo (legacy
	// tests, dev fixtures) silently skips the spawn.
	if m.items != nil && m.catalog != nil {
		if err := m.applyStartingEquipment(ctx, &c); err != nil {
			slog.Warn("chargen: starting-equipment spawn failed",
				"char", c.ID, "bg", bg.ID, "error", err)
		}
	}

	m.step = chargenStepDone
	applyAccountSettings(s, m.settings)
	return promoteToGame(ctx, s, c, m.repo, m.game)
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
