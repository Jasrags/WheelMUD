package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

const (
	// zoneMapDefaultDepth is bigger than mapDefaultDepth (3) because
	// builders inspecting a whole zone benefit from seeing more at
	// once. With sector glyphs the readability stays decent up to ~12.
	zoneMapDefaultDepth = 8
	// zoneMapMaxDepth caps the BFS so a zone with deep recursive
	// sub-zones can't blow the renderer's bounding box. Override per
	// invocation via `depth=N` (clamped down).
	zoneMapMaxDepth = 12
)

// NewZoneMap builds the §10 admin zonemap — a builder-facing zone
// visualization. Reuses the player map's BFS via mapOptions:
// respectNoMap=false (admins see through hidden zones),
// respectHidden=false (admins see hidden exits), and the seed zone's
// id as boundaryZoneID so off-zone neighbours render as `( X )` 1-hop
// boundary cells without recursion. Sector glyphs land inside the
// brackets; legend + footer carry counts, cross-zone exits, vertical
// exits, and flagged rooms.
//
// AuthAdmin gate enforced by Registry.Dispatch.
func NewZoneMap(rooms repo.RoomRepo, exits repo.ExitRepo, zones repo.ZoneRepo) *telnet.Command {
	return &telnet.Command{
		Name: "zonemap",
		Help: "Render an admin map of a zone with sector glyphs and seam markers",
		Long: "Usage: zonemap                 — current zone (seed = current room)\n" +
			"       zonemap <external_id>   — named zone (seed = starter or lowest-id room)\n" +
			"       zonemap depth=N         — current zone with depth override\n" +
			"       zonemap <external_id> depth=N\n\n" +
			"Renders the seed zone as a BFS minimap with sector-letter cells.\n" +
			"Off-zone neighbours render as `( X )` 1-hop boundary markers.\n" +
			"Footer: sector counts, cross-zone exits, vertical exits, flagged\n" +
			"rooms. See ROADMAP §zonemap for design notes.",
		Auth: telnet.AuthAdmin,
		Run: func(c *telnet.Context) error {
			zoneRef, depth, err := parseZoneMapArgs(c.Args)
			if err != nil {
				return c.Session.WriteString("{{" + defangCfmt(err.Error()) + "}}::yellow\r\n")
			}
			return runZoneMap(c, rooms, exits, zones, zoneRef, depth)
		},
	}
}

// parseZoneMapArgs walks Args looking for an optional positional
// zone external_id and an optional `depth=N` flag. Order is free;
// unknown flags are rejected.
func parseZoneMapArgs(args []string) (zoneRef string, depth int, err error) {
	depth = zoneMapDefaultDepth
	for _, a := range args {
		if eq := strings.Index(a, "="); eq >= 0 {
			key := strings.ToLower(a[:eq])
			val := a[eq+1:]
			if key != "depth" {
				return "", 0, fmt.Errorf("unknown flag %q (expected depth=N)", a)
			}
			n, perr := strconv.Atoi(val)
			if perr != nil || n < 1 {
				return "", 0, fmt.Errorf("depth must be a positive integer, got %q", val)
			}
			if n > zoneMapMaxDepth {
				n = zoneMapMaxDepth
			}
			depth = n
			continue
		}
		if zoneRef != "" {
			return "", 0, fmt.Errorf("unexpected extra arg %q", a)
		}
		zoneRef = a
	}
	return zoneRef, depth, nil
}

