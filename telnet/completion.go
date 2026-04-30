package telnet

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// maxCandidatesShown caps the listing output. Past this we emit
	// "... and N more" rather than spamming the screen.
	maxCandidatesShown = 30
	// helpColumnThreshold is the upper bound for the per-line "name - help"
	// flat layout. Past this we columnize and drop the help text.
	helpColumnThreshold = 10
	// columnGutter is the spacing between columns in columnized output.
	columnGutter = 2
)

// Candidate is a single completion result.
type Candidate struct {
	Text string // the full token to insert in place of the partial
	Help string // optional one-line description, shown in flat listings
}

// Completer is an optional interface for modes that support tab completion.
// Modes that do not implement it cause Tab to bell.
type Completer interface {
	// Complete returns candidates for the current buffer. The buffer is the
	// raw bytes the user has typed so far (no terminator). Returning nil or
	// an empty slice means "no completion available" (caller will bell).
	Complete(s *Session, buffer string) []Candidate
}

// commonPrefix returns the longest string that is a prefix of every input.
func commonPrefix(items []string) string {
	if len(items) == 0 {
		return ""
	}
	cp := items[0]
	for _, s := range items[1:] {
		n := 0
		for n < len(cp) && n < len(s) && cp[n] == s[n] {
			n++
		}
		cp = cp[:n]
		if cp == "" {
			break
		}
	}
	return cp
}

// formatCandidates renders candidates as a multi-line listing suitable for
// printing above a redrawn prompt. width is the terminal width in columns;
// pass <=0 to default to 80. The input slice is not retained or mutated.
func formatCandidates(cands []Candidate, width int) string {
	if width <= 0 {
		width = 80
	}
	total := len(cands)
	visible := cands
	if total > maxCandidatesShown {
		visible = cands[:maxCandidatesShown]
	}

	var b strings.Builder
	if len(visible) <= helpColumnThreshold {
		writeFlat(&b, visible)
	} else {
		writeColumns(&b, visible, width)
	}
	if total > maxCandidatesShown {
		fmt.Fprintf(&b, "... and %d more\r\n", total-maxCandidatesShown)
	}
	return b.String()
}

func writeFlat(b *strings.Builder, cands []Candidate) {
	for _, c := range cands {
		b.WriteString(c.Text)
		if c.Help != "" {
			b.WriteString("  -  ")
			b.WriteString(c.Help)
		}
		b.WriteString("\r\n")
	}
}

func writeColumns(b *strings.Builder, cands []Candidate, width int) {
	// Display widths in runes, not bytes — bytes match runes for ASCII but
	// diverge the moment a candidate carries any multi-byte UTF-8.
	widths := make([]int, len(cands))
	longest := 0
	for i, c := range cands {
		w := utf8.RuneCountInString(c.Text)
		widths[i] = w
		if w > longest {
			longest = w
		}
	}
	colWidth := longest + columnGutter
	cols := width / colWidth
	if cols < 1 {
		cols = 1
	}
	rows := (len(cands) + cols - 1) / cols

	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			idx := c*rows + r
			if idx >= len(cands) {
				continue
			}
			name := cands[idx].Text
			nextIdx := (c+1)*rows + r
			isLastInRow := c == cols-1 || nextIdx >= len(cands)
			b.WriteString(name)
			if !isLastInRow {
				b.WriteString(strings.Repeat(" ", colWidth-widths[idx]))
			}
		}
		b.WriteString("\r\n")
	}
}
