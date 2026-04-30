package telnet

import (
	"strings"
	"unicode/utf8"
)

// WrapText reflows text to width display columns, treating ANSI CSI escapes
// (`\x1b[...m` and friends) as zero-width so colored output wraps correctly.
// Existing newlines are preserved and reset the column counter; CR bytes are
// dropped so the caller can rejoin with `\r\n`.
//
// Width <= 0 disables wrapping. Tokens longer than width are emitted on a
// fresh line and overflow rather than being broken — splitting mid-word would
// require either grapheme awareness or a hyphenation policy, neither of which
// is in scope yet.
func WrapText(text string, width int) string {
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
		wlen := utf8.RuneCountInString(word)
		if col > 0 && col+wlen > width {
			// Trim a single trailing space if present, then break.
			trimTrailingSpace(&out)
			out.WriteByte('\n')
			col = 0
		}
		out.WriteString(word)
		col += wlen
		i = j
	}
	return out.String()
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