func runZoneMap(c *telnet.Context, rooms repo.RoomRepo, exits repo.ExitRepo,
	zones repo.ZoneRepo, zoneRef string, depth int) error {
	zone, seed, ok := resolveZoneAndSeed(c, rooms, zones, zoneRef)
	if !ok {
		// resolveZoneAndSeed wrote the user-facing error already.
		return nil
	}

	// Enrich pass: ListAll once and build an id→Room map so the BFS
	// result and the footer assembly can resolve sector + zone +
	// flags + external_id without per-cell round trips. Likewise
	// preload zone external_ids so cross-zone footer lines name
	// the target zone instead of a numeric id.
	all, err := rooms.ListAll(c.Ctx)
	if err != nil {
		return c.Session.WriteString("{{Could not list rooms right now.}}::red\r\n")
	}
	byID := make(map[int64]repo.Room, len(all))
	for _, r := range all {
		byID[r.ID] = r
	}
	allZones, err := zones.List(c.Ctx)
	if err != nil {
		return c.Session.WriteString("{{Could not list zones right now.}}::red\r\n")
	}
	zoneNames := make(map[int64]string, len(allZones))
	for _, z := range allZones {
		zoneNames[z.ID] = z.ExternalID
	}

	cells, edges, err := exploreMap(c.Ctx, rooms, exits, seed, depth, mapOptions{
		respectNoMap:   false,
		respectHidden:  false,
		boundaryZoneID: zone.ID,
	})
	if err != nil {
		return c.Session.WriteString("{{Could not draw the zonemap right now.}}::red\r\n")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "{{Zone:}}::cyan|bold {{%s}}::yellow {{— %s}}::gray   "+
		"(level %d-%d, builder: %s)\r\n",
		defangCfmt(zone.ExternalID), defangCfmt(zone.Name),
		zone.MinLevel, zone.MaxLevel, defangCfmt(zone.Builder))
	b.WriteString("\r\n")
	b.WriteString(drawGrid(cells, edges, [2]int{0, 0}, zoneMapGlyph, colorizeZoneMapRow))
	b.WriteString("\r\n")
	b.WriteString(zoneMapLegend())
	b.WriteString(zoneMapFooter(c.Ctx, zone.ID, cells, edges, byID, zoneNames, exits))
	return c.Session.WriteString(b.String())
}

// resolveZoneAndSeed picks the zone and the BFS seed from the args.
// Three flows:
//   - bare invocation: seed = session's current room; zone = its
//     ZoneID. Errors when CurrentRoomID is zero.
//   - explicit zoneRef: zone resolved by external_id; seed is the
//     starter (id=1) if it lives in that zone, else the lowest-id
//     room with that ZoneID, else error if the zone is empty.
//
// On error, writes a user-facing message directly to the session
// and returns ok=false so the caller can short-circuit without
// bubbling a non-nil error up to the dispatcher (which would
// double-render the failure). Uses an explicit ok bool rather than
// a zero-value Zone sentinel so a hypothetical zone with id=0
// can't be confused with an error path.
func resolveZoneAndSeed(c *telnet.Context, rooms repo.RoomRepo, zones repo.ZoneRepo,
	zoneRef string) (repo.Zone, repo.Room, bool) {
	if zoneRef == "" {
		if c.Session.CurrentRoomID == 0 {
			_ = c.Session.WriteString(
				"{{You are nowhere in particular — pass a zone external_id.}}::red\r\n")
			return repo.Zone{}, repo.Room{}, false
		}
		room, err := rooms.FindByID(c.Ctx, c.Session.CurrentRoomID)
		if err != nil {
			_ = c.Session.WriteString("{{Could not look up your current room.}}::red\r\n")
			return repo.Zone{}, repo.Room{}, false
		}
		zone, err := zones.GetByID(c.Ctx, room.ZoneID)
		if err != nil {
			_ = c.Session.WriteString("{{Your current room has no zone.}}::red\r\n")
			return repo.Zone{}, repo.Room{}, false
		}
		return zone, room, true
	}

	zone, err := zones.GetByExternalID(c.Ctx, zoneRef)
	if err != nil {
		if errors.Is(err, repo.ErrZoneNotFound) {
			_ = c.Session.WriteString(
				"{{No such zone:}}::red " + defangCfmt(zoneRef) + "\r\n")
			return repo.Zone{}, repo.Room{}, false
		}
		_ = c.Session.WriteString("{{Could not look up that zone right now.}}::red\r\n")
		return repo.Zone{}, repo.Room{}, false
	}

	seed, err := pickZoneSeed(c.Ctx, rooms, zone.ID)
	if err != nil {
		_ = c.Session.WriteString(
			"{{Zone }}::red " + defangCfmt(zoneRef) + " {{has no rooms.}}::red\r\n")
		return repo.Zone{}, repo.Room{}, false
	}
	return zone, seed, true
}

// pickZoneSeed returns the starter (id=1) when it sits in zoneID,
// else the lowest-id room belonging to the zone. Errors when the
// zone has no rooms.
func pickZoneSeed(ctx context.Context, rooms repo.RoomRepo, zoneID int64) (repo.Room, error) {
	starter, err := rooms.FindByID(ctx, repo.StarterRoomID)
	if err == nil && starter.ZoneID == zoneID {
		return starter, nil
	}
	all, err := rooms.ListAll(ctx)
	if err != nil {
		return repo.Room{}, err
	}
	// ListAll is id-sorted (per the contract added with auto-coords);
	// the first room matching zoneID is the lowest-id seed.
	for _, r := range all {
		if r.ZoneID == zoneID {
			return r, nil
		}
	}
	return repo.Room{}, errors.New("zone has no rooms")
}

