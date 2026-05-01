package telnet

import (
	"bytes"
	"unicode"
)

// LineEdit is a cursor-aware input model. All editing primitives mutate
// the buffer + cursor and return the bytes that must be written to the
// terminal to keep the display in sync. A nil/empty return means the
// caller has nothing to write (e.g., MoveLeft at column 0).
//
// LineEdit is owned by the read goroutine inside RunSession; it is not
// safe for concurrent use. The same goroutine-affinity rule applies as
// to the input buffer it replaces.
type LineEdit struct {
	Buf    []byte
	Cursor int
}

// Reset zeroes the buffer and cursor. Called after a line is dispatched.
// Returns nil — the caller has already written CRLF.
func (l *LineEdit) Reset() {
	l.Buf = l.Buf[:0]
	l.Cursor = 0
}

// Insert places b at the cursor position. Returns the bytes the terminal
// must receive to render the change in place.
func (l *LineEdit) Insert(b byte) []byte {
	l.Buf = append(l.Buf, 0)
	copy(l.Buf[l.Cursor+1:], l.Buf[l.Cursor:])
	l.Buf[l.Cursor] = b
	l.Cursor++

	suffix := l.Buf[l.Cursor:]
	if len(suffix) == 0 {
		return []byte{b}
	}
	out := make([]byte, 0, 1+len(suffix)+len(suffix))
	out = append(out, b)
	out = append(out, suffix...)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, len(suffix))...)
	return out
}

// Backspace removes the character before the cursor. Returns nil at
// column 0.
func (l *LineEdit) Backspace() []byte {
	if l.Cursor == 0 {
		return nil
	}
	l.Buf = append(l.Buf[:l.Cursor-1], l.Buf[l.Cursor:]...)
	l.Cursor--

	suffix := l.Buf[l.Cursor:]
	if len(suffix) == 0 {
		return []byte{ASCII_BS, ' ', ASCII_BS}
	}
	out := make([]byte, 0, 1+len(suffix)+1+len(suffix)+1)
	out = append(out, ASCII_BS)
	out = append(out, suffix...)
	out = append(out, ' ')
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, len(suffix)+1)...)
	return out
}

// Delete removes the character at the cursor (forward-delete). Returns
// nil at end of line.
func (l *LineEdit) Delete() []byte {
	if l.Cursor >= len(l.Buf) {
		return nil
	}
	l.Buf = append(l.Buf[:l.Cursor], l.Buf[l.Cursor+1:]...)
	suffix := l.Buf[l.Cursor:]
	out := make([]byte, 0, len(suffix)+1+len(suffix)+1)
	out = append(out, suffix...)
	out = append(out, ' ')
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, len(suffix)+1)...)
	return out
}

// MoveLeft moves the cursor one cell left. Returns nil at column 0.
func (l *LineEdit) MoveLeft() []byte {
	if l.Cursor == 0 {
		return nil
	}
	l.Cursor--
	return []byte{ASCII_BS}
}

// MoveRight moves the cursor one cell right by re-emitting the character
// at the current position. Returns nil at end of line.
func (l *LineEdit) MoveRight() []byte {
	if l.Cursor >= len(l.Buf) {
		return nil
	}
	c := l.Buf[l.Cursor]
	l.Cursor++
	return []byte{c}
}

// MoveHome moves the cursor to column 0.
func (l *LineEdit) MoveHome() []byte {
	if l.Cursor == 0 {
		return nil
	}
	out := bytes.Repeat([]byte{ASCII_BS}, l.Cursor)
	l.Cursor = 0
	return out
}

// MoveEnd moves the cursor past the last character.
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
	removed := l.Cursor
	rest := append([]byte(nil), l.Buf[l.Cursor:]...)
	l.Buf = append(l.Buf[:0], rest...)
	l.Cursor = 0

	out := make([]byte, 0, removed+len(rest)+removed+removed+len(rest))
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, removed)...)
	out = append(out, rest...)
	out = append(out, bytes.Repeat([]byte{' '}, removed)...)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, len(rest)+removed)...)
	return out
}

// KillToEnd removes everything from the cursor to the end of the line.
// Cursor stays put.
func (l *LineEdit) KillToEnd() []byte {
	if l.Cursor >= len(l.Buf) {
		return nil
	}
	removed := len(l.Buf) - l.Cursor
	l.Buf = l.Buf[:l.Cursor]
	out := make([]byte, 0, 2*removed)
	out = append(out, bytes.Repeat([]byte{' '}, removed)...)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, removed)...)
	return out
}

// KillPrevWord removes from the start of the previous word up to the
// cursor. "Word" boundary is whitespace; we first skip trailing
// whitespace, then run back through non-whitespace.
func (l *LineEdit) KillPrevWord() []byte {
	if l.Cursor == 0 {
		return nil
	}
	i := l.Cursor
	for i > 0 && unicode.IsSpace(rune(l.Buf[i-1])) {
		i--
	}
	for i > 0 && !unicode.IsSpace(rune(l.Buf[i-1])) {
		i--
	}
	if i == l.Cursor {
		return nil
	}
	removed := l.Cursor - i
	suffix := append([]byte(nil), l.Buf[l.Cursor:]...)
	l.Buf = append(l.Buf[:i], suffix...)
	l.Cursor = i

	out := make([]byte, 0, removed+len(suffix)+removed+removed+len(suffix))
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, removed)...)
	out = append(out, suffix...)
	out = append(out, bytes.Repeat([]byte{' '}, removed)...)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, len(suffix)+removed)...)
	return out
}

// Replace swaps the entire line for s. Used by history navigation. The
// echo erases the old display, writes s, and leaves the cursor at end.
func (l *LineEdit) Replace(s string) []byte {
	oldLen := len(l.Buf)
	out := make([]byte, 0, l.Cursor+oldLen+oldLen+len(s))
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, l.Cursor)...)
	out = append(out, bytes.Repeat([]byte{' '}, oldLen)...)
	out = append(out, bytes.Repeat([]byte{ASCII_BS}, oldLen)...)
	out = append(out, s...)
	l.Buf = append(l.Buf[:0], s...)
	l.Cursor = len(l.Buf)
	return out
}
