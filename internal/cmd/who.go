package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewWho builds the who command. Lists every bound session with its
// character name and idle time, marks the caller "(you)". Class /
// level / title columns are deferred until char-create populates
// the underlying Character fields (ROADMAP §13).
//
// Rendering goes through internal/display so the section header
// matches the chargen review and score sheet — names are yellow|bold,
// idle suffixes muted gray, the "(you)" / wizinvis "*" markers cyan.
func NewWho(sessions *session.Registry, characters repo.CharacterRepo) *telnet.Command {
	return &telnet.Command{
		Name: "who",
		Help: "List connected players",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			snap := sessions.Snapshot()
			rows := collectWhoRows(c.Ctx, c.Session, snap, time.Now().UTC(), characters)
			sort.Slice(rows, func(i, j int) bool { return rows[i].sortKey < rows[j].sortKey })

			if err := display.SectionHeader(c.Session,
				fmt.Sprintf("Players online (%d)", len(rows))); err != nil {
				return err
			}
			var b strings.Builder
			for _, r := range rows {
				b.WriteString(formatWhoRow(r))
			}
			return c.Session.WritePagedWrapped(b.String())
		},
	}
}

// whoRow is the materialized presentation of one peer session: the
// pieces the row formatter splices into a cfmt-styled line. sortKey
// is held separately so styling tags don't perturb sort order.
type whoRow struct {
	name    string
	you     bool
	hidden  bool
	pvp     bool
	idle    string // "" when below the idle threshold
	sortKey string
}

// collectWhoRows runs the visibility + display rules over a session
// snapshot and returns a slice ready to format. Pulled out of the
// command body so the wizinvis + idle + marker logic is testable
// without spinning up a registry + dispatcher.
func collectWhoRows(ctx context.Context, viewer *telnet.Session, snap map[int64]*telnet.Session, now time.Time, characters repo.CharacterRepo) []whoRow {
	rows := make([]whoRow, 0, len(snap))
	viewerIsAdmin := viewer != nil && viewer.AuthLevel >= telnet.AuthAdmin
	for _, peer := range snap {
		// wizinvis: hide invisible peers from non-admin viewers.
		// The caller's own session is always visible to itself.
		if peer != viewer && peer.IsHidden() && !viewerIsAdmin {
			continue
		}
		charID, name, _ := peer.InWorld()
		if name == "" {
			// Pre-character session — login or character-select.
			// "(connecting)" rather than remote address so who can't
			// be used to enumerate IPs.
			name = "(connecting)"
		}
		r := whoRow{name: name, sortKey: strings.ToLower(name)}
		if d := peer.IdleSince(now); d >= 30*time.Second {
			r.idle = formatIdle(d)
		}
		if peer == viewer {
			r.you = true
		}
		// Admin-visible marker for hidden peers (including self when
		// self is wizinvis), so the operator can see who is invisible.
		if peer.IsHidden() && viewerIsAdmin {
			r.hidden = true
		}
		// PvP tag: read from the same source `attack` consults so the
		// flag is consistent. A lookup miss leaves the row tagless
		// rather than failing the whole command.
		if charID != 0 && characters != nil {
			ch, err := characters.GetByID(ctx, charID)
			if err != nil {
				slog.Debug("who: pvp lookup failed", "char", charID, "error", err)
			} else {
				r.pvp = ch.PvP
			}
		}
		rows = append(rows, r)
	}
	return rows
}

// formatWhoRow emits one player line with cfmt styling. Defang
// guards against a stored character name carrying cfmt syntax.
func formatWhoRow(r whoRow) string {
	var b strings.Builder
	b.WriteString("  {{")
	b.WriteString(display.Defang(r.name, "(unknown)"))
	b.WriteString("}}::yellow|bold")
	if r.pvp {
		b.WriteString(" {{[PvP]}}::red")
	}
	if r.you {
		b.WriteString(" {{(you)}}::cyan")
	}
	if r.hidden {
		b.WriteString(" {{*}}::cyan|bold")
	}
	if r.idle != "" {
		b.WriteString(" {{idle ")
		b.WriteString(r.idle)
		b.WriteString("}}::gray")
	}
	b.WriteString("\r\n")
	return b.String()
}

// formatIdle renders a duration as a short human-friendly string:
// "45s", "3m", "1h12m". Long enough idles round to whole hours.
func formatIdle(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	}
}
