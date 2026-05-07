package mode

// chargen_hub.go owns the post-name/race "build hub" — the sheet-and-
// numbered-router screen that replaces the original linear chargen
// waterfall. After the player picks a name and a race, every other
// substep is reachable from this hub, and every substep returns here
// when "done." Once every required row is filled the hub auto-opens
// the review screen so the player can finalise.
//
// Hub UI conventions mirror the post-login account menu (account_menu.go
// and siblings): numbered choices, [B]/[R]/[Q] navigation, inline
// [Y/N] confirms drawn from internal/display.

import (
	"fmt"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/telnet"
)

// hubRow enumerates the destinations the build-hub menu offers. The
// numbered list rendered to the player is a contiguous 1..len(rows)
// slice; channeling collapses into a non-selectable "n/a" display row
// for non-channeler classes but never disappears, so the sheet stays
// visually stable as the player tweaks earlier choices.
type hubRow struct {
	number    int        // 1-based display number; 0 for the n/a channeling row
	label     string     // left-column label
	status    string     // right-column status (chosen value or "— not chosen —")
	target    chargenStep
	available bool       // false → row renders dimmed and refuses selection
}

// writeHub renders the build hub. When draftComplete is true and the
// suppressAutoReview latch is clear, this routes straight to the
// review screen instead — the design contract is "as soon as
// everything is filled, show the player the final sheet."
func (m *CharacterCreate) writeHub(s *telnet.Session) error {
	if m.draftComplete() && !m.suppressAutoReview {
		m.step = chargenStepReview
		return m.writeReview(s)
	}
	// suppressAutoReview is a one-shot — clear it so the *next* hub
	// render (post a substep edit) can re-auto-open review again.
	m.suppressAutoReview = false

	if err := display.SectionHeader(s, "Character build"); err != nil {
		return err
	}
	rows := m.hubRows()
	if err := m.writeHubSheet(s, rows); err != nil {
		return err
	}
	if err := display.Rule(s); err != nil {
		return err
	}
	if err := m.writeHubMenu(s, rows); err != nil {
		return err
	}
	if err := display.Rule(s); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte("  [R]estart   [Q]uit\r\n\r\n")); err != nil {
		return err
	}
	return s.WriteRaw([]byte(hubRangeHint(rows) + " >\r\n"))
}

// writeHubSheet emits the top half of the hub: name + race (the two
// pre-hub picks, always filled by the time we land here) plus every
// hubRow's label/status pair.
func (m *CharacterCreate) writeHubSheet(s *telnet.Session, rows []hubRow) error {
	if err := display.FieldRow(s, "Name", m.draft.Name, chargenLabelGutter); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Race", raceLabel(m.draft.Race), chargenLabelGutter); err != nil {
		return err
	}
	for _, r := range rows {
		if err := display.FieldRow(s, r.label, r.status, chargenLabelGutter); err != nil {
			return err
		}
	}
	return nil
}

// writeHubMenu emits the numbered router. The non-selectable
// channeling row (number == 0) renders dimmed without a digit.
func (m *CharacterCreate) writeHubMenu(s *telnet.Session, rows []hubRow) error {
	var b strings.Builder
	for _, r := range rows {
		if r.number == 0 {
			fmt.Fprintf(&b, "  {{   %s — not applicable for your class}}::gray\r\n",
				display.Defang(r.label, ""))
			continue
		}
		fmt.Fprintf(&b, "  {{%d)}}::gray %s\r\n", r.number, display.Defang(r.label, ""))
	}
	return s.WriteString(b.String())
}

