package display

// Render helpers shared across screens that style with cfmt: chargen
// (internal/mode/chargen_render.go), the score sheet
// (internal/cmd/score.go), who/channels/news as those are migrated.
//
// Conventions match the existing chargen palette:
//
//	─── Title ─────  cyan|bold   section header rule
//	Subtitle        cyan|bold   group divider (no rule)
//	  label: value  yellow|bold label, plain value
//	─────────────   gray         quiet divider
//	>> msg          red|bold + red    error
//	✓  msg          green|bold + green confirmation
//
// Every helper passes label/value text through Defang first so a
// cfmt-meaningful token from the DB or YAML can't recolour downstream
// output. Authored prose that flows through cfmt directly (room body,
// background description) does NOT need defanging — those bodies use
// Session.WriteWrapped on the raw cfmt tags.

import (
	"fmt"
	"strings"

	"github.com/Jasrags/WheelMUD/telnet"
)

const (
	// RuleMinWidth is the floor for divider widths when Session.Width
	// is zero (test fixtures often skip NAWS) or extremely narrow.
	RuleMinWidth = 40
	// RuleMaxWidth caps divider growth on very wide terminals so the
	// header doesn't visually overpower body text.
	RuleMaxWidth = 78
)

// RuleWidth returns the divider width for the session, clamped to
// [RuleMinWidth, RuleMaxWidth]. A nil session falls back to the
// minimum.
func RuleWidth(s *telnet.Session) int {
	w := 0
	if s != nil {
		w = s.Width
	}
	if w <= 0 {
		w = RuleMinWidth
	}
	if w > RuleMaxWidth {
		w = RuleMaxWidth
	}
	return w
}

// SectionHeader emits a styled rule + title + trailing dash run sized
// to RuleWidth(s). Title is defanged before splicing into the cfmt
// tag.
//
//	─── <title> ──────────────────────────────
func SectionHeader(s *telnet.Session, title string) error {
	w := RuleWidth(s)
	leading := "─── "
	body := Defang(title, "")
	used := len(leading) + len(body) + 1
	trailing := ""
	if used < w {
		trailing = " " + strings.Repeat("─", w-used)
	}
	return s.WriteString(fmt.Sprintf(
		"{{%s%s%s}}::cyan|bold\r\n", leading, body, trailing))
}

// Subsection emits a leading CRLF + cyan|bold title with no rule. Use
// for groups inside a screen ("Identity", "Build", "Loadout" in the
// chargen review; "Abilities", "Vitals", "Wealth" on the score
// sheet).
func Subsection(s *telnet.Session, title string) error {
	return s.WriteString(fmt.Sprintf(
		"\r\n{{%s}}::cyan|bold\r\n", Defang(title, "")))
}

// FieldRow emits a "  label: value\r\n" row with label padded to
// gutter columns (or to len(label) if longer) and rendered yellow|
// bold; value renders plain. Both fields are defanged.
//
// gutter < 1 falls back to a default of 14 — the longest current
// chargen label ("Background skills:") fits without drift.
func FieldRow(s *telnet.Session, label, value string, gutter int) error {
	if gutter < 1 {
		gutter = 14
	}
	pad := gutter
	if len(label) > pad {
		pad = len(label)
	}
	return s.WriteString(fmt.Sprintf(
		"  {{%-*s}}::yellow|bold %s\r\n",
		pad, Defang(label, "")+":", Defang(value, "")))
}

// Rule emits a gray divider sized to RuleWidth(s). Used to close
// info blocks without restating the section title.
func Rule(s *telnet.Session) error {
	return s.WriteString(fmt.Sprintf(
		"{{%s}}::gray\r\n", strings.Repeat("─", RuleWidth(s))))
}

// Error renders a single error line:
//
//	>> <msg>
//
// with ">> " in red|bold and the body in red. Msg is defanged.
func Error(s *telnet.Session, msg string) error {
	return s.WriteString(fmt.Sprintf(
		"{{>> }}::red|bold {{%s}}::red\r\n", Defang(msg, "")))
}

// OK renders a single confirmation line:
//
//	✓ <msg>
//
// with the glyph in green|bold and the body in green. Msg is
// defanged.
func OK(s *telnet.Session, msg string) error {
	return s.WriteString(fmt.Sprintf(
		"{{✓ }}::green|bold {{%s}}::green\r\n", Defang(msg, "")))
}
