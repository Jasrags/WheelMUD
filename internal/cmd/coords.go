package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewCoords builds the `coords` admin command — surface for the
// auto-coord BFS runner (internal/world/coords_derive). Subcommands:
//
//	coords                       — usage
//	coords rebuild               — run derivation now, report summary
//	coords show <id|external_id> — show a room's coords + anchor flag
//	coords issues                — list conflicts + orphans from a fresh
//	                               derivation pass (idempotent;
//	                               persists any newly-correct coords)
//
// AuthAdmin: builders use it to investigate coord drift after a YAML
// edit; players don't need it. The derivation runner is itself
// idempotent so `coords issues` doubles as a "pulse check" without
// changing persisted state on a stable world.
func NewCoords(rooms repo.RoomRepo, exits repo.ExitRepo) *telnet.Command {
	return &telnet.Command{
		Name: "coords",
		Help: "Inspect and rebuild auto-derived room coordinates (admin)",
		Long: "Usage: coords                          — show usage\n" +
			"       coords rebuild                  — run BFS derivation now\n" +
			"       coords show <id|external_id>    — show one room's coords + anchor flag\n" +
			"       coords issues                   — list conflicts + orphans\n\n" +
			"The derivation pass walks from every CoordsAnchor room (or the\n" +
			"starter as a synthetic anchor) and assigns coords to non-anchor\n" +
			"rooms by summing direction deltas along the BFS frontier.\n" +
			"Anchors are never overwritten; first-arrival wins on conflicts.\n" +
			"See ROADMAP §Auto-derived room coordinates and migration 0026.",
		Auth: telnet.AuthAdmin,
		Run: func(c *telnet.Context) error {
			if len(c.Args) == 0 {
				return c.Session.WriteString(coordsUsage())
			}
			switch strings.ToLower(c.Args[0]) {
			case "rebuild":
				return runCoordsRebuild(c, rooms, exits)
			case "show":
				if len(c.Args) < 2 {
					return c.Session.WriteString("{{Usage: coords show <id|external_id>}}::yellow\r\n")
				}
				return runCoordsShow(c, rooms, c.Args[1])
			case "issues":
				return runCoordsIssues(c, rooms, exits)
			default:
				// Bare-id form: `coords emonds_field.green` aliases to
				// `coords show emonds_field.green`. Keeps it ergonomic
				// when an admin already typed the room reference.
				return runCoordsShow(c, rooms, c.Args[0])
			}
		},
	}
}

func coordsUsage() string {
	return "{{Usage:}}::cyan|bold\r\n" +
		"  {{coords rebuild}}::yellow                run BFS derivation now\r\n" +
		"  {{coords show <id|external_id>}}::yellow show one room's coords + anchor\r\n" +
		"  {{coords issues}}::yellow                list conflicts + orphans\r\n"
}

func runCoordsRebuild(c *telnet.Context, rooms repo.RoomRepo, exits repo.ExitRepo) error {
	summary, err := world.DeriveCoords(c.Ctx, rooms, exits)
	if err != nil {
		return c.Session.WriteString("{{Coord rebuild failed:}}::red " + defangCfmt(err.Error()) + "\r\n")
	}
	return c.Session.WriteString(formatCoordSummary(summary))
}

func runCoordsIssues(c *telnet.Context, rooms repo.RoomRepo, exits repo.ExitRepo) error {
	summary, err := world.DeriveCoords(c.Ctx, rooms, exits)
	if err != nil {
		return c.Session.WriteString("{{Coord scan failed:}}::red " + defangCfmt(err.Error()) + "\r\n")
	}
	if len(summary.Conflicts) == 0 && len(summary.Orphans) == 0 {
		return c.Session.WriteString("{{No coord issues.}}::green\r\n")
	}
	var b strings.Builder
	if len(summary.Conflicts) > 0 {
		b.WriteString("{{Conflicts}}::red|bold {{(BFS reached these rooms via paths with disagreeing coords; first-arrival kept):}}::gray\r\n")
		writeRoomIDList(&b, c.Ctx, rooms, summary.Conflicts)
	}
	if len(summary.Orphans) > 0 {
		b.WriteString("{{Orphans}}::yellow|bold {{(unreachable from any anchor; coords left unchanged):}}::gray\r\n")
		writeRoomIDList(&b, c.Ctx, rooms, summary.Orphans)
	}
	return c.Session.WriteString(b.String())
}