// hubRows snapshots the per-row state for both the sheet view and
// the numbered router. Order matches chargenStep numeric order so the
// labels read top-to-bottom in the player's natural mental model. The
// numbering is deliberately stable: rows 1..6 are always visible and
// always selectable. The channeling row is row 7 for channeler
// classes and a non-selectable n/a marker for everyone else, so a
// player learns "abilities is 3" once and that mapping never moves.
func (m *CharacterCreate) hubRows() []hubRow {
	rows := []hubRow{
		{number: 1, label: "Background", target: chargenStepBackground, status: m.bgStatus(), available: true},
		{number: 2, label: "Class", target: chargenStepClass, status: m.classStatus(), available: true},
		{number: 3, label: "Abilities", target: chargenStepAbilities, status: m.abilitiesStatus(), available: true},
		{number: 4, label: "Identity", target: chargenStepIdentity, status: m.identityStatus(), available: true},
		{number: 5, label: "Feat", target: chargenStepFeat, status: m.featStatus(), available: true},
		{number: 6, label: "Skills", target: chargenStepSkills, status: m.skillsStatus(), available: true},
	}
	if m.classIsChanneler() {
		rows = append(rows, hubRow{
			number: 7, label: "Channeling", target: chargenStepChanneling,
			status: m.channelingStatus(), available: true,
		})
		rows = append(rows, hubRow{
			number: 8, label: "Equipment", target: chargenStepEquipment,
			status: m.equipmentStatus(), available: true,
		})
	} else {
		rows = append(rows, hubRow{
			number: 0, label: "Channeling", target: chargenStepChanneling,
			status: m.channelingStatus(), available: false,
		})
		rows = append(rows, hubRow{
			number: 7, label: "Equipment", target: chargenStepEquipment,
			status: m.equipmentStatus(), available: true,
		})
	}
	return rows
}

// hubRangeHint mirrors account_menu.rangeHint but for the hub's
// dynamically-sized router.
func hubRangeHint(rows []hubRow) string {
	first, last := 0, 0
	for _, r := range rows {
		if r.number == 0 {
			continue
		}
		if first == 0 {
			first = r.number
		}
		last = r.number
	}
	if first == 0 {
		return "[]"
	}
	if first == last {
		return fmt.Sprintf("[%d]", first)
	}
	return fmt.Sprintf("[%d-%d]", first, last)
}

// applyHub dispatches one line of input on the hub view. Numbered
// choices route to the corresponding substep and call its init helper;
// [R]/[Q] open the matching [Y/N] confirm; anything else echoes a
// concise error and re-prompts.
func (m *CharacterCreate) applyHub(s *telnet.Session, line string) error {
	in := strings.TrimSpace(line)
	switch strings.ToLower(in) {
	case "":
		return m.writeHub(s)
	case "r", "restart":
		return m.beginRestartConfirm(s)
	case "q", "quit", "exit":
		return m.beginCancelConfirm(s)
	}
	rows := m.hubRows()
	idx, err := parsePositiveIndex(in, hubMaxNumber(rows))
	if err != nil {
		return writeError(s, "Invalid choice. "+hubRangeHint(rows)+" or [R]estart / [Q]uit.")
	}
	row := hubRowByNumber(rows, idx+1)
	if row == nil || !row.available {
		// Picking the dimmed channeling number is the most likely path
		// here; surface a focused error rather than a generic one.
		if row != nil && row.target == chargenStepChanneling {
			return writeError(s, "Channeling is not applicable for your class.")
		}
		return writeError(s, "Invalid choice. "+hubRangeHint(rows)+".")
	}
	return m.enterSubstep(s, row.target)
}

// hubMaxNumber returns the largest selectable number in rows so the
// numeric parser knows the upper bound. Defensive zero when the hub is
// empty (shouldn't happen in production).
func hubMaxNumber(rows []hubRow) int {
	n := 0
	for _, r := range rows {
		if r.number > n {
			n = r.number
		}
	}
	return n
}

func hubRowByNumber(rows []hubRow, n int) *hubRow {
	for i := range rows {
		if rows[i].number == n {
			return &rows[i]
		}
	}
	return nil
}

// enterSubstep transitions the hub into the named substep, calling
// the init helper and rendering the substep menu. Substep handlers
// route back to the hub on "done" or [B]ack.
func (m *CharacterCreate) enterSubstep(s *telnet.Session, target chargenStep) error {
	switch target {
	case chargenStepBackground:
		m.step = chargenStepBackground
		return m.writeBackgroundMenu(s)
	case chargenStepClass:
		m.step = chargenStepClass
		return m.writeClassMenu(s)
	case chargenStepAbilities:
		m.step = chargenStepAbilities
		m.initAbilitiesIfNeeded()
		return m.writeAbilitiesMenu(s)
	case chargenStepIdentity:
		m.step = chargenStepIdentity
		m.initIdentityIfNeeded()
		// Re-entering identity always lands on the sub-hub, never on
		// a half-finished field prompt from a prior visit.
		m.pendingIdentityField = identityFieldNone
		return m.writeIdentityMenu(s)
	case chargenStepFeat:
		m.step = chargenStepFeat
		m.initFeatStepIfNeeded()
		return m.writeFeatMenu(s)
	case chargenStepSkills:
		m.step = chargenStepSkills
		m.initSkillsStepIfNeeded()
		return m.writeSkillsMenu(s)
	case chargenStepChanneling:
		m.step = chargenStepChanneling
		m.initChannelingStepIfNeeded()
		// Always re-enter at the affinities stage so the player can
		// revise without backtracking through a half-finished weave
		// list.
		m.channelingStage = channelingStageAffinities
		return m.writeChannelingMenu(s)
	case chargenStepEquipment:
		m.step = chargenStepEquipment
		m.initEquipmentStepIfNeeded()
		return m.writeEquipmentMenu(s)
	}
	return nil
}

