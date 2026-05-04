package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

const (
	mapDefaultDepth = 3
	mapMinDepth     = 1
	mapMaxDepth     = 5
)

// dirVec maps each direction code to its (x, y, z) unit step on the
// minimap grid. y grows southward (n decreases y) so the rendered grid
// reads "north is up" without a separate flip. z is informational only
// — vertical exits stay inside the current cell and decorate its glyph.
var dirVec = map[string][3]int{
	repo.DirNorth:     {0, -1, 0},
	repo.DirSouth:     {0, 1, 0},
	repo.DirEast:      {1, 0, 0},
	repo.DirWest:      {-1, 0, 0},
	repo.DirNortheast: {1, -1, 0},
	repo.DirNorthwest: {-1, -1, 0},
	repo.DirSoutheast: {1, 1, 0},
	repo.DirSouthwest: {-1, 1, 0},
	repo.DirUp:        {0, 0, 1},
	repo.DirDown:      {0, 0, -1},
}

// cell describes one occupied minimap square keyed by (x,y).
type cell struct {
	roomID  int64
	hasUp   bool
	hasDown bool
	// unknown rooms (NoMap destinations under a NoMap-respecting BFS)
	// render as `[?]` and the BFS does not recurse through them.
	unknown bool
	// offZone is set when exploreMap was given a non-zero
	// boundaryZoneID and the room sits in a different zone. The BFS
	// records the cell as a 1-hop boundary marker (rendered `( X )`)
	// and does not recurse through it. Player `map` calls leave the
	// boundary unset, so this is always false on that path.
	offZone bool
	// sector carries the destination room's sector so the renderer
	// can tint cells by sector without re-fetching the room. Both
	// callers (player + zonemap) populate this — the player path
	// uses it for color, zonemap uses it for both glyph letter and
	// color.
	sector repo.Sector
}

// mapOptions modulates the BFS for player vs. admin callers. The
// player command supplies the strict defaults (respectNoMap +
// respectHidden, no zone boundary); zonemap relaxes the gates and
// passes its zone id so the BFS can flag off-zone neighbours without
// recursing through them.
type mapOptions struct {
	// respectNoMap=true preserves the player-map behaviour: NoMap
	// rooms render `[?]` and the BFS stops at them. zonemap sets
	// false so admins see the actual sector glyph.
	respectNoMap bool
	// respectHidden=true skips hidden exits during traversal (player
	// behaviour). zonemap sets false so admins see hidden doors.
	respectHidden bool
	// boundaryZoneID, when non-zero, marks any room outside this zone
	// as offZone in the cells map and stops recursion at it. Zero
	// means "no zone boundary" — the player-map default.
	boundaryZoneID int64
}

// playerMapOptions returns the strict-defaults options used by the
// player `map` command. Centralised so the player call site reads
// declaratively and a future tweak doesn't drift between callers.
func playerMapOptions() mapOptions {
	return mapOptions{respectNoMap: true, respectHidden: true}
}

// edge connects two grid coords; recorded once per exit so the renderer
// can draw a connector char only between rooms that actually have an
// exit between them — never between rooms that merely happen to be
// grid-adjacent.
type edge struct {
	from [2]int
	to   [2]int
}

// NewMap builds the §10 BFS minimap command. With no args it renders a
// depth-3 map; `map <n>` clamps to [1, 5]. Auth: AuthPlayer. Hidden
// exits are skipped (mirrors look). NoMap rooms render `[?]` at the
// boundary and stop traversal. Cells are tinted by sector via the
// shared zonemap palette; the zone's display name (not its external
// id) is printed above the grid when available.
func NewMap(rooms repo.RoomRepo, exits repo.ExitRepo, zones repo.ZoneRepo) *telnet.Command {
	return &telnet.Command{
		Name: "map",
		Help: "Show a small map of the rooms around you (default depth 3, max 5)",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			depth := mapDefaultDepth
			if len(c.Args) > 0 {
				n, err := strconv.Atoi(c.Args[0])
				if err != nil {
					return c.Session.WriteString("{{Usage: map [depth]   (depth must be a number 1..5)}}::red\r\n")
				}
				if n < mapMinDepth {
					n = mapMinDepth
				}
				if n > mapMaxDepth {
					n = mapMaxDepth
				}
				depth = n
			}
			return renderMap(c.Ctx, c.Session, rooms, exits, zones, depth)
		},
	}
}

