package telnet

import (
	"bytes"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// LineEdit is a cursor-aware input model. All editing primitives mutate
// the buffer + cursor and return the bytes that must be written to the
// terminal to keep the display in sync. A nil/empty return means the
// caller has nothing to write (e.g., MoveLeft at column 0).
//
// LineEdit is owned by the read goroutine inside RunSession; it is not
// safe for concurrent use. The same goroutine-affinity rule applies as
// to the input buffer it replaces.
//
// **Width model**: Buf holds raw UTF-8 bytes; Cursor is the BYTE
// offset. Editing primitives decode by rune (`utf8.DecodeRune` /
// `DecodeLastRune`) so a multi-byte glyph edits as one unit, and the
// echo bytes use cell counts (`runewidth.RuneWidth` /
// `runewidth.StringWidth`) so the terminal cursor advances the right
// number of columns. CJK fullwidth glyphs render as 2 cells; combining
// marks as 0 (a minimum of 1 BS is emitted on backspace so the cursor
// always advances).
type LineEdit struct {
	Buf    []byte
	Cursor int
}

// cellWidth is the runewidth-aware width of a byte slice. Used to
// size BS echo strings across all primitives.
func cellWidth(b []byte) int { return runewidth.StringWidth(string(b)) }

// runeCellWidth wraps RuneWidth with the minimum-of-1 floor that
// backspace / delete / motion need: a 0-width combining mark would
// otherwise emit zero `\b`s and the cursor would stall.
func runeCellWidth(r rune) int {
	w := runewidth.RuneWidth(r)
	if w <= 0 {
		w = 1
	}
	return w
}

// Reset zeroes the buffer and cursor. Called after a line is dispatched.
// Returns nil — the caller has already written CRLF.
func (l *LineEdit) Reset() {
	l.Buf = l.Buf[:0]
	l.Cursor = 0
}

// Insert places one ASCII byte at the cursor position. Kept as the
// fast path for the dispatcher's 0x00-0x7F branch; non-ASCII bytes
// route through InsertRune via the UTF-8 collector instead.
//
// Returns the bytes the terminal must receive to render the change
// in place. Mid-buffer inserts re-emit the suffix and backspace by
// suffix cell count (correct for ASCII because cell == rune == byte).
func (l *LineEdit) Insert(b byte) []byte {
	l.Buf = append(l.Buf, 0)
	copy(l.Buf[l.Cursor+1:], l.Buf[l.Cursor:])
	l.Buf[l.Cursor] = b
	l.Cursor++

	suffix := l.Buf[l.Cursor:]
	if len(suffix) == 0 {
		return []byte{b}
	}
	suffixCells := cellWidth(suffix)
	out := make([]byte, 0, 1+len(suffix)+suffixCells)
	out = append(out, b)
	out = append(out, suffix...)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, suffixCells)...)
	return out
}

// InsertRune places a multi-byte rune at the cursor. Echoes the rune's
// UTF-8 bytes (terminal renders the glyph and advances its cursor by
// the rune's cell width) followed by the suffix and a cell-aware
// backspace train so the input cursor lands back at its logical
// position.
func (l *LineEdit) InsertRune(r rune) []byte {
	encbuf := make([]byte, utf8.UTFMax)
	n := utf8.EncodeRune(encbuf, r)
	enc := encbuf[:n]

	// Splice the rune's bytes in at l.Cursor.
	l.Buf = append(l.Buf, enc...) // grow capacity
	copy(l.Buf[l.Cursor+n:], l.Buf[l.Cursor:len(l.Buf)-n])
	copy(l.Buf[l.Cursor:], enc)
	l.Cursor += n

	suffix := l.Buf[l.Cursor:]
	if len(suffix) == 0 {
		out := make([]byte, n)
		copy(out, enc)
		return out
	}
	suffixCells := cellWidth(suffix)
	out := make([]byte, 0, n+len(suffix)+suffixCells)
	out = append(out, enc...)
	out = append(out, suffix...)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, suffixCells)...)
	return out
}

// Backspace removes the rune ending at the cursor. Returns nil at
// column 0. The echo erases the rune's cells via backspaces, redraws
// the suffix, blanks the now-vacated trailing cells, and backspaces
// the cursor to the new position.
func (l *LineEdit) Backspace() []byte {
	if l.Cursor == 0 {
		return nil
	}
	// DecodeLastRune on a non-empty slice always returns size >= 1
	// (RuneError + size 1 for a malformed trailing byte). The
	// Cursor==0 guard above ensures the slice is non-empty, so a
	// size==0 check would be dead. Malformed bytes degrade
	// gracefully: w=1, single-byte removal.
	r, size := utf8.DecodeLastRune(l.Buf[:l.Cursor])
	w := runeCellWidth(r)
	l.Buf = append(l.Buf[:l.Cursor-size], l.Buf[l.Cursor:]...)
	l.Cursor -= size

	suffix := l.Buf[l.Cursor:]
	suffixCells := cellWidth(suffix)
	out := make([]byte, 0, w+len(suffix)+w+suffixCells+w)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, w)...)
	out = append(out, suffix...)
	out = append(out, bytes.Repeat([]byte{' '}, w)...)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, suffixCells+w)...)
	return out
}

