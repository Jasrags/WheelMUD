package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/emote"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewSocialsList builds the `socials` verb — a player-facing dump
// of every social in the §M.3 catalog grouped by whether the
// social accepts a target. AuthPlayer; no args, no side effects.
// Magenta verb names match the broadcast colour used by NewSocials
// / NewEmote so the listing reads as the same family of verbs.
//
// A nil or empty catalog renders as "No socials configured." rather
// than crashing — mirrors emote.Catalog.All()'s nil-safety.
func NewSocialsList(cat *emote.Catalog) *telnet.Command {
	return &telnet.Command{
		Name: "socials",
		Help: "List the available social verbs (smile, wave, ...)",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			return runSocialsList(cat, c.Session)
		},
	}
}

func runSocialsList(cat *emote.Catalog, s *telnet.Session) error {
	all := cat.All()
	if len(all) == 0 {
		return s.WriteString("{{No socials configured.}}::yellow\r\n")
	}

	var targeted, untargeted []emote.Social
	for _, sc := range all {
		if sc.Targetable() {
			targeted = append(targeted, sc)
		} else {
			untargeted = append(untargeted, sc)
		}
	}
	sort.Slice(targeted, func(i, j int) bool { return targeted[i].ID < targeted[j].ID })
	sort.Slice(untargeted, func(i, j int) bool { return untargeted[i].ID < untargeted[j].ID })

	if err := display.SectionHeader(s, "Socials"); err != nil {
		return err
	}

	if len(targeted) > 0 {
		if err := display.Subsection(s, "Targetable (greet someone by name)"); err != nil {
			return err
		}
		var b strings.Builder
		writeSocialRows(&b, targeted, socialNameWidth(targeted))
		if err := s.WriteString(b.String()); err != nil {
			return err
		}
	}
	if len(untargeted) > 0 {
		if err := display.Subsection(s, "Untargeted-only"); err != nil {
			return err
		}
		var b strings.Builder
		writeSocialRows(&b, untargeted, socialNameWidth(untargeted))
		if err := s.WriteString(b.String()); err != nil {
			return err
		}
	}
	return s.WriteString("{{Try }}::gray{{help <verb>}}::cyan{{ for the full text of a single social.}}::gray\r\n")
}

// writeSocialRows renders one social per line: magenta verb name
// padded to `width` runes, two spaces, then the help text. Help
// strings are operator-authored YAML — defang via display.Defang
// so a hostile entry can't close the surrounding magenta tag or
// inject a competing style. ID padding is byte-based; social IDs
// match [a-z][a-z0-9_]* so byte length == rune length.
//
// Padding MUST live inside the `{{...}}::magenta` cfmt wrapper —
// putting it after `::magenta` causes the terminal reset sequence
// to land mid-cell, which breaks column alignment in clients that
// honour SGR boundaries per glyph (most do).
func writeSocialRows(b *strings.Builder, list []emote.Social, width int) {
	for _, sc := range list {
		help := display.Defang(sc.Help, "")
		fmt.Fprintf(b, "  {{%-*s}}::magenta  %s\r\n", width, sc.ID, help)
	}
}

func socialNameWidth(list []emote.Social) int {
	w := 0
	for _, sc := range list {
		if len(sc.ID) > w {
			w = len(sc.ID)
		}
	}
	return w
}