func renderMap(ctx context.Context, s *telnet.Session, rooms repo.RoomRepo, exits repo.ExitRepo, zones repo.ZoneRepo, depth int) error {
	if s.CurrentRoomID == 0 {
		return s.WriteString("{{You are nowhere in particular.}}::red\r\n")
	}

	cur, err := rooms.FindByID(ctx, s.CurrentRoomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return s.WriteString("{{The room around you has gone missing. Tell an admin.}}::red\r\n")
		}
		return s.WriteString("{{Could not draw the map right now.}}::red\r\n")
	}

	cells, edges, err := exploreMap(ctx, rooms, exits, cur, depth, playerMapOptions())
	if err != nil {
		return s.WriteString("{{Could not draw the map right now.}}::red\r\n")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "{{Map (depth %d):}}::cyan|bold\r\n", depth)
	if name := lookupZoneName(ctx, zones, cur.ZoneID); name != "" {
		// Zone.Name is operator-authored YAML / DB content; defang
		// before splicing into a cfmt template so a stray `}}::style`
		// in a zone name can't close the yellow tag and inject styling.
		// Same policy as zonemap.go's header.
		fmt.Fprintf(&b, "{{Zone:}}::cyan|bold {{%s}}::yellow\r\n", defangCfmt(name))
	}
	b.WriteString(drawGrid(cells, edges, [2]int{0, 0}, playerCellWrap))
	b.WriteString(sectorLegendLine())
	// Cell-semantics line uses the bracketed forms so the player's
	// muscle-memory glyphs ([*], [?], [^]/[v]/[%]) carry through. The
	// `[*]` swatch is yellow|bold to match the on-grid seed cell;
	// `[?]` is gray to match NoMap; vertical-link glyphs render
	// neutrally at yellow since they're decorations, not sectors.
	b.WriteString("  {{[*]}}::yellow|bold = you · {{[?]}}::gray = unmapped · " +
		"{{[^]}}::yellow/{{[v]}}::yellow/{{[%]}}::yellow = up/down/both\r\n")
	return s.WriteString(b.String())
}

// lookupZoneName resolves zoneID to the human-readable Zone.Name.
// Returns "" on ZoneID==0, ErrZoneNotFound, or any other error so the
// renderMap caller can omit the line silently for legacy rooms. The
// external id is intentionally NOT surfaced — the player command
// shows the display name only.
func lookupZoneName(ctx context.Context, zones repo.ZoneRepo, zoneID int64) string {
	if zones == nil || zoneID == 0 {
		return ""
	}
	z, err := zones.GetByID(ctx, zoneID)
	if err != nil {
		if !errors.Is(err, repo.ErrZoneNotFound) {
			slog.Debug("map: zone lookup failed", "zone", zoneID, "error", err)
		}
		return ""
	}
	return z.Name
}

