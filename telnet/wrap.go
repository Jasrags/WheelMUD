package telnet

import (
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// WrapText reflows text to width display columns, treating ANSI CSI escapes
// (`\x1b[...m` and friends) as zero-width so colored output wraps correctly.
// Existing newlines are preserved and reset the column counter; CR bytes are
// dropped so the caller can rejoin with `\r\n`.
//
// When `cellWidth` is true the column accounting uses display-cell width
// (CJK and emoji glyphs count as 2 cells, zero-width joiners as 0). This
// is correct for terminals that have negotiated CHARSET UTF-8. When false,
// rune count is used — the legacy ASCII-only behavior that ships fast on
// 7-bit text but mis-wraps CJK by half a line.
//
// Width <= 0 disables wrapping. Tokens longer than width are split into
// successive chunks at the column boundary — a bare cut, no hyphen —
// so URLs and long item names stay copy-pasteable. Pathological cases
// where the very first rune of a token already exceeds the width cap
// (e.g. width=1 with a 2-cell CJK glyph) overflow by one chunk to
// guarantee forward progress.
func WrapText(text string, width int, cellWidth bool) string {
	if width <= 0 || text == "" {
		return text
	}
	var out strings.Builder
	out.Grow(len(text) + len(text)/width)

	col := 0
	i := 0
	for i < len(text) {
		c := text[i]

		if c == '\x1b' {
			j := skipANSI(text, i)
			out.WriteString(text[i:j])
			i = j
			continue
		}
		if c == '\n' {
			out.WriteByte('\n')
			col = 0
			i++
			continue
		}
		if c == '\r' {
			i++
			continue
		}
		if c == ' ' || c == '\t' {
			if col >= width {
				out.WriteByte('\n')
				col = 0
				i++
				continue
			}
			out.WriteByte(' ')
			col++
			i++
			continue
		}

		// Read a word: run of non-space, non-newline, non-escape bytes.
		j := i
		for j < len(text) {
			b := text[j]
			if b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\x1b' {
				break
			}
			j++
		}
		word := text[i:j]
		i = j
		wlen := tokenWidth(word, cellWidth)

		if wlen <= width {
			// Fits on a line. Soft-break to a new line if the
			// remaining space on the current one is too tight.
			if col > 0 && col+wlen > width {
				trimTrailingSpace(&out)
				out.WriteByte('\n')
				col = 0
			}
			out.WriteString(word)
			col += wlen
			continue
		}

		// Token itself exceeds width. Push it to a fresh line and
		// chunk it at the column boundary.
		if col > 0 {
			trimTrailingSpace(&out)
			out.WriteByte('\n')
			col = 0
		}
		remaining := word
		for remaining != "" {
			head, tail := splitAtWidth(remaining, width, cellWidth)
			out.WriteString(head)
			if tail == "" {
				col = tokenWidth(head, cellWidth)
				break
			}
			out.WriteByte('\n')
			remaining = tail
		}
	}
	return out.String()
}

// tokenWidth returns the display-cell or rune count of s, switched
// by the cellWidth flag. Extracted so the WrapText main loop reads
// without duplicating the switch.
func tokenWidth(s string, cellWidth bool) int {
	if cellWidth {
		return runewidth.StringWidth(s)
	}
	return utf8.RuneCountInString(s)
}

// splitAtWidth walks s rune by rune and returns (head, tail) where
// head is the largest prefix whose cell width is <= width. When the
// very first rune already exceeds the width cap, that rune still
// lands in head — the chunk overflows by `RuneWidth(r) - width`
// cells — so the caller's chunk loop is guaranteed to make forward
// progress on any input. width <= 0 returns the whole string as
// head, mirroring the WrapText "no wrap" contract.
func splitAtWidth(s string, width int, cellWidth bool) (string, string) {
	if width <= 0 || s == "" {
		return s, ""
	}
	col := 0
	cut := 0
	for cut < len(s) {
		r, sz := utf8.DecodeRuneInString(s[cut:])
		w := 1
		if cellWidth {
			w = runewidth.RuneWidth(r)
		}
		if col+w > width {
			if cut == 0 {
				// Pathological width cap: first rune alone is wider
				// than width. Emit it solo so the loop terminates.
				return s[:sz], s[sz:]
			}
			break
		}
		col += w
		cut += sz
	}
	return s[:cut], s[cut:]
}

// skipANSI returns the index just past an ANSI escape that starts at i.
// Handles CSI (`ESC [ ... final`) and short two-byte escapes; bounded so a
// pathological stream cannot pin the wrapper.
func skipANSI(s string, i int) int {
	const maxEscape = 64
	if i >= len(s) || s[i] != '\x1b' {
		return i
	}
	end := i + maxEscape
	if end > len(s) {
		end = len(s)
	}
	j := i + 1
	if j >= end {
		return j
	}
	// CSI: ESC [
	if s[j] == '[' {
		j++
		for j < end {
			b := s[j]
			j++
			if b >= 0x40 && b <= 0x7e {
				return j
			}
		}
		return j
	}
	// OSC: ESC ]  ... BEL or ESC \
	if s[j] == ']' {
		j++
		for j < end {
			if s[j] == 0x07 {
				return j + 1
			}
			if s[j] == '\x1b' && j+1 < end && s[j+1] == '\\' {
				return j + 2
			}
			j++
		}
		return j
	}
	// Short two-byte escape: ESC X.
	return j + 1
}

func trimTrailingSpace(b *strings.Builder) {
	s := b.String()
	if len(s) == 0 || s[len(s)-1] != ' ' {
		return
	}
	b.Reset()
	b.WriteString(s[:len(s)-1])
}