// writeRoomIDList prints sorted room ids with their external_id when
// resolvable. One line per id, padded so admins can scan a column.
// Lookup failures fall back to the bare id rather than aborting the
// list — a missing-row miss inside an issues report is itself
// information.
func writeRoomIDList(b *strings.Builder, ctx context.Context, rooms repo.RoomRepo, ids []int64) {
	for _, id := range ids {
		room, err := rooms.FindByID(ctx, id)
		if err != nil {
			fmt.Fprintf(b, "  {{%-6d}}::yellow {{(lookup failed)}}::gray\r\n", id)
			continue
		}
		fmt.Fprintf(b, "  {{%-6d}}::yellow %s {{at (%d,%d,%d)}}::gray\r\n",
			id, defangCfmt(room.ExternalID), room.CoordX, room.CoordY, room.CoordZ)
	}
}

func runCoordsShow(c *telnet.Context, rooms repo.RoomRepo, ref string) error {
	room, ok := resolveRoomRef(c, rooms, ref)
	if !ok {
		return nil // user-facing miss already surfaced by resolveRoomRef
	}
	anchor := "auto-derived"
	color := "yellow"
	if room.CoordsAnchor {
		anchor = "anchor (authored)"
		color = "cyan|bold"
	}
	out := fmt.Sprintf("{{Room %d}}::cyan|bold {{(%s)}}::gray\r\n", room.ID, defangCfmt(room.ExternalID)) +
		fmt.Sprintf("  {{Name:}}::yellow %s\r\n", defangCfmt(room.Name)) +
		fmt.Sprintf("  {{Coords:}}::yellow (%d, %d, %d)\r\n", room.CoordX, room.CoordY, room.CoordZ) +
		fmt.Sprintf("  {{Provenance:}}::yellow {{%s}}::%s\r\n", anchor, color)
	return c.Session.WriteString(out)
}

// resolveRoomRef accepts either a numeric room id or an external_id
// (the YAML-authored stable string) and returns the matching Room.
// On miss / lookup failure it writes a user-facing error directly to
// the session and returns ok=false so the caller can short-circuit
// without bubbling a non-nil error up to the dispatcher (which would
// double-render the failure).
func resolveRoomRef(c *telnet.Context, rooms repo.RoomRepo, ref string) (repo.Room, bool) {
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		room, lookupErr := rooms.FindByID(c.Ctx, id)
		if lookupErr == nil {
			return room, true
		}
		if !errors.Is(lookupErr, repo.ErrRoomNotFound) {
			_ = c.Session.WriteString("{{Could not look up that room right now.}}::red\r\n")
			return repo.Room{}, false
		}
		// Fall through: numeric form may also be a legitimate
		// external_id (e.g. "0001"); try the string lookup before
		// declaring a miss.
	}
	room, err := rooms.FindByExternalID(c.Ctx, ref)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			_ = c.Session.WriteString("{{No such room:}}::red " + defangCfmt(ref) + "\r\n")
			return repo.Room{}, false
		}
		_ = c.Session.WriteString("{{Could not look up that room right now.}}::red\r\n")
		return repo.Room{}, false
	}
	return room, true
}

func formatCoordSummary(s world.CoordSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "{{Coord derivation:}}::cyan|bold\r\n")
	if s.SyntheticAnchor {
		fmt.Fprintf(&b, "  {{Anchors:}}::yellow 0 (using starter as synthetic anchor)\r\n")
	} else {
		fmt.Fprintf(&b, "  {{Anchors:}}::yellow %d\r\n", s.Anchors)
	}
	fmt.Fprintf(&b, "  {{Assigned:}}::yellow %d\r\n", s.Assigned)
	fmt.Fprintf(&b, "  {{Conflicts:}}::yellow %d\r\n", len(s.Conflicts))
	fmt.Fprintf(&b, "  {{Orphans:}}::yellow %d\r\n", len(s.Orphans))
	if len(s.Conflicts) == 0 && len(s.Orphans) == 0 {
		b.WriteString("  {{World is clean.}}::green\r\n")
		return b.String()
	}
	b.WriteString("  {{Run `coords issues` for the offending room ids.}}::gray\r\n")
	return b.String()
}