// exploreMap walks the exit graph from start out to depth steps,
// assigning every reachable room a (x, y) grid coordinate. First-visit
// wins for both already-seen rooms and grid conflicts; conflicts are
// logged at slog.Debug. Hidden exits are skipped. NoMap destinations
// occupy the boundary cell as `[?]` and are not recursed through.
//
// Returned edges record the (from, to) grid coords of every non-vertical
// exit whose destination is also placed on the grid; the renderer uses
// edges (not grid adjacency) to decide where to draw connectors.
func exploreMap(ctx context.Context, rooms repo.RoomRepo, exits repo.ExitRepo, start repo.Room, depth int, opts mapOptions) (map[[2]int]*cell, []edge, error) {
	cells := make(map[[2]int]*cell)
	visited := make(map[int64][2]int)
	var edges []edge
	type qItem struct {
		roomID int64
		coord  [2]int
		d      int
	}
	origin := [2]int{0, 0}
	queue := []qItem{{start.ID, origin, 0}}
	visited[start.ID] = origin
	cells[origin] = &cell{roomID: start.ID, sector: start.Sector}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		outs, err := exits.ListFrom(ctx, item.roomID)
		if err != nil {
			return nil, nil, err
		}
		for _, e := range outs {
			if opts.respectHidden && e.Flags.Hidden {
				continue
			}
			vec, ok := dirVec[e.Direction]
			if !ok {
				continue
			}
			// Vertical-only exits stay in the current cell as glyph hints.
			if vec[0] == 0 && vec[1] == 0 {
				if c := cells[item.coord]; c != nil {
					if vec[2] > 0 {
						c.hasUp = true
					} else if vec[2] < 0 {
						c.hasDown = true
					}
				}
				continue
			}

			nextCoord := [2]int{item.coord[0] + vec[0], item.coord[1] + vec[1]}

			// Already-visited destination: just record the edge so the
			// connector renders. The destination's actual coord wins
			// (first-visit), which may differ from nextCoord — long
			// edges are silently dropped by the renderer's adjacency
			// check.
			if existingCoord, seen := visited[e.ToRoomID]; seen {
				// First-visit-wins: if the existing coord is not
				// grid-adjacent to item.coord, the renderer will drop
				// this edge silently. Log so a builder examining the
				// debug stream can spot a topology that's impossible
				// to render straight (cycles where N+E+S+W ≠ origin).
				dx := existingCoord[0] - item.coord[0]
				dy := existingCoord[1] - item.coord[1]
				if dx < -1 || dx > 1 || dy < -1 || dy > 1 {
					slog.Debug("map: long edge dropped (cycle resolved to distant first-visit coord)",
						"from", item.coord, "to", existingCoord,
						"room", e.ToRoomID, "direction", e.Direction)
				}
				edges = append(edges, edge{item.coord, existingCoord})
				continue
			}

			// Beyond depth: don't add new cells. No connector either,
			// since the destination won't appear on the grid.
			if item.d >= depth {
				continue
			}

			// Grid conflict: a different room already occupies the
			// target coord. Skip both cell and edge to avoid pointing
			// at the wrong room.
			if existing, occupied := cells[nextCoord]; occupied {
				slog.Debug("map: grid conflict, skipping later visit",
					"existing_room", existing.roomID,
					"skipped_room", e.ToRoomID,
					"coord", nextCoord)
				continue
			}

			dest, err := rooms.FindByID(ctx, e.ToRoomID)
			if err != nil {
				if errors.Is(err, repo.ErrRoomNotFound) {
					continue
				}
				return nil, nil, err
			}

			if opts.respectNoMap && dest.Flags.NoMap {
				cells[nextCoord] = &cell{roomID: dest.ID, unknown: true}
				visited[dest.ID] = nextCoord
				edges = append(edges, edge{item.coord, nextCoord})
				continue
			}

			// zonemap path: a room in a different zone gets recorded
			// as an off-zone boundary cell, the edge is recorded so
			// the renderer can draw the connector to it, but the BFS
			// does not enqueue it for further traversal. Off-zone
			// adjacency to the seed zone is the only depth at which
			// off-zone rooms appear.
			if opts.boundaryZoneID != 0 && dest.ZoneID != opts.boundaryZoneID {
				cells[nextCoord] = &cell{
					roomID:  dest.ID,
					offZone: true,
					sector:  dest.Sector,
				}
				visited[dest.ID] = nextCoord
				edges = append(edges, edge{item.coord, nextCoord})
				continue
			}

			cells[nextCoord] = &cell{roomID: dest.ID, sector: dest.Sector}
			visited[dest.ID] = nextCoord
			edges = append(edges, edge{item.coord, nextCoord})
			queue = append(queue, qItem{dest.ID, nextCoord, item.d + 1})
		}
	}
	return cells, edges, nil
}

