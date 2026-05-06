package mode

// Heroic Characteristics (#14): gender, age, height, weight,
// handedness, and alignment. Height/weight roll Table 6-1 for the
// chosen race + background; the rest get conservative defaults the
// player can override. The name was already collected up-front in
// chargenStepName, so this step only covers the remaining identity
// fields.

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/telnet"
)

const (
	humanComeOfAge      = 20 // late teens / early 20s default per the rulebook
	ogierYoungHeroAge   = 95 // ogier "young heroes" run 90+
	maxIdentityAgeYears = 32000
	cmPerInch           = 2.54
	kgPerLb             = 0.45359237
)

// rollD rolls n dice with `sides` sides each. n must be >= 0.
func rollD(r *rand.Rand, n, sides int) int {
	total := 0
	for i := 0; i < n; i++ {
		total += 1 + r.Intn(sides)
	}
	return total
}

// rollHeightWeight implements the Table 6-1 procedure for the chosen
// race and background height modifier:
//
//  1. heightModTotal = race-specific height roll + background mod
//  2. heightIn = baseHeightIn + heightModTotal
//  3. weightLb = baseWeightLb + (race-specific weight roll * heightModTotal)
//
// Returns height in cm and weight in kg, rounded to int16 for storage
// and clamped to int16 to keep the persisted column safe even if a
// future racial table pushes weight past 32k lb.
func rollHeightWeight(r *rand.Rand, race string, gender creature.Gender, bgHeightModIn int) (int16, int16) {
	var baseHeightIn, baseWeightLb int
	var heightDiceN, heightDiceSides int
	var weightDiceN, weightDiceSides, weightAdd int

	switch race {
	case "ogier":
		if gender == creature.GenderFemale {
			baseHeightIn = 7*12 - 5 // 6 ft 7 in
			baseWeightLb = 19 * 10
		} else {
			baseHeightIn = 7 * 12 // 7 ft 0 in
			baseWeightLb = 27 * 10
		}
		heightDiceN, heightDiceSides = 2, 8
		weightDiceN, weightDiceSides, weightAdd = 1, 6, 1
	case "human", "":
		if gender == creature.GenderFemale {
			baseHeightIn = 5 * 12 // 5 ft 0 in
			baseWeightLb = 10 * 10
		} else {
			baseHeightIn = 5*12 + 4 // 5 ft 4 in
			baseWeightLb = 14 * 10
		}
		heightDiceN, heightDiceSides = 2, 4
		weightDiceN, weightDiceSides, weightAdd = 1, 8, 1
	default:
		// Unknown race tokens fall through to human defaults — the
		// chargen race step already validates the input, so this only
		// fires on programmer error during a refactor.
		baseHeightIn = 5*12 + 4
		baseWeightLb = 14 * 10
		heightDiceN, heightDiceSides = 2, 4
		weightDiceN, weightDiceSides, weightAdd = 1, 8, 1
	}

	heightModTotal := rollD(r, heightDiceN, heightDiceSides) + bgHeightModIn
	heightIn := baseHeightIn + heightModTotal
	if heightIn < 1 {
		heightIn = 1
	}
	weightLb := baseWeightLb + (rollD(r, weightDiceN, weightDiceSides)+weightAdd)*heightModTotal
	if weightLb < 1 {
		weightLb = 1
	}

	return clampInt16(math.Round(float64(heightIn) * cmPerInch)),
		clampInt16(math.Round(float64(weightLb) * kgPerLb))
}

// clampInt16 saturates a float to the int16 range so a future racial
// table or an extreme background mod can't silently wrap the
// persisted column.
func clampInt16(v float64) int16 {
	if v >= float64(math.MaxInt16) {
		return math.MaxInt16
	}
	if v <= float64(math.MinInt16) {
		return math.MinInt16
	}
	return int16(v)
}

// defaultAge picks a plausible starting age for the race. Humans land
// in the late teens / early 20s per the rulebook; Ogier "young heroes"
// run 90+. Players can override.
func defaultAge(race string) int16 {
	if race == "ogier" {
		return ogierYoungHeroAge
	}
	return humanComeOfAge
}

// backgroundHeightMod looks up the active background's HeightModIn,
// returning 0 when the catalog has no entry (defensive — the chargen
// flow already validated the BackgroundID).
func (m *CharacterCreate) backgroundHeightMod() int {
	if m.catalog == nil || m.draft.BackgroundID == "" {
		return 0
	}
	bg, _ := m.catalog.Background(m.draft.BackgroundID)
	if bg == nil {
		return 0
	}
	return bg.HeightModIn
}

// rerollHeightWeight re-rolls into the draft using the current race +
// gender + background. Called from the abilities → identity transition
// (via initIdentityIfNeeded), the explicit `roll` verb, and `gender`
// (which changes the base table).
func (m *CharacterCreate) rerollHeightWeight() {
	m.draft.HeightCm, m.draft.WeightKg = rollHeightWeight(
		m.randSource(), m.draft.Race, m.draft.Gender, m.backgroundHeightMod())
}

