package mode

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
	chargenStepReview
	chargenStepDone
)

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
}

func NewCharacterCreate(characters repo.CharacterRepo, game telnet.Mode) *CharacterCreate {
	return &CharacterCreate{repo: characters, game: game}
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
		return "Background id [back/cancel]: "
	case chargenStepClass:
		return "Class id [back/cancel]: "
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
		fmt.Fprintf(&b, "  %2d. %-16s %s\r\n", i+1, bg.ID, bg.Name)
	}
	return s.WriteRaw([]byte(b.String()))
}

func (m *CharacterCreate) applyBackground(s *telnet.Session, input string) error {
	bgs := m.catalog.BackgroundsForRace(m.draft.Race)
	bg := pickFromList(input, len(bgs), func(i int) string { return bgs[i].ID })
	if bg < 0 {
		return s.WriteRaw([]byte("Unknown background. Type the id or list number.\r\n"))
	}
	m.draft.BackgroundID = bgs[bg].ID
	m.step = chargenStepClass
	return m.writeClassMenu(s)
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
		channeler := ""
		if cl.Channeler {
			channeler = " (channeler)"
		}
		fmt.Fprintf(&b, "  %2d. %-16s %s%s\r\n", i+1, cl.ID, cl.Name, channeler)
	}
	return s.WriteRaw([]byte(b.String()))
}

func (m *CharacterCreate) applyClass(s *telnet.Session, input string) error {
	cls := m.catalog.ClassesForRace(m.draft.Race)
	idx := pickFromList(input, len(cls), func(i int) string { return cls[i].ID })
	if idx < 0 {
		return s.WriteRaw([]byte("Unknown class. Type the id or list number.\r\n"))
	}
	m.draft.ClassID = cls[idx].ID
	m.step = chargenStepReview
	return m.writeReview(s)
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
	return s.WriteRaw([]byte(b.String()))
}

func (m *CharacterCreate) applyReview(ctx context.Context, s *telnet.Session, input string) error {
	if !strings.EqualFold(input, "yes") && !strings.EqualFold(input, "y") {
		return s.WriteRaw([]byte("Type 'yes' to confirm, 'back' to revise, or 'cancel' to abort.\r\n"))
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

	c, err := m.repo.Create(ctx, repo.Character{
		AccountID:   s.AccountID,
		Name:        m.draft.Name,
		AuthLevel:   repo.AuthLevelPlayer,
		Race:        race,
		Background:  bg.Enum,
		ClassLevels: map[creature.Class]int8{cl.Enum: 1},
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
