package mode

// Channeler branch (#15 slice 2). Substep slots in between skills
// and review for classes whose YAML declares `channeler: true`
// (Initiate, Wilder). Non-channeler classes skip silently — the
// applySkills "done" handler advances directly to review.
//
// What gets picked here:
//
//   1. Source — auto-derived from gender (male=Saidin, female=Saidar).
//      Both eligible classes use channel_source: "either" today, so
//      no choice is presented. Future Asha'man/Aes Sedai variants
//      that hardcode one source can be honored without changing
//      this menu.
//   2. Affinities — exactly 2 of 5 (Air/Earth/Fire/Water/Spirit).
//      Standard d20 starting set. Affinity adjustment math at cast
//      time keys off this bitmask.
//   3. Starting weaves — exactly 3 from the level-0 catalog filtered
//      to weaves whose Power matches at least one selected affinity.
//
// Output is committed via buildChanneling at chargenStepReview into a
// *creature.Channeling and persisted via the new channeling_json
// column (migration 0033).

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/telnet"
)

const (
	// channelerAffinityCount is the number of Powers a starting
	// channeler picks. Standard d20 value; configurable later if
	// per-class variants need it.
	channelerAffinityCount = 2
	// channelerStartingWeaveCount is the number of level-0 weaves
	// the player picks at chargen.
	channelerStartingWeaveCount = 3
)

// powerNames maps creature.Power → display label. Index = enum value.
// Mirrors PowerAir..PowerSpirit declared in internal/creature.
var powerNames = [...]string{"Air", "Earth", "Fire", "Water", "Spirit"}

// powerIDs is the lower-case token used by the substep menu and
// applyChanneling parser. Matches yaml power tokens in weaves.yaml.
var powerIDs = [...]string{"air", "earth", "fire", "water", "spirit"}

// classIsChanneler returns true when the draft's class is flagged as
// a channeler in the catalog. Caller of applySkills "done" uses this
// to decide whether to advance into chargenStepChanneling or skip
// straight to review.
func (m *CharacterCreate) classIsChanneler() bool {
	cl, _ := m.catalog.Class(m.draft.ClassID)
	return cl != nil && cl.Channeler
}

// initChannelingStepIfNeeded primes the substep on first entry. The
// Source defaults from gender; affinities and weaves stay empty so
// the menu prompts the player.
func (m *CharacterCreate) initChannelingStepIfNeeded() {
	if m.draft.ChannelingInit {
		return
	}
	switch m.draft.Gender {
	case creature.GenderMale:
		m.draft.ChannelSource = creature.SourceSaidin
	default:
		// Female or unspecified default to Saidar. Saidar-eligible
		// catalog content is the broader set today (initiate +
		// wilder both accept either source).
		m.draft.ChannelSource = creature.SourceSaidar
	}
	m.draft.Affinities = 0
	m.draft.StartingWeaves = nil
	m.draft.ChannelingInit = true
}