// beginRestartConfirm pivots into the [Y/N] confirm for the "wipe the
// draft and start over from the name step" gesture. Cleared from the
// hub or any substep via the legacy `cancel` verb path too.
func (m *CharacterCreate) beginRestartConfirm(s *telnet.Session) error {
	m.pendingConfirm = confirmRestart
	m.step = chargenStepHubConfirm
	if err := display.SectionHeader(s, "Restart character creation"); err != nil {
		return err
	}
	return s.WriteRaw([]byte("  Wipe everything and start over? [Y/N] >\r\n"))
}

// beginCancelConfirm pivots into the [Y/N] confirm for the "exit
// chargen entirely" gesture. With onCancel wired (AccountMenu path) Y
// returns the player to the menu they came from. Without it
// (post-auth direct-to-chargen path for accounts with no characters)
// Y collapses to the same restart-in-place behaviour today's `cancel`
// verb implements, since there's no menu to return to.
func (m *CharacterCreate) beginCancelConfirm(s *telnet.Session) error {
	m.pendingConfirm = confirmCancel
	m.step = chargenStepHubConfirm
	if err := display.SectionHeader(s, "Cancel character creation"); err != nil {
		return err
	}
	return s.WriteRaw([]byte("  Exit and discard this character? [Y/N] >\r\n"))
}

// applyHubConfirm answers the [Y/N] prompt for the active confirm.
// Empty / N / B / cancel returns to the hub view with the draft
// untouched. Y commits the destructive action.
func (m *CharacterCreate) applyHubConfirm(s *telnet.Session, line string) error {
	in := strings.TrimSpace(strings.ToLower(line))
	switch in {
	case "y", "yes":
		switch m.pendingConfirm {
		case confirmRestart:
			m.draft = chargenDraft{}
			m.pendingConfirm = confirmNone
			m.suppressAutoReview = false
			m.step = chargenStepName
			return s.WriteString(
				"{{Cancelled. Restarting character creation.}}::yellow\r\n")
		case confirmCancel:
			m.pendingConfirm = confirmNone
			m.suppressAutoReview = false
			if m.onCancel != nil {
				return m.onCancel(s)
			}
			// No menu to return to — fall back to the in-place reset.
			m.draft = chargenDraft{}
			m.step = chargenStepName
			return s.WriteString(
				"{{Cancelled. Restarting character creation.}}::yellow\r\n")
		}
		// Defensive: unknown confirm kind, drop back to hub.
		m.pendingConfirm = confirmNone
		m.step = chargenStepHub
		return m.writeHub(s)
	case "", "n", "no", "b", "back", "cancel":
		m.pendingConfirm = confirmNone
		m.step = chargenStepHub
		return m.writeHub(s)
	}
	return writeError(s, "Please answer Y or N.")
}

// draftComplete returns true when every required hub row is filled —
// the trigger for auto-opening the review screen.
func (m *CharacterCreate) draftComplete() bool {
	if m.draft.BackgroundID == "" || m.draft.ClassID == "" {
		return false
	}
	if !m.draft.AbilitiesSet || !m.draft.IdentitySet {
		return false
	}
	if !m.featRowComplete() {
		return false
	}
	if !m.draft.SkillsInit {
		return false
	}
	if m.classIsChanneler() && !m.channelingRowComplete() {
		return false
	}
	if !m.equipmentRowComplete() {
		return false
	}
	return true
}

// featRowComplete reports whether the feat row counts as filled. A
// background with zero choosable feats marks the row complete as soon
// as the background is locked in (the bonus feats are auto-applied at
// commit time and need no player input).
func (m *CharacterCreate) featRowComplete() bool {
	bg, _ := m.catalog.Background(m.draft.BackgroundID)
	if bg == nil {
		return false
	}
	if m.draft.SelectedFeatID != "" {
		return true
	}
	return len(m.catalog.FeatsForBackground(bg.ID)) == 0
}