// initIdentityIfNeeded stamps defaults on first entry into the identity
// substep. Idempotent: re-entry via `back` from review preserves all
// prior selections (including a re-rolled height/weight).
func (m *CharacterCreate) initIdentityIfNeeded() {
	if m.draft.IdentitySet {
		return
	}
	m.draft.Gender = creature.GenderMale
	m.draft.Age = defaultAge(m.draft.Race)
	m.draft.Handedness = creature.HandRight
	m.draft.Alignment = creature.PostureGood
	m.rerollHeightWeight()
	m.draft.IdentitySet = true
}

// renderHeight pretty-prints cm as feet/inches for the menu line.
func renderHeight(cm int16) string {
	totalIn := int(math.Round(float64(cm) / cmPerInch))
	ft := totalIn / 12
	in := totalIn % 12
	return fmt.Sprintf("%d ft %d in (%d cm)", ft, in, cm)
}

// renderWeight pretty-prints kg as stone (1 stone = 10 lb in WoT) for
// the menu line.
func renderWeight(kg int16) string {
	lb := int(math.Round(float64(kg) / kgPerLb))
	stone := lb / 10
	rem := lb % 10
	if rem == 0 {
		return fmt.Sprintf("%d stone (%d lb / %d kg)", stone, lb, kg)
	}
	return fmt.Sprintf("%d stone %d lb (%d lb / %d kg)", stone, rem, lb, kg)
}

func genderLabel(g creature.Gender) string {
	switch g {
	case creature.GenderFemale:
		return "female"
	case creature.GenderMale:
		return "male"
	}
	return "unspecified"
}

func handLabel(h creature.Hand) string {
	switch h {
	case creature.HandLeft:
		return "left"
	case creature.HandAmbidextrous:
		return "ambidextrous"
	}
	return "right"
}

func postureLabel(p creature.Posture) string {
	switch p {
	case creature.PostureBad:
		return "bad"
	case creature.PostureEvil:
		return "evil"
	}
	return "good"
}

func (m *CharacterCreate) writeIdentityMenu(s *telnet.Session) error {
	// When awaiting a specific field's value, render only the field
	// prompt — keeps the screen tight and the next-line semantic
	// obvious.
	if m.pendingIdentityField != identityFieldNone {
		return m.writeIdentityFieldPrompt(s)
	}
	if err := writeStepHeader(s, chargenStepIdentity); err != nil {
		return err
	}
	rows := [...]struct {
		label, value string
	}{
		{"Gender", genderLabel(m.draft.Gender)},
		{"Age", fmt.Sprintf("%d", m.draft.Age)},
		{"Height/Weight", fmt.Sprintf("%s · %s",
			renderHeight(m.draft.HeightCm), renderWeight(m.draft.WeightKg))},
		{"Handedness", handLabel(m.draft.Handedness)},
		{"Alignment", postureLabel(m.draft.Alignment)},
	}
	var b strings.Builder
	for i, r := range rows {
		fmt.Fprintf(&b,
			"  {{%d)}}::gray {{%-13s}}::yellow|bold %s\r\n",
			i+1, defangChargenField(r.label), defangChargenField(r.value))
	}
	b.WriteString("\r\n  Pick a number to edit  ·  {{[R]}}::yellow re-roll height/weight  ·  {{[D]}}::green|bold one\r\n")
	return s.WriteString(b.String())
}

// writeIdentityFieldPrompt emits a one-line "  Field: <hint> >" so
// the player knows exactly what input the next line expects. Called
// only while pendingIdentityField is set.
func (m *CharacterCreate) writeIdentityFieldPrompt(s *telnet.Session) error {
	switch m.pendingIdentityField {
	case identityFieldGender:
		return s.WriteString("  Gender ({{m}}::yellow ale / {{f}}::yellow emale) >\r\n")
	case identityFieldAge:
		return s.WriteString("  Age (years, positive integer) >\r\n")
	case identityFieldHanded:
		return s.WriteString(
			"  Handedness ({{r}}::yellow ight / {{l}}::yellow eft / {{a}}::yellow mbidextrous) >\r\n")
	case identityFieldAlign:
		return s.WriteString(
			"  Alignment ({{good}}::green / {{bad}}::yellow / {{evil}}::red) >\r\n")
	}
	return nil
}

