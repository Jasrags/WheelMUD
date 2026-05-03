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
	// unknown rooms (NoMap destinations) render as `[?]` and the BFS
	// does not recurse through them.
	unknown bool
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
// boundary and stop traversal.
func NewMap(rooms repo.RoomRepo, exits repo.ExitRepo) *telnet.Command {
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
			return renderMap(c.Ctx, c.Session, rooms, exits, depth)
		},
	}
}

func renderMap(ctx context.Context, s *telnet.Session, rooms repo.RoomRepo, exits repo.ExitRepo, depth int) error {
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

	cells, edges, err := exploreMap(ctx, rooms, exits, cur, depth)
	if err != nil {
		return s.WriteString("{{Could not draw the map right now.}}::red\r\n")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "{{Map (depth %d):}}::cyan|bold\r\n", depth)
	b.WriteString(drawGrid(cells, edges, [2]int{0, 0}))
	// Legend uses unbracketed glyphs to avoid colliding with the
	// bracketed cell forms (`[*]`, `[?]`, ...) when callers grep the
	// output for grid contents.
	b.WriteString("  {{*}}::yellow|bold = you, ")
	b.WriteString("{{?}}::gray = unmapped, ")
	b.WriteString("{{^v%}}::yellow = up/down/both\r\n")
	return s.WriteString(b.String())
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
func exploreMap(ctx context.Context, rooms repo.RoomRepo, exits repo.ExitRepo, start repo.Room, depth int) (map[[2]int]*cell, []edge, error) {
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
	cells[origin] = &cell{roomID: start.ID}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		outs, err := exits.ListFrom(ctx, item.roomID)
		if err != nil {
			return nil, nil, err
		}
		for _, e := range outs {
			if e.Flags.Hidden {
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

			if dest.Flags.NoMap {
				cells[nextCoord] = &cell{roomID: dest.ID, unknown: true}
				visited[dest.ID] = nextCoord
				edges = append(edges, edge{item.coord, nextCoord})
				continue
			}

			cells[nextCoord] = &cell{roomID: dest.ID}
			visited[dest.ID] = nextCoord
			edges = append(edges, edge{item.coord, nextCoord})
			queue = append(queue, qItem{dest.ID, nextCoord, item.d + 1})
		}
	}
	return cells, edges, nil
}

// drawGrid lays cells onto a fixed-pitch character grid. Each cell is a
// 3-char glyph; cells are separated by one connector column horizontally
// and one connector row vertically. Cell (x, y) lands at column
// (x-minX)*4 (..+2) and row (y-minY)*2.
//
// Connectors are drawn from the edges slice — never from grid adjacency
// — so two rooms that are visually next to each other but lack an exit
// between them stay disconnected.
func drawGrid(cells map[[2]int]*cell, edges []edge, current [2]int) string {
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

	// Place cell glyphs.
	for _, k := range keys {
		info := cells[k]
		col := (k[0] - minX) * 4
		row := (k[1] - minY) * 2
		glyph := "[ ]"
		switch {
		case k == current:
			glyph = "[*]"
		case info.unknown:
			glyph = "[?]"
		case info.hasUp && info.hasDown:
			glyph = "[%]"
		case info.hasUp:
			glyph = "[^]"
		case info.hasDown:
			glyph = "[v]"
		}
		// glyph is always a 3-rune ASCII bracketed form; guard the
		// write so a future multi-byte glyph (or a stray cfmt tag
		// accidentally placed here instead of in colorizeMapRow)
		// truncates instead of panicking.
		for i, r := range glyph {
			if col+i >= w {
				break
			}
			rows[row][col+i] = r
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
	for _, r := range rows {
		b.WriteString("  ")
		b.WriteString(colorizeMapRow(string(r)))
		b.WriteString("\r\n")
	}
	return b.String()
}

// colorizeMapRow wraps each known cell glyph with cfmt color tags.
// cfmt expands the tags to ANSI escapes around the literal bracketed
// text, so visual column alignment is preserved on the wire.
//
// Invariant: every glyph form is a unique 3-rune string and no glyph
// is a substring of another. If a new glyph is added that violates
// this, ReplaceAll ordering becomes load-bearing and silent
// miscoloring follows; pick a non-overlapping form instead.
func colorizeMapRow(line string) string {
	line = strings.ReplaceAll(line, "[*]", "{{[*]}}::yellow|bold")
	line = strings.ReplaceAll(line, "[?]", "{{[?]}}::gray")
	line = strings.ReplaceAll(line, "[%]", "{{[%]}}::yellow")
	line = strings.ReplaceAll(line, "[^]", "{{[^]}}::yellow")
	line = strings.ReplaceAll(line, "[v]", "{{[v]}}::yellow")
	line = strings.ReplaceAll(line, "[ ]", "{{[ ]}}::yellow")
	return line
}