// zoneMapGlyph picks the cell glyph for the admin renderer. In-zone
// rooms get `[X]` where X is the sector glyph; off-zone rooms get
// `(X)`. Vertical exits, NoMap rooms (when surfaced), and the seed
// itself fall back to the player-style markers so admins still
// recognise the existing language.
func zoneMapGlyph(c *cell, isCurrent bool) string {
	if isCurrent {
		return "[*]"
	}
	if c.unknown {
		return "[?]"
	}
	g := sectorGlyph(c.sector)
	if c.offZone {
		return "(" + g + ")"
	}
	return "[" + g + "]"
}

// sectorGlyph maps a sector to a single character. Choices favour
// readability + first-letter mnemonics; conflicts (forest/field both
// 'f') are resolved by upper/lower case + colour. The colour table
// in colorizeZoneMapRow keys off the same characters so this mapping
// and the colour table must stay in lock-step.
func sectorGlyph(s repo.Sector) string {
	switch s {
	case repo.SectorCity:
		return "C"
	case repo.SectorForest:
		return "F"
	case repo.SectorField:
		return "f"
	case repo.SectorHills:
		return "h"
	case repo.SectorMountain:
		return "M"
	case repo.SectorDesert:
		return "d"
	case repo.SectorWater:
		return "W"
	case repo.SectorUnderwater:
		return "U"
	case repo.SectorAir:
		return "A"
	case repo.SectorUnderground:
		return "u"
	case repo.SectorBlight:
		return "B"
	case repo.SectorWaste:
		return "w"
	case repo.SectorStedding:
		return "S"
	case repo.SectorSwamp:
		return "s"
	}
	return "?"
}

// colorizeZoneMapRow wraps each known cell form with a cfmt colour
// tag. Two bracket families (`[X]` in-zone, `(X)` off-zone) plus the
// player markers (`[*]`, `[?]`). Unique 3-rune forms required by the
// drawGrid invariant — every replacement target appears exactly once
// in any rendered cell, so ReplaceAll ordering is not load-bearing.
//
// Adding a new sector requires two updates in lock-step: extend
// sectorGlyph with the new mnemonic, and add a row to
// sectorColorTable so the glyph picks up its colour. A glyph that
// only lands in sectorGlyph (no colorTable row) renders unstyled —
// flagged on appearance during PR review rather than failing the
// boot.
func colorizeZoneMapRow(line string) string {
	// Player markers retain their player-map colours so admins keep
	// muscle memory across the two commands. `[?]` is reused from
	// the player command for NoMap rooms even though zonemap calls
	// exploreMap with respectNoMap=false; if a future caller flips
	// the option we still want NoMap cells to render the prior way.
	line = strings.ReplaceAll(line, "[*]", "{{[*]}}::yellow|bold")
	line = strings.ReplaceAll(line, "[?]", "{{[?]}}::gray")
	for _, m := range sectorColorTable {
		line = strings.ReplaceAll(line, "["+m.glyph+"]", "{{["+m.glyph+"]}}::"+m.color)
		line = strings.ReplaceAll(line, "("+m.glyph+")", "{{("+m.glyph+")}}::"+m.color)
	}
	return line
}

// sectorColorTable maps every supported sector glyph (single char)
// to its cfmt colour. Order doesn't matter for correctness because
// the drawGrid invariant guarantees every glyph form is unique.
var sectorColorTable = []struct {
	glyph string
	color string
}{
	{"C", "white|bold"},
	{"F", "green|bold"},
	{"f", "green"},
	{"h", "yellow"},
	{"M", "gray|bold"},
	{"d", "yellow|bold"},
	{"W", "blue"},
	{"U", "blue|bold"},
	{"A", "cyan"},
	{"u", "gray"},
	{"B", "red"},
	{"w", "yellow"},
	{"S", "green|bold"},
	{"s", "green"},
}

func zoneMapLegend() string {
	var b strings.Builder
	b.WriteString("  {{Legend:}}::cyan ")
	parts := []string{
		"{{[C]}}::white|bold city",
		"{{[F]}}::green|bold forest",
		"{{[f]}}::green field",
		"{{[h]}}::yellow hills",
		"{{[M]}}::gray|bold mountain",
		"{{[d]}}::yellow|bold desert",
		"{{[W]}}::blue water",
		"{{[U]}}::blue|bold underwater",
		"{{[A]}}::cyan air",
		"{{[u]}}::gray underground",
		"{{[B]}}::red blight",
		"{{[w]}}::yellow waste",
		"{{[S]}}::green|bold stedding",
		"{{[s]}}::green swamp",
	}
	b.WriteString(strings.Join(parts, " · "))
	b.WriteString("\r\n")
	b.WriteString("  {{[*]}}::yellow|bold = seed · {{( X )}}::gray = off-zone neighbour · " +
		"{{[?]}}::gray = nomap\r\n")
	return b.String()
}

