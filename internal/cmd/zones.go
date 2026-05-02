package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewZones builds the `zones` admin command:
//
//	zones                  — list every persisted zone
//	zones list             — same
//	zones show <id>        — detail view for one zone
//
// Read-only in this slice; admin edit (zedit) lands with §16. Gated
// at AuthAdmin: builders need to see authorship + reset cadence to
// debug area-reset behavior, players don't.
func NewZones(zones repo.ZoneRepo, rooms repo.RoomRepo) *telnet.Command {
	return &telnet.Command{
		Name: "zones",
		Help: "List zones, or show details for one (admin)",
		Long: "Usage: zones                — list every zone\n" +
			"       zones list           — same\n" +
			"       zones show <id>      — show one zone's metadata + room count\n\n" +
			"Reads from the zones table populated by the YAML world loader.\n" +
			"Edit via §16 builder tools (not yet implemented).",
		Auth: telnet.AuthAdmin,
		Run: func(c *telnet.Context) error {
			if len(c.Args) == 0 || strings.EqualFold(c.Args[0], "list") {
				return runZonesList(c, zones)
			}
			if strings.EqualFold(c.Args[0], "show") {
				if len(c.Args) < 2 {
					return c.Session.WriteString("{{Usage: zones show <external_id>}}::yellow\r\n")
				}
				return runZonesShow(c, zones, rooms, c.Args[1])
			}
			// Bare-id form: `zones emonds_field` is a friendlier alias
			// for `zones show emonds_field`.
			return runZonesShow(c, zones, rooms, c.Args[0])
		},
	}
}

func runZonesList(c *telnet.Context, zones repo.ZoneRepo) error {
	all, err := zones.List(c.Ctx)
	if err != nil {
		return c.Session.WriteString("{{Could not list zones right now.}}::red\r\n")
	}
	if len(all) == 0 {
		return c.Session.WriteString("{{No zones loaded.}}::yellow\r\n")
	}
	var b strings.Builder
	fmt.Fprintf(&b,
		"{{%-3s  %-32s  %-30s  %-7s  %-6s  %-7s  %s}}::cyan|bold\r\n",
		"ID", "EXTERNAL_ID", "NAME", "LVL", "RESET", "MODE", "BUILDER")
	for _, z := range all {
		fmt.Fprintf(&b,
			"%-3d  %-32s  %-30s  %-7s  %-6s  %-7s  %s\r\n",
			z.ID,
			truncate(z.ExternalID, 32),
			truncate(z.Name, 30),
			fmt.Sprintf("%d-%d", z.MinLevel, z.MaxLevel),
			fmt.Sprintf("%ds", z.ResetIntervalS),
			string(z.ResetMode),
			z.Builder,
		)
	}
	return c.Session.WriteString(b.String())
}

func runZonesShow(c *telnet.Context, zones repo.ZoneRepo, rooms repo.RoomRepo, externalID string) error {
	z, err := zones.GetByExternalID(c.Ctx, externalID)
	if err != nil {
		if errors.Is(err, repo.ErrZoneNotFound) {
			// Render the user-supplied id outside the cfmt markup so a
			// caller-controlled value can't close the {{...}} tag and
			// inject an arbitrary style sequence.
			return c.Session.WriteString("{{No such zone:}}::red " + defangCfmt(externalID) + "\r\n")
		}
		return c.Session.WriteString("{{Could not look up that zone right now.}}::red\r\n")
	}
	roomCount, err := rooms.CountByZone(c.Ctx, z.ID)
	if err != nil {
		// Don't fail the whole command — show the zone, note the gap.
		roomCount = -1
	}

	var b strings.Builder
	fmt.Fprintf(&b, "{{Zone:}}::cyan|bold     %s {{(#%d)}}::gray\r\n", z.ExternalID, z.ID)
	fmt.Fprintf(&b, "  {{Name:}}::yellow     %s\r\n", z.Name)
	if z.Builder != "" {
		fmt.Fprintf(&b, "  {{Builder:}}::yellow  %s\r\n", z.Builder)
	}
	fmt.Fprintf(&b, "  {{Levels:}}::yellow   %d-%d\r\n", z.MinLevel, z.MaxLevel)
	fmt.Fprintf(&b, "  {{Reset:}}::yellow    every %ds, mode=%s\r\n", z.ResetIntervalS, z.ResetMode)
	if z.Climate != "" {
		fmt.Fprintf(&b, "  {{Climate:}}::yellow  %s\r\n", z.Climate)
	}
	if roomCount >= 0 {
		fmt.Fprintf(&b, "  {{Rooms:}}::yellow    %d\r\n", roomCount)
	} else {
		fmt.Fprintf(&b, "  {{Rooms:}}::yellow    (count unavailable)\r\n")
	}
	if len(z.Ambient) > 0 {
		fmt.Fprintf(&b, "  {{Ambient:}}::yellow\r\n")
		for _, line := range z.Ambient {
			fmt.Fprintf(&b, "    - %s\r\n", line)
		}
	}
	return c.Session.WriteString(b.String())
}

// truncate trims s to n runes, appending an ellipsis if it had to
// cut. Rune-aware so authored zone names containing non-ASCII glyphs
// (typographic apostrophes, em-dashes) don't truncate mid-codepoint.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// defangCfmt neutralizes any cfmt control sequences in user-supplied
// text by breaking the `{{` and `}}::style` tokens. Used when echoing
// caller-controlled strings (e.g. an admin's `zones show <id>` arg)
// inside a styled error line so a hostile value can't inject markup.
func defangCfmt(s string) string {
	s = strings.ReplaceAll(s, "{{", "{ {")
	s = strings.ReplaceAll(s, "}}", "} }")
	return s
}