// drawGrid lays cells onto a fixed-pitch character grid. Each cell is a
// 3-char glyph chosen by the caller-supplied wrapCell; cells are
// separated by one connector column horizontally and one connector row
// vertically. Cell (x, y) lands at column (x-minX)*4 (..+2) and row
// (y-minY)*2.
//
// wrapCell returns two strings per cell: the bare 3-rune glyph (used
// for layout / column accounting) and the on-the-wire form (typically
// the same glyph wrapped in cfmt color tags, expanded by the session
// writer to ANSI). The bare glyph keeps the rune buffer aligned for
// connector drawing; the wrapped form is overlaid at row-emit time so
// cfmt tag bytes don't disturb visual column math.
//
// Connectors are drawn from the edges slice — never from grid adjacency
// — so two rooms that are visually next to each other but lack an exit
// between them stay disconnected.
func drawGrid(cells map[[2]int]*cell, edges []edge, current [2]int,
	wrapCell func(c *cell, isCurrent bool) (glyph, wrapped string),
) string {
	if len(cells) == 0 {
		return "  (empty)\r\n"
	}
	minX, maxX := math.MaxInt, math.MinInt
	minY, maxY := math.MaxInt, math.MinInt
	for c := range cells {
		if c[0] < minX {
			minX = c[0]
		}
		if c[0] > maxX {
			maxX = c[0]
		}
		if c[1] < minY {
			minY = c[1]
		}
		if c[1] > maxY {
			maxY = c[1]
		}
	}
	w := (maxX-minX+1)*4 - 1
	h := (maxY-minY+1)*2 - 1
	if h < 1 {
		h = 1
	}

	rows := make([][]rune, h)
	for i := range rows {
		rows[i] = make([]rune, w)
		for j := range rows[i] {
			rows[i][j] = ' '
		}
	}

	// Sort coords for deterministic glyph placement (no functional
	// effect today, but pins test output if iteration ever becomes
	// load-bearing).
	keys := make([][2]int, 0, len(cells))
	for c := range cells {
		keys = append(keys, c)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][1] != keys[j][1] {
			return keys[i][1] < keys[j][1]
		}
		return keys[i][0] < keys[j][0]
	})

	// Per-row, per-column overlay for cfmt-wrapped glyph spans. The
	// rune buffer above only holds plain ASCII; wrapped strings carry
	// cfmt tags whose bytes don't cost terminal columns and so cannot
	// safely live in a rune grid. We record (row, colStart) → wrapped
	// and substitute at emit time. The overlay struct is file-scoped
	// so emitRow can take its slice type explicitly.
	overlays := make(map[int][]overlay, len(rows))

	// Place cell glyphs.
	for _, k := range keys {
		info := cells[k]
		col := (k[0] - minX) * 4
		row := (k[1] - minY) * 2
		glyph, wrapped := wrapCell(info, k == current)
		// glyph is always a 3-rune ASCII bracketed form; guard the
		// write so a future multi-byte glyph (or a stray cfmt tag
		// accidentally placed here in the bare glyph) truncates
		// instead of panicking.
		for i, r := range glyph {
			if col+i >= w {
				break
			}
			rows[row][col+i] = r
		}
		if wrapped != "" && wrapped != glyph {
			overlays[row] = append(overlays[row], overlay{col: col, wrapped: wrapped})
		}
	}

	// Place connectors. Only render edges whose endpoints are
	// grid-adjacent (|dx|<=1, |dy|<=1) and whose destination cell is
	// in the grid. Long edges (cycles that resolved to a distant
	// first-visit coord) are silently dropped — the connector would
	// have to bend through other cells to reach.
	for _, eg := range edges {
		dx := eg.to[0] - eg.from[0]
		dy := eg.to[1] - eg.from[1]
		if dx == 0 && dy == 0 {
			continue
		}
		if dx < -1 || dx > 1 || dy < -1 || dy > 1 {
			continue
		}
		if cells[eg.to] == nil {
			continue
		}
		col := (eg.from[0] - minX) * 4
		row := (eg.from[1] - minY) * 2
		var ccol, crow int
		var glyph rune
		switch {
		case dx == 1 && dy == 0:
			ccol, crow, glyph = col+3, row, '-'
		case dx == -1 && dy == 0:
			ccol, crow, glyph = col-1, row, '-'
		case dx == 0 && dy == 1:
			ccol, crow, glyph = col+1, row+1, '|'
		case dx == 0 && dy == -1:
			ccol, crow, glyph = col+1, row-1, '|'
		case dx == 1 && dy == 1:
			ccol, crow, glyph = col+3, row+1, '\\'
		case dx == -1 && dy == -1:
			ccol, crow, glyph = col-1, row-1, '\\'
		case dx == 1 && dy == -1:
			ccol, crow, glyph = col+3, row-1, '/'
		case dx == -1 && dy == 1:
			ccol, crow, glyph = col-1, row+1, '/'
		}
		if crow < 0 || crow >= h || ccol < 0 || ccol >= w {
			continue
		}
		rows[crow][ccol] = glyph
	}

	var b strings.Builder
	for rIdx, r := range rows {
		b.WriteString("  ")
		b.WriteString(emitRow(r, overlays[rIdx]))
		b.WriteString("\r\n")
	}
	return b.String()
}