// zoneMapFooter walks the BFS result and assembles the four footer
// sections. Sector counts cover only the BFS-reached set so the
// totals match what's drawn on the grid above; vertical exits and
// flagged rooms cover the whole zone via byID so builders see
// rooms that only have vertical or hidden exits even when those
// rooms aren't on the 2D slice.
func zoneMapFooter(ctx context.Context, zoneID int64,
	cells map[[2]int]*cell, edges []edge,
	byID map[int64]repo.Room, zoneNames map[int64]string,
	exits repo.ExitRepo) string {
	allZoneIDs := zoneRoomIDs(byID, zoneID)
	var b strings.Builder
	b.WriteString(zoneMapSectorCounts(zoneID, cells, byID))
	b.WriteString(zoneMapCrossZoneExits(ctx, cells, edges, byID, zoneNames, exits))
	b.WriteString(zoneMapVerticalExits(ctx, allZoneIDs, byID, exits))
	b.WriteString(zoneMapFlaggedRooms(allZoneIDs, byID))
	return b.String()
}

// zoneRoomIDs returns the sorted ids of every room belonging to
// zoneID. Used by the metadata footer sections (vertical exits,
// flagged rooms) so they describe the whole zone rather than just
// the BFS-reached slice.
func zoneRoomIDs(byID map[int64]repo.Room, zoneID int64) []int64 {
	ids := make([]int64, 0, len(byID))
	for _, r := range byID {
		if r.ZoneID == zoneID {
			ids = append(ids, r.ID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// zoneMapSectorCounts produces the "Sectors:" line summarising the
// sector mix of the BFS-reached in-zone cells. Limited to BFS-reached
// rooms on purpose so the line matches what's actually drawn on the
// grid; off-zone neighbours and unknown cells are excluded.
func zoneMapSectorCounts(zoneID int64, cells map[[2]int]*cell, byID map[int64]repo.Room) string {
	counts := make(map[repo.Sector]int)
	for _, c := range cells {
		if c.unknown || c.offZone {
			continue
		}
		room, ok := byID[c.roomID]
		if !ok || room.ZoneID != zoneID {
			continue
		}
		counts[room.Sector]++
	}
	if len(counts) == 0 {
		return ""
	}
	type bucket struct {
		sector repo.Sector
		count  int
	}
	buckets := make([]bucket, 0, len(counts))
	for s, n := range counts {
		buckets = append(buckets, bucket{s, n})
	}
	sort.Slice(buckets, func(i, j int) bool {
		// Descending count, then sector name for stability.
		if buckets[i].count != buckets[j].count {
			return buckets[i].count > buckets[j].count
		}
		return buckets[i].sector < buckets[j].sector
	})
	parts := make([]string, 0, len(buckets))
	for _, b := range buckets {
		parts = append(parts, fmt.Sprintf("%s: %d", b.sector, b.count))
	}
	return fmt.Sprintf("  {{Sectors:}}::cyan %s\r\n", strings.Join(parts, " · "))
}

// zoneMapCrossZoneExits emits one line per off-zone destination with
// the source room, direction, target room, and target zone external_id.
// Direction is re-fetched via directionBetween because the BFS edge
// struct doesn't carry it.
func zoneMapCrossZoneExits(ctx context.Context, cells map[[2]int]*cell, edges []edge,
	byID map[int64]repo.Room, zoneNames map[int64]string, exits repo.ExitRepo) string {
	type entry struct {
		fromExt, direction, toExt, toZone string
	}
	var rows []entry
	for _, eg := range edges {
		toCell, ok := cells[eg.to]
		if !ok || !toCell.offZone {
			continue
		}
		fromCell, ok := cells[eg.from]
		if !ok {
			continue
		}
		fromRoom := byID[fromCell.roomID]
		toRoom := byID[toCell.roomID]
		dir, ok := directionBetween(ctx, exits, fromRoom.ID, toRoom.ID)
		if !ok {
			dir = "?"
		}
		toZoneName, ok := zoneNames[toRoom.ZoneID]
		if !ok {
			toZoneName = fmt.Sprintf("zone#%d", toRoom.ZoneID)
		}
		rows = append(rows, entry{
			fromExt:   fromRoom.ExternalID,
			direction: dir,
			toExt:     toRoom.ExternalID,
			toZone:    toZoneName,
		})
	}
	if len(rows) == 0 {
		return ""
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].fromExt != rows[j].fromExt {
			return rows[i].fromExt < rows[j].fromExt
		}
		return rows[i].direction < rows[j].direction
	})
	var b strings.Builder
	b.WriteString("  {{Cross-zone exits:}}::cyan\r\n")
	for _, r := range rows {
		// Every interpolated value passes through defangCfmt — the
		// row sits inside cfmt-styled spans and a builder-controlled
		// string could otherwise close the span and inject markup.
		// direction comes from a fixed DB enum today; defanging is
		// the contract regardless.
		fmt.Fprintf(&b, "    %s  %s → %s {{[%s]}}::gray\r\n",
			defangCfmt(r.fromExt), defangCfmt(r.direction),
			defangCfmt(r.toExt), defangCfmt(r.toZone))
	}
	return b.String()
}

// zoneMapVerticalExits lists every u/d exit whose source is in the
// seed zone. Walked from byID rather than the BFS slice so a room
// reachable only via vertical exits (orphan towers, basements that
// don't connect to the cardinal grid) still surfaces here.
func zoneMapVerticalExits(ctx context.Context, allZoneIDs []int64,
	byID map[int64]repo.Room, exits repo.ExitRepo) string {
	type entry struct {
		fromExt, dir, toExt string
	}
	var rows []entry
	for _, id := range allZoneIDs {
		room := byID[id]
		outs, err := exits.ListFrom(ctx, id)
		if err != nil {
			continue
		}
		for _, e := range outs {
			if e.Direction != repo.DirUp && e.Direction != repo.DirDown {
				continue
			}
			target := byID[e.ToRoomID]
			rows = append(rows, entry{
				fromExt: room.ExternalID, dir: e.Direction, toExt: target.ExternalID,
			})
		}
	}
	if len(rows) == 0 {
		return ""
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].fromExt != rows[j].fromExt {
			return rows[i].fromExt < rows[j].fromExt
		}
		return rows[i].dir < rows[j].dir
	})
	var b strings.Builder
	b.WriteString("  {{Vertical exits:}}::cyan\r\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "    %s  %s → %s\r\n",
			defangCfmt(r.fromExt), defangCfmt(r.dir), defangCfmt(r.toExt))
	}
	return b.String()
}