// applyIdentity dispatches identity input. Slice 4 makes the
// canonical form a numbered sub-hub: pick a row, then the next line
// is the value for that field. Picking row 3 (Height/Weight) re-rolls
// directly with no follow-up prompt. The legacy verb forms
// (gender m / age 25 / handed left / align bad / roll) all keep
// working at the top level so existing tests and muscle memory are
// undisturbed.
func (m *CharacterCreate) applyIdentity(s *telnet.Session, input string) error {
	trimmed := strings.TrimSpace(input)

	// Awaiting a specific field's value — route the line to that
	// field's parser. The latch only clears on successful validation;
	// a bad value re-prompts in place so the player isn't dumped back
	// to the menu after a typo.
	if m.pendingIdentityField != identityFieldNone {
		field := m.pendingIdentityField
		var (
			ok  bool
			err error
		)
		switch field {
		case identityFieldGender:
			ok, err = m.applyIdentityGender(s, strings.ToLower(trimmed))
		case identityFieldAge:
			ok, err = m.applyIdentityAge(s, trimmed)
		case identityFieldHanded:
			ok, err = m.applyIdentityHanded(s, strings.ToLower(trimmed))
		case identityFieldAlign:
			ok, err = m.applyIdentityAlign(s, strings.ToLower(trimmed))
		}
		if err != nil {
			return err
		}
		if !ok {
			// Validation failed; the handler already wrote an error
			// line. Re-show the field prompt so the next line is
			// interpreted as a retry rather than a menu pick.
			return m.writeIdentityFieldPrompt(s)
		}
		m.pendingIdentityField = identityFieldNone
		return nil
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return m.writeIdentityMenu(s)
	}
	verb := strings.ToLower(fields[0])
	arg := ""
	if len(fields) > 1 {
		arg = strings.ToLower(strings.Join(fields[1:], " "))
	}

	switch verb {
	case "show":
		return m.writeIdentityMenu(s)
	case "d", "done", "next":
		m.step = chargenStepHub
		return m.writeHub(s)
	case "r", "roll":
		m.rerollHeightWeight()
		return m.writeIdentityMenu(s)
	case "gender":
		_, err := m.applyIdentityGender(s, arg)
		return err
	case "age":
		_, err := m.applyIdentityAge(s, arg)
		return err
	case "handed", "handedness":
		_, err := m.applyIdentityHanded(s, arg)
		return err
	case "align", "alignment":
		_, err := m.applyIdentityAlign(s, arg)
		return err
	}

	// Bare numeric pick from the sub-hub.
	if len(fields) == 1 {
		switch verb {
		case "1":
			m.pendingIdentityField = identityFieldGender
			return m.writeIdentityFieldPrompt(s)
		case "2":
			m.pendingIdentityField = identityFieldAge
			return m.writeIdentityFieldPrompt(s)
		case "3":
			// Height/Weight has no value to type — picking the row
			// re-rolls directly.
			m.rerollHeightWeight()
			return m.writeIdentityMenu(s)
		case "4":
			m.pendingIdentityField = identityFieldHanded
			return m.writeIdentityFieldPrompt(s)
		case "5":
			m.pendingIdentityField = identityFieldAlign
			return m.writeIdentityFieldPrompt(s)
		}
	}

	return writeError(s,
		"Pick a number 1..5, or use a verb (gender / age / handed / align / roll / done).")
}

// applyIdentityGender returns (validated, writeErr). When validated
// is false the caller knows the input didn't parse; the handler
// already wrote an error line, so the caller's job is to keep the
// pending-field latch set and re-prompt. The verb-form callers
// ignore the bool — for them, validation failure is semantically
// the same as success since they always return to the top-level
// applyIdentity dispatcher anyway.
func (m *CharacterCreate) applyIdentityGender(s *telnet.Session, arg string) (bool, error) {
	switch arg {
	case "m", "male":
		m.draft.Gender = creature.GenderMale
	case "f", "female":
		m.draft.Gender = creature.GenderFemale
	default:
		return false, writeError(s, "Gender must be 'm' / 'male' or 'f' / 'female'.")
	}
	// Base height/weight depends on gender; re-roll so the line
	// stays consistent with the chosen gender.
	m.rerollHeightWeight()
	return true, m.writeIdentityMenu(s)
}

func (m *CharacterCreate) applyIdentityAge(s *telnet.Session, arg string) (bool, error) {
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 || n > maxIdentityAgeYears {
		return false, writeError(s, "Age must be a positive integer.")
	}
	m.draft.Age = int16(n)
	return true, m.writeIdentityMenu(s)
}

func (m *CharacterCreate) applyIdentityHanded(s *telnet.Session, arg string) (bool, error) {
	switch arg {
	case "r", "right":
		m.draft.Handedness = creature.HandRight
	case "l", "left":
		m.draft.Handedness = creature.HandLeft
	case "a", "ambi", "ambidextrous":
		m.draft.Handedness = creature.HandAmbidextrous
	default:
		return false, writeError(s, "Handedness must be 'r' / 'l' / 'a'.")
	}
	return true, m.writeIdentityMenu(s)
}

func (m *CharacterCreate) applyIdentityAlign(s *telnet.Session, arg string) (bool, error) {
	switch arg {
	case "good":
		m.draft.Alignment = creature.PostureGood
	case "bad":
		m.draft.Alignment = creature.PostureBad
	case "evil":
		m.draft.Alignment = creature.PostureEvil
	default:
		return false, writeError(s, "Alignment must be 'good', 'bad', or 'evil'.")
	}
	return true, m.writeIdentityMenu(s)
}
