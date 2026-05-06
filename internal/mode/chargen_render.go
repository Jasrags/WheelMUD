package mode

// Chargen render helpers: cfmt-styled, width-aware section headers,
// label rows, errors, and confirmations. Every chargen substep emits
// output through these helpers (or through Session.WriteString with
// cfmt tags directly) so:
//
//   1. The palette stays in lock-step with internal/cmd/look.go and
//      internal/news/render.go (cyan|bold titles, yellow|bold labels,
//      gray muted, red errors, green confirms). See
//      skills/mud/ui-expert/references/theme-and-cfmt-vocabulary.md.
//   2. Color downsampling via Session.ColorLevel works on every
//      output (no raw \x1b[...m bypass).
//   3. The 60-col floor is respected by capping rule width at the
//      negotiated NAWS width.
//
// Helpers are package-local; promotion to a shared internal/display
// package happens when a third caller appears (chargen + score + who
// at minimum).

import (
	"fmt"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/telnet"
)

const (
	// chargenStepCount is the total visible-step count rendered in
	// step headers and prompts. Keep this in sync with chargenStep
	// values that the player sees (name … review = 8).
	chargenStepCount = 8

	// chargenLabelGutter is the column width reserved for label text
	// in writeFieldRow. 14 cols + space + colon + space gives the
	// existing longest label ("Background skills:") room while keeping
	// shorter labels visually grouped.
	chargenLabelGutter = 14

	// chargenRuleMinWidth caps the section-header rule at this many
	// dashes when Session.Width is unset or extremely narrow. The
	// rule never grows past Session.Width or chargenRuleMaxWidth.
	chargenRuleMinWidth = 40
	chargenRuleMaxWidth = 78
)

// chargenStepLabel is the human-readable name shown in step headers
// and prompts for each substep. Out-of-band steps (Done) return "".
func chargenStepLabel(step chargenStep) string {
	switch step {
	case chargenStepName:
		return "Choose a name"
	case chargenStepRace:
		return "Race"
	case chargenStepBackground:
		return "Background"
	case chargenStepClass:
		return "Class"
	case chargenStepAbilities:
		return "Ability scores"
	case chargenStepIdentity:
		return "Identity"
	case chargenStepFeat:
		return "First-level feat"
	case chargenStepSkills:
		return "Skill ranks"
	case chargenStepChanneling:
		return "Channeling"
	case chargenStepReview:
		return "Review"
	}
	return ""
}

// chargenStepNumber maps the internal step enum to a 1-based "step n
// of chargenStepCount" presentation. chargenStepDone returns 0 (no
// header is rendered after commit).
func chargenStepNumber(step chargenStep) int {
	switch step {
	case chargenStepName:
		return 1
	case chargenStepRace:
		return 2
	case chargenStepBackground:
		return 3
	case chargenStepClass:
		return 4
	case chargenStepAbilities:
		return 5
	case chargenStepIdentity:
		return 6
	case chargenStepFeat:
		return 7
	case chargenStepSkills:
		return 7 // shares slot 7 visually with feats; both are "Build"
	case chargenStepChanneling:
		return 7 // also shares slot 7 — channelers see 3 sub-screens at "Build"
	case chargenStepReview:
		return 8
	}
	return 0
}

// ruleWidth picks the width for a chargen section-header rule based
// on the session's current NAWS-negotiated terminal width. Falls back
// to chargenRuleMinWidth when Session.Width is zero (test fixtures
// often skip NAWS).
func ruleWidth(s *telnet.Session) int {
	w := 0
	if s != nil {
		w = s.Width
	}
	if w <= 0 {
		w = chargenRuleMinWidth
	}
	if w > chargenRuleMaxWidth {
		w = chargenRuleMaxWidth
	}
	return w
}

// writeStepHeader emits an opening rule for the substep:
//
//	─── Step 3/8 — Background ──────────────────────────────
//
// The leading "─── " and trailing "─" run are coloured cyan|bold to
// match the look.go room-title palette. Renders one trailing CRLF so
// the menu body starts on its own line.
func writeStepHeader(s *telnet.Session, step chargenStep) error {
	num := chargenStepNumber(step)
	label := chargenStepLabel(step)
	if num == 0 || label == "" {
		return nil
	}
	w := ruleWidth(s)
	header := fmt.Sprintf("Step %d/%d — %s", num, chargenStepCount, label)
	leading := "─── "
	trailing := ""
	used := len(leading) + len(header) + 1 // " " before trailing run
	if used < w {
		trailing = " " + strings.Repeat("─", w-used)
	}
	line := fmt.Sprintf("{{%s%s%s}}::cyan|bold\r\n", leading, header, trailing)
	return s.WriteString(line)
}

// writeRule emits a plain divider sized to ruleWidth(s). Used to
// close info blocks in the chargen flow without restating the step
// label. Color is gray so it reads as quieter than the opening
// header.
func writeRule(s *telnet.Session) error {
	w := ruleWidth(s)
	line := fmt.Sprintf("{{%s}}::gray\r\n", strings.Repeat("─", w))
	return s.WriteString(line)
}

// writeFieldRow emits a "  label: value\r\n" row with the label
// padded to chargenLabelGutter columns and rendered yellow|bold,
// value rendered plain. Both label and value are passed through
// defangChargenField since the value side may carry catalog or
// player input.
//
// The label text MUST NOT contain cfmt tags itself — the helper
// wraps it. Pass plain strings.
func writeFieldRow(s *telnet.Session, label, value string) error {
	pad := chargenLabelGutter
	if len(label) > pad {
		pad = len(label)
	}
	line := fmt.Sprintf("  {{%-*s}}::yellow|bold %s\r\n",
		pad, defangChargenField(label)+":",
		defangChargenField(value))
	return s.WriteString(line)
}

// writeError renders one error line as
//
//	>> <msg>
//
// with the leading ">> " in red|bold and the message body in red.
// The text content of msg is preserved verbatim (after defang) so
// substring-based test assertions on error wording continue to pass.
func writeError(s *telnet.Session, msg string) error {
	return s.WriteString(fmt.Sprintf(
		"{{>> }}::red|bold {{%s}}::red\r\n",
		defangChargenField(msg)))
}

// writeOK renders one confirmation line as
//
//	✓ <msg>
//
// with the leading glyph in green|bold and the message body in
// green. Used by review-confirm transitions; not currently emitted
// during chargen substeps but provided for symmetry and for the
// follow-on screens (score, who) that will join this style.
func writeOK(s *telnet.Session, msg string) error {
	return s.WriteString(fmt.Sprintf(
		"{{✓ }}::green|bold {{%s}}::green\r\n",
		defangChargenField(msg)))
}

// defangChargenField scrubs cfmt injection and control bytes from a
// chargen string before it is spliced into a tagged template. Empty
// input passes through unchanged so format-string padding (e.g.
// "%-22s") doesn't end up with stray fallback text. Authored prose
// that is NOT spliced into a tag (e.g. background descriptions
// emitted via WriteWrapped) does not need to be defanged — those
// bodies flow through cfmt directly so builders can colour them.
//
// Delegates to internal/display.Defang to keep the policy single-
// sourced; the mode package wrapper exists so future chargen-only
// rules (e.g. trim leading whitespace) can land here without
// disturbing the world-rendering path.
func defangChargenField(in string) string {
	if in == "" {
		return ""
	}
	return display.Defang(in, "")
}