// eligibleStartingWeaves returns the level-0 weaves whose Power
// matches at least one bit of the supplied affinity bitmask. When
// affinities is zero (player hasn't picked yet) the result is empty
// so the menu shows "(pick affinities first)".
func eligibleStartingWeaves(cat *chargen.Catalog, affinities creature.PowerSet) []*chargen.Weave {
	if affinities == 0 {
		return nil
	}
	all := cat.WeavesAtLevel(0)
	out := make([]*chargen.Weave, 0, len(all))
	for _, w := range all {
		p, ok := powerEnum(w.Power)
		if !ok {
			continue
		}
		if affinities&(1<<uint(p)) == 0 {
			continue
		}
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// powerEnum maps a YAML power token ("Air", "Fire", …) to the
// creature.Power enum. Tokens are matched case-insensitively to
// be permissive about catalog content.
func powerEnum(token string) (creature.Power, bool) {
	t := strings.ToLower(token)
	for i, name := range powerIDs {
		if t == name {
			return creature.Power(i), true
		}
	}
	return 0, false
}

// affinityIdx maps a lower-case power token to its
// creature.Power index, or -1 on miss.
func affinityIdx(token string) int {
	t := strings.ToLower(token)
	for i, name := range powerIDs {
		if t == name {
			return i
		}
	}
	return -1
}

// powerSetFlags returns the set bits of ps in canonical order.
func powerSetFlags(ps creature.PowerSet) []creature.Power {
	out := make([]creature.Power, 0, 5)
	for i := 0; i < len(powerIDs); i++ {
		if ps&(1<<uint(i)) != 0 {
			out = append(out, creature.Power(i))
		}
	}
	return out
}

func (m *CharacterCreate) writeChannelingMenu(s *telnet.Session) error {
	if err := writeStepHeader(s, chargenStepChanneling); err != nil {
		return err
	}

	source := "saidar"
	gender := genderLabel(m.draft.Gender)
	if m.draft.ChannelSource == creature.SourceSaidin {
		source = "saidin"
	}
	if err := writeFieldRow(s, "Source",
		fmt.Sprintf("%s (from gender: %s)", source, gender)); err != nil {
		return err
	}

	// Affinity menu.
	var b strings.Builder
	fmt.Fprintf(&b,
		"\r\n{{Affinities (pick exactly %d of 5):}}::yellow|bold\r\n",
		channelerAffinityCount)
	for i, name := range powerNames {
		mark := " "
		if m.draft.Affinities&(1<<uint(i)) != 0 {
			mark = "*"
		}
		if mark == "*" {
			fmt.Fprintf(&b,
				"  {{%s}}::green|bold {{%2d.}}::gray {{%s}}::yellow|bold\r\n",
				mark, i+1, defangChargenField(name))
		} else {
			fmt.Fprintf(&b,
				"    {{%2d.}}::gray {{%s}}::yellow\r\n",
				i+1, defangChargenField(name))
		}
	}
	picked := powerSetFlags(m.draft.Affinities)
	if len(picked) > 0 {
		labels := make([]string, 0, len(picked))
		for _, p := range picked {
			labels = append(labels, powerNames[int(p)])
		}
		fmt.Fprintf(&b, "  {{Selected:}}::green|bold {{%s}}::green\r\n",
			defangChargenField(strings.Join(labels, ", ")))
	}

	// Starting weaves — only meaningful once affinities are set.
	weaves := eligibleStartingWeaves(m.catalog, m.draft.Affinities)
	if m.draft.Affinities == 0 {
		b.WriteString(
			"\r\n{{Starting weaves:}}::yellow|bold {{(pick affinities first)}}::gray\r\n")
	} else if len(weaves) == 0 {
		b.WriteString(
			"\r\n{{Starting weaves:}}::yellow|bold {{(no level-0 weaves match your affinities)}}::red\r\n")
	} else {
		fmt.Fprintf(&b,
			"\r\n{{Starting weaves (pick exactly %d of eligible):}}::yellow|bold\r\n",
			channelerStartingWeaveCount)
		selected := map[string]bool{}
		for _, id := range m.draft.StartingWeaves {
			selected[id] = true
		}
		for i, w := range weaves {
			mark := " "
			if selected[w.ID] {
				mark = "*"
			}
			if mark == "*" {
				fmt.Fprintf(&b,
					"  {{%s}}::green|bold {{%2d.}}::gray {{%-16s}}::yellow|bold {{%s}}::gray\r\n",
					mark, i+1,
					defangChargenField(w.ID), defangChargenField(w.Power))
			} else {
				fmt.Fprintf(&b,
					"    {{%2d.}}::gray {{%-16s}}::yellow {{%s}}::gray\r\n",
					i+1,
					defangChargenField(w.ID), defangChargenField(w.Power))
			}
		}
		if len(m.draft.StartingWeaves) > 0 {
			fmt.Fprintf(&b, "  {{Selected:}}::green|bold {{%s}}::green\r\n",
				defangChargenField(strings.Join(m.draft.StartingWeaves, ", ")))
		}
	}

	b.WriteString("\r\n")
	b.WriteString(fmt.Sprintf(
		"  {{affinities <a> <b>}}::yellow      pick exactly %d powers\r\n",
		channelerAffinityCount))
	b.WriteString(fmt.Sprintf(
		"  {{weaves <id> <id> <id>}}::yellow   pick exactly %d eligible weaves\r\n",
		channelerStartingWeaveCount))
	b.WriteString("  {{show}}::yellow                    re-render this menu\r\n")
	b.WriteString("  {{done}}::green|bold                   accept and continue\r\n")
	return s.WriteString(b.String())
}

func (m *CharacterCreate) applyChanneling(s *telnet.Session, input string) error {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return m.writeChannelingMenu(s)
	}
	verb := strings.ToLower(fields[0])
	rest := fields[1:]

	switch verb {
	case "show":
		return m.writeChannelingMenu(s)
	case "affinities", "affinity":
		return m.applyAffinities(s, rest)
	case "weaves", "weave":
		return m.applyStartingWeaves(s, rest)
	case "done", "next":
		if got := len(powerSetFlags(m.draft.Affinities)); got != channelerAffinityCount {
			return writeError(s, fmt.Sprintf(
				"Pick exactly %d affinities first (have %d).",
				channelerAffinityCount, got))
		}
		if len(m.draft.StartingWeaves) != channelerStartingWeaveCount {
			return writeError(s, fmt.Sprintf(
				"Pick exactly %d starting weaves first (have %d).",
				channelerStartingWeaveCount, len(m.draft.StartingWeaves)))
		}
		m.step = chargenStepReview
		return m.writeReview(s)
	}
	return writeError(s,
		"Usage: affinities <a> <b> | weaves <id> <id> <id> | show | done")
}

// applyAffinities parses N power tokens, validates count + dedup, and
// stores the result as a PowerSet bitmask. Picking affinities clears
// the prior weave selection — eligible weaves change with affinities.
func (m *CharacterCreate) applyAffinities(s *telnet.Session, tokens []string) error {
	if len(tokens) != channelerAffinityCount {
		return writeError(s, fmt.Sprintf(
			"Pick exactly %d affinities (got %d).",
			channelerAffinityCount, len(tokens)))
	}
	var bits creature.PowerSet
	seen := map[creature.Power]bool{}
	for _, tok := range tokens {
		idx := affinityIdx(tok)
		if idx < 0 {
			return writeError(s, fmt.Sprintf(
				"Unknown power %q. Choose from: %s.",
				tok, strings.Join(powerIDs[:], ", ")))
		}
		p := creature.Power(idx)
		if seen[p] {
			return writeError(s, fmt.Sprintf(
				"Duplicate affinity %q.", tok))
		}
		seen[p] = true
		bits |= 1 << uint(p)
	}
	if bits != m.draft.Affinities {
		// Affinity set changed — drop prior weave picks since the
		// eligibility filter just shifted under them.
		m.draft.StartingWeaves = nil
	}
	m.draft.Affinities = bits
	return m.writeChannelingMenu(s)
}

// applyStartingWeaves parses N weave ids, validates count + dedup +
// eligibility, and stores them on the draft.
func (m *CharacterCreate) applyStartingWeaves(s *telnet.Session, tokens []string) error {
	if len(tokens) != channelerStartingWeaveCount {
		return writeError(s, fmt.Sprintf(
			"Pick exactly %d starting weaves (got %d).",
			channelerStartingWeaveCount, len(tokens)))
	}
	eligible := eligibleStartingWeaves(m.catalog, m.draft.Affinities)
	if len(eligible) == 0 {
		return writeError(s, "Pick affinities before choosing weaves.")
	}
	eligibleIDs := map[string]bool{}
	for _, w := range eligible {
		eligibleIDs[w.ID] = true
	}
	picks := make([]string, 0, channelerStartingWeaveCount)
	seen := map[string]bool{}
	for _, tok := range tokens {
		id := strings.ToLower(tok)
		if !eligibleIDs[id] {
			return writeError(s, fmt.Sprintf(
				"Weave %q is not in your eligible list.", tok))
		}
		if seen[id] {
			return writeError(s, fmt.Sprintf(
				"Duplicate weave %q.", tok))
		}
		seen[id] = true
		picks = append(picks, id)
	}
	m.draft.StartingWeaves = picks
	return m.writeChannelingMenu(s)
}

// buildChanneling materialises the draft picks into a
// *creature.Channeling for repo.Character.Create. Only called when
// the class is a channeler; non-channeler characters get nil.
func (m *CharacterCreate) buildChanneling() *creature.Channeling {
	if !m.classIsChanneler() {
		return nil
	}
	cl, _ := m.catalog.Class(m.draft.ClassID)
	chType := creature.ChannelerInitiate
	if cl != nil && cl.ID == "wilder" {
		chType = creature.ChannelerWilder
	}
	weaves := append([]string(nil), m.draft.StartingWeaves...)
	return &creature.Channeling{
		GenderSource:   m.draft.ChannelSource,
		ChannelerType:  chType,
		Affinities:     m.draft.Affinities,
		WeavesKnownIDs: weaves,
	}
}
