package mode

// Chargen render helpers. The shared cross-screen primitives —
// rules, label rows, error/OK lines, defang — now live in
// internal/display and the wrappers in this file delegate to them
// (every cmd/*.go and account_menu_*.go caller already imports the
// display package, so the third-caller bar from the original
// chargen_render comment is met).
//
// What stays here is chargen-specific:
//
//   - writeStepHeader renders the "Step 3/8 — Background" banner
//     keyed off the chargenStep enum
//   - chargenStepLabel / chargenStepNumber map enum values to the
//     player-facing strings used by writeStepHeader
//   - chargenLabelGutter is the gutter the wrapped writeFieldRow
//     passes through to display.FieldRow so chargen rows align
//     visually across substeps

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
	case chargenStepEquipment:
		return "Starting equipment"
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
	case chargenStepEquipment:
		return 7 // shares slot 7 ("Build") — picked alongside feats/skills/channeling
	case chargenStepReview:
		return 8
	}
	return 0
}

// writeStepHeader emits an opening rule for the substep:
//
//	─── Step 3/8 — Background ──────────────────────────────
//
// The leading "─── " and trailing "─" run are coloured cyan|bold to
// match the look.go room-title palette. The width comes from
// display.RuleWidth so this helper stays in lock-step with
// display.SectionHeader rendering.
func writeStepHeader(s *telnet.Session, step chargenStep) error {
	num := chargenStepNumber(step)
	label := chargenStepLabel(step)
	if num == 0 || label == "" {
		return nil
	}
	w := display.RuleWidth(s)
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

// writeRule, writeFieldRow, writeError, writeOK delegate to the
// internal/display equivalents so chargen substeps and the rest of
// the codebase share one rendering path.
func writeRule(s *telnet.Session) error               { return display.Rule(s) }
func writeError(s *telnet.Session, msg string) error  { return display.Error(s, msg) }
func writeOK(s *telnet.Session, msg string) error     { return display.OK(s, msg) }

// writeFieldRow renders a chargen-styled label/value row. The gutter
// is held constant so chargen sub-screens align visually even when
// the longest label varies between screens.
func writeFieldRow(s *telnet.Session, label, value string) error {
	return display.FieldRow(s, label, value, chargenLabelGutter)
}

// defangChargenField scrubs cfmt injection and control bytes from a
// chargen string before it is spliced into a tagged template. Empty
// input passes through unchanged so format-string padding (e.g.
// "%-22s") doesn't end up with stray fallback text.
func defangChargenField(in string) string {
	if in == "" {
		return ""
	}
	return display.Defang(in, "")
}