// Delete removes the rune at the cursor (forward-delete). Returns
// nil at end of line.
func (l *LineEdit) Delete() []byte {
	if l.Cursor >= len(l.Buf) {
		return nil
	}
	// DecodeRune on a non-empty slice always returns size >= 1; the
	// Cursor>=len guard above is the empty-slice check.
	r, size := utf8.DecodeRune(l.Buf[l.Cursor:])
	w := runeCellWidth(r)
	l.Buf = append(l.Buf[:l.Cursor], l.Buf[l.Cursor+size:]...)
	suffix := l.Buf[l.Cursor:]
	suffixCells := cellWidth(suffix)
	out := make([]byte, 0, len(suffix)+w+suffixCells+w)
	out = append(out, suffix...)
	out = append(out, bytes.Repeat([]byte{' '}, w)...)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, suffixCells+w)...)
	return out
}

// MoveLeft moves the cursor one rune left. Returns nil at column 0.
func (l *LineEdit) MoveLeft() []byte {
	if l.Cursor == 0 {
		return nil
	}
	r, size := utf8.DecodeLastRune(l.Buf[:l.Cursor])
	l.Cursor -= size
	return bytes.Repeat([]byte{ASCII_BS}, runeCellWidth(r))
}

// MoveRight moves the cursor one rune right by re-emitting the rune's
// UTF-8 bytes (terminal re-renders and advances). Returns nil at end
// of line.
func (l *LineEdit) MoveRight() []byte {
	if l.Cursor >= len(l.Buf) {
		return nil
	}
	_, size := utf8.DecodeRune(l.Buf[l.Cursor:])
	out := append([]byte(nil), l.Buf[l.Cursor:l.Cursor+size]...)
	l.Cursor += size
	return out
}

// MoveHome moves the cursor to column 0. Echoes BS×cellWidth(prefix).
func (l *LineEdit) MoveHome() []byte {
	if l.Cursor == 0 {
		return nil
	}
	cells := cellWidth(l.Buf[:l.Cursor])
	l.Cursor = 0
	return bytes.Repeat([]byte{ASCII_BS}, cells)
}

// MoveEnd moves the cursor past the last byte. Echoes the trailing
// bytes (terminal renders + advances).
func (l *LineEdit) MoveEnd() []byte {
	if l.Cursor >= len(l.Buf) {
		return nil
	}
	out := append([]byte(nil), l.Buf[l.Cursor:]...)
	l.Cursor = len(l.Buf)
	return out
}

// KillToStart removes everything from column 0 up to (but not including)
// the cursor. Cursor lands at column 0.
func (l *LineEdit) KillToStart() []byte {
	if l.Cursor == 0 {
		return nil
	}
	prefixCells := cellWidth(l.Buf[:l.Cursor])
	rest := append([]byte(nil), l.Buf[l.Cursor:]...)
	restCells := cellWidth(rest)
	l.Buf = append(l.Buf[:0], rest...)
	l.Cursor = 0

	out := make([]byte, 0, prefixCells+len(rest)+prefixCells+restCells+prefixCells)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, prefixCells)...)
	out = append(out, rest...)
	out = append(out, bytes.Repeat([]byte{' '}, prefixCells)...)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, restCells+prefixCells)...)
	return out
}

// KillToEnd removes everything from the cursor to the end of the line.
// Cursor stays put.
func (l *LineEdit) KillToEnd() []byte {
	if l.Cursor >= len(l.Buf) {
		return nil
	}
	removedCells := cellWidth(l.Buf[l.Cursor:])
	l.Buf = l.Buf[:l.Cursor]
	out := make([]byte, 0, 2*removedCells)
	out = append(out, bytes.Repeat([]byte{' '}, removedCells)...)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, removedCells)...)
	return out
}

// KillPrevWord removes from the start of the previous word up to the
// cursor. "Word" boundary is whitespace; we first skip trailing
// whitespace, then run back through non-whitespace. Rune-aware: a
// CJK glyph counts as one non-space rune.
func (l *LineEdit) KillPrevWord() []byte {
	if l.Cursor == 0 {
		return nil
	}
	i := l.Cursor
	// Skip trailing whitespace (decoded rune-by-rune).
	for i > 0 {
		r, size := utf8.DecodeLastRune(l.Buf[:i])
		if size == 0 || !unicode.IsSpace(r) {
			break
		}
		i -= size
	}
	// Walk back through non-whitespace.
	for i > 0 {
		r, size := utf8.DecodeLastRune(l.Buf[:i])
		if size == 0 || unicode.IsSpace(r) {
			break
		}
		i -= size
	}
	if i == l.Cursor {
		return nil
	}
	removedCells := cellWidth(l.Buf[i:l.Cursor])
	suffix := append([]byte(nil), l.Buf[l.Cursor:]...)
	suffixCells := cellWidth(suffix)
	l.Buf = append(l.Buf[:i], suffix...)
	l.Cursor = i

	out := make([]byte, 0, removedCells+len(suffix)+removedCells+suffixCells+removedCells)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, removedCells)...)
	out = append(out, suffix...)
	out = append(out, bytes.Repeat([]byte{' '}, removedCells)...)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, suffixCells+removedCells)...)
	return out
}

// Replace swaps the entire line for s. Used by history navigation. The
// echo erases the old display (cell-aware), writes s, and leaves the
// cursor at end.
func (l *LineEdit) Replace(s string) []byte {
	prefixCells := cellWidth(l.Buf[:l.Cursor])
	oldCells := cellWidth(l.Buf)
	out := make([]byte, 0, prefixCells+oldCells+oldCells+len(s))
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, prefixCells)...)
	out = append(out, bytes.Repeat([]byte{' '}, oldCells)...)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, oldCells)...)
	out = append(out, s...)
	l.Buf = append(l.Buf[:0], s...)
	l.Cursor = len(l.Buf)
	return out
}