// emitRow renders one rune row, substituting each cell overlay's
// wrapped string in place of its 3-rune span. Overlays are sorted by
// column ascending; the writer walks the row, splices wrapped forms
// in, and skips the 3 cell columns each time.
func emitRow(runes []rune, ov []overlay) string {
	if len(ov) == 0 {
		return string(runes)
	}
	sort.Slice(ov, func(i, j int) bool { return ov[i].col < ov[j].col })
	var b strings.Builder
	cursor := 0
	for _, o := range ov {
		if o.col < cursor {
			// Two cells overlapping is a layout invariant violation;
			// skip the second to keep output legible.
			continue
		}
		if o.col > cursor {
			b.WriteString(string(runes[cursor:o.col]))
		}
		b.WriteString(o.wrapped)
		cursor = o.col + 3
		if cursor > len(runes) {
			cursor = len(runes)
		}
	}
	if cursor < len(runes) {
		b.WriteString(string(runes[cursor:]))
	}
	return b.String()
}

// overlay is the per-cell cfmt-tagged span recorded during glyph
// placement so emitRow can substitute it for the bare-rune span. Kept
// at file scope (rather than nested in drawGrid) so emitRow can take
// its slice type explicitly.
type overlay struct {
	col     int
	wrapped string
}

// playerCellWrap picks the bracketed cell glyph used by the player
// `map` command and returns it alongside its cfmt-tagged wire form.
// Glyph rules (preserved): seed `[*]`, NoMap `[?]`, vertical-link
// decorations `[^]/[v]/[%]`, plain `[ ]`. Color rules: `[*]` is
// always yellow|bold, `[?]` is always gray, every other glyph picks
// up its sector color via sectorCfmtStyle (yellow fallback). Sector
// glyph **letters** stay admin-only; the player sees only color.
func playerCellWrap(c *cell, isCurrent bool) (glyph, wrapped string) {
	switch {
	case isCurrent:
		glyph = "[*]"
		return glyph, "{{" + glyph + "}}::yellow|bold"
	case c.unknown:
		glyph = "[?]"
		return glyph, "{{" + glyph + "}}::gray"
	case c.hasUp && c.hasDown:
		glyph = "[%]"
	case c.hasUp:
		glyph = "[^]"
	case c.hasDown:
		glyph = "[v]"
	default:
		glyph = "[ ]"
	}
	style := sectorCfmtStyle(c.sector)
	if style == "" {
		style = "yellow"
	}
	return glyph, "{{" + glyph + "}}::" + style
}