// channelingRowComplete mirrors the substep's "done" guard: exactly
// channelerAffinityCount affinities and channelerStartingWeaveCount
// starting weaves picked.
func (m *CharacterCreate) channelingRowComplete() bool {
	if !m.draft.ChannelingInit {
		return false
	}
	if len(powerSetFlags(m.draft.Affinities)) != channelerAffinityCount {
		return false
	}
	if len(m.draft.StartingWeaves) != channelerStartingWeaveCount {
		return false
	}
	return true
}

// notChosen is the placeholder rendered for an empty hub-row status.
const notChosen = "— not chosen —"

func (m *CharacterCreate) bgStatus() string {
	if m.draft.BackgroundID == "" {
		return notChosen
	}
	if bg, ok := m.catalog.Background(m.draft.BackgroundID); ok && bg != nil {
		return fmt.Sprintf("%s (%s)", bg.Name, bg.ID)
	}
	return m.draft.BackgroundID
}

func (m *CharacterCreate) classStatus() string {
	if m.draft.ClassID == "" {
		return notChosen
	}
	if cl, ok := m.catalog.Class(m.draft.ClassID); ok && cl != nil {
		return fmt.Sprintf("%s (%s)", cl.Name, cl.ID)
	}
	return m.draft.ClassID
}

func (m *CharacterCreate) abilitiesStatus() string {
	if !m.draft.AbilitiesSet {
		return "— not assigned —"
	}
	parts := make([]string, 0, len(abilityKeys))
	for i, key := range abilityKeys {
		parts = append(parts, fmt.Sprintf("%s %d",
			strings.ToUpper(key), m.draft.Abilities[i]))
	}
	return strings.Join(parts, "  ")
}

func (m *CharacterCreate) identityStatus() string {
	if !m.draft.IdentitySet {
		return "— defaults —"
	}
	return fmt.Sprintf("%s, age %d", genderLabel(m.draft.Gender), m.draft.Age)
}

func (m *CharacterCreate) featStatus() string {
	if m.draft.BackgroundID == "" {
		return "(pick a background first)"
	}
	if m.draft.SelectedFeatID != "" {
		if f, ok := m.catalog.Feat(m.draft.SelectedFeatID); ok && f != nil {
			return f.Name
		}
		return m.draft.SelectedFeatID
	}
	if m.featRowComplete() {
		return "(none required)"
	}
	return notChosen
}

func (m *CharacterCreate) skillsStatus() string {
	if m.draft.BackgroundID == "" || m.draft.ClassID == "" || !m.draft.AbilitiesSet {
		return "(pick background, class, abilities first)"
	}
	if !m.draft.SkillsInit {
		return notChosen
	}
	spent := m.skillsSpent()
	return fmt.Sprintf("%d / %d points spent", spent, m.draft.SkillBudget)
}

func (m *CharacterCreate) channelingStatus() string {
	if !m.classIsChanneler() {
		return "n/a"
	}
	if !m.channelingRowComplete() {
		return notChosen
	}
	return fmt.Sprintf("%d affinities, %d weaves",
		len(powerSetFlags(m.draft.Affinities)), len(m.draft.StartingWeaves))
}

// equipmentStatus renders the hub-row status string for the
// equipment substep — bundle label when picked, "(pick a background
// first)" before bg is set, "— not chosen —" otherwise.
func (m *CharacterCreate) equipmentStatus() string {
	if m.draft.BackgroundID == "" {
		return "(pick a background first)"
	}
	bg, _ := m.catalog.Background(m.draft.BackgroundID)
	if bg == nil {
		return notChosen
	}
	idx := m.draft.SelectedEquipmentOptionIdx
	if idx < 1 || idx > len(bg.EquipmentOptions) {
		return notChosen
	}
	return bg.EquipmentOptions[idx-1].Label
}

// raceLabel renders a friendly race label for the hub sheet. The
// chargen flow only stores "human" / "ogier" today; future races slot
// in here without changing callers.
func raceLabel(race string) string {
	switch race {
	case "human":
		return "Human"
	case "ogier":
		return "Ogier"
	case "":
		return notChosen
	}
	return race
}