// zoneMapFlaggedRooms groups the seed zone's rooms by flag, omitting
// empty categories so the footer stays terse on a fully-default zone.
// Walks byID rather than the BFS slice so flagged rooms surface even
// when they're only reachable via vertical or hidden exits.
func zoneMapFlaggedRooms(allZoneIDs []int64, byID map[int64]repo.Room) string {
	groups := []struct {
		name string
		test func(repo.RoomFlags) bool
	}{
		{"peaceful", func(f repo.RoomFlags) bool { return f.Peaceful }},
		{"dark", func(f repo.RoomFlags) bool { return f.Dark }},
		{"silent", func(f repo.RoomFlags) bool { return f.Silent }},
		{"nopvp", func(f repo.RoomFlags) bool { return f.NoPVP }},
		{"noteleport", func(f repo.RoomFlags) bool { return f.NoTeleport }},
		{"nomap", func(f repo.RoomFlags) bool { return f.NoMap }},
		{"indoors", func(f repo.RoomFlags) bool { return f.Indoors }},
	}
	var lines strings.Builder
	any := false
	for _, g := range groups {
		var ids []string
		for _, id := range allZoneIDs {
			room := byID[id]
			if g.test(room.Flags) {
				ids = append(ids, defangCfmt(room.ExternalID))
			}
		}
		if len(ids) == 0 {
			continue
		}
		any = true
		fmt.Fprintf(&lines, "    {{%s:}}::yellow %s\r\n", g.name, strings.Join(ids, ", "))
	}
	if !any {
		return ""
	}
	return "  {{Flagged rooms:}}::cyan\r\n" + lines.String()
}

// directionBetween finds the direction code on the exit from→to, if
// any. Returns ("", false) when no such exit exists. Used to label
// cross-zone footer lines.
func directionBetween(ctx context.Context, exits repo.ExitRepo, from, to int64) (string, bool) {
	outs, err := exits.ListFrom(ctx, from)
	if err != nil {
		return "", false
	}
	for _, e := range outs {
		if e.ToRoomID == to {
			return e.Direction, true
		}
	}
	return "", false
}

