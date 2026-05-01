package telnet

import (
	"bytes"
	"testing"
)

func TestLineEditInsertEnd(t *testing.T) {
	var l LineEdit
	if got := l.Insert('a'); !bytes.Equal(got, []byte{'a'}) {
		t.Fatalf("insert 'a' echo = %q", got)
	}
	if got := l.Insert('b'); !bytes.Equal(got, []byte{'b'}) {
		t.Fatalf("insert 'b' echo = %q", got)
	}
	if string(l.Buf) != "ab" || l.Cursor != 2 {
		t.Fatalf("state = %q cur=%d", l.Buf, l.Cursor)
	}
}

func TestLineEditInsertMidLine(t *testing.T) {
	l := LineEdit{Buf: []byte("ac"), Cursor: 1}
	got := l.Insert('b')
	// Echo: 'b' + suffix "c" + 1 backspace
	want := []byte{'b', 'c', ASCII_BS}
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = %q, want %q", got, want)
	}
	if string(l.Buf) != "abc" || l.Cursor != 2 {
		t.Fatalf("state = %q cur=%d", l.Buf, l.Cursor)
	}
}

// Pinned test: insertion into a multi-byte trailing suffix must not
// corrupt the suffix. Go's copy is memmove-backed so the in-place
// right-shift is safe; this test prevents future regressions if the
// implementation ever moves to a hand-rolled left-to-right loop.
func TestLineEditInsertMultiByteSuffix(t *testing.T) {
	l := LineEdit{Buf: []byte("acde"), Cursor: 1}
	got := l.Insert('b')
	want := []byte{'b', 'c', 'd', 'e', ASCII_BS, ASCII_BS, ASCII_BS}
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = %q, want %q", got, want)
	}
	if string(l.Buf) != "abcde" || l.Cursor != 2 {
		t.Fatalf("state = %q cur=%d, want abcde cur=2", l.Buf, l.Cursor)
	}
}

func TestLineEditBackspaceEnd(t *testing.T) {
	l := LineEdit{Buf: []byte("ab"), Cursor: 2}
	got := l.Backspace()
	want := []byte{ASCII_BS, ' ', ASCII_BS}
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = %q, want %q", got, want)
	}
	if string(l.Buf) != "a" || l.Cursor != 1 {
		t.Fatalf("state = %q cur=%d", l.Buf, l.Cursor)
	}
}

func TestLineEditBackspaceMid(t *testing.T) {
	l := LineEdit{Buf: []byte("abc"), Cursor: 2}
	got := l.Backspace()
	// \b + "c" + " " + \b\b
	want := []byte{ASCII_BS, 'c', ' ', ASCII_BS, ASCII_BS}
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = %q, want %q", got, want)
	}
	if string(l.Buf) != "ac" || l.Cursor != 1 {
		t.Fatalf("state = %q cur=%d", l.Buf, l.Cursor)
	}
}

func TestLineEditBackspaceAtZero(t *testing.T) {
	var l LineEdit
	if got := l.Backspace(); got != nil {
		t.Fatalf("backspace at 0 should be nil, got %q", got)
	}
}

func TestLineEditDelete(t *testing.T) {
	l := LineEdit{Buf: []byte("abc"), Cursor: 1}
	got := l.Delete()
	// suffix after delete is "c"; echo = "c" + " " + \b\b
	want := []byte{'c', ' ', ASCII_BS, ASCII_BS}
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = %q, want %q", got, want)
	}
	if string(l.Buf) != "ac" || l.Cursor != 1 {
		t.Fatalf("state = %q cur=%d", l.Buf, l.Cursor)
	}
}

func TestLineEditMoveLeftRight(t *testing.T) {
	l := LineEdit{Buf: []byte("abc"), Cursor: 3}
	if got := l.MoveLeft(); !bytes.Equal(got, []byte{ASCII_BS}) {
		t.Fatalf("left echo = %q", got)
	}
	if l.Cursor != 2 {
		t.Fatalf("cursor = %d, want 2", l.Cursor)
	}
	if got := l.MoveRight(); !bytes.Equal(got, []byte{'c'}) {
		t.Fatalf("right echo = %q", got)
	}
	if l.Cursor != 3 {
		t.Fatalf("cursor = %d, want 3", l.Cursor)
	}
	// Right at end: nil.
	if got := l.MoveRight(); got != nil {
		t.Fatalf("right at end = %q, want nil", got)
	}
}

func TestLineEditMoveHomeEnd(t *testing.T) {
	l := LineEdit{Buf: []byte("hello"), Cursor: 3}
	if got := l.MoveHome(); !bytes.Equal(got, bytes.Repeat([]byte{ASCII_BS}, 3)) {
		t.Fatalf("home echo = %q", got)
	}
	if l.Cursor != 0 {
		t.Fatalf("cursor after home = %d", l.Cursor)
	}
	if got := l.MoveEnd(); !bytes.Equal(got, []byte("hello")) {
		t.Fatalf("end echo = %q", got)
	}
	if l.Cursor != 5 {
		t.Fatalf("cursor after end = %d", l.Cursor)
	}
}

func TestLineEditKillToStart(t *testing.T) {
	l := LineEdit{Buf: []byte("hello"), Cursor: 3}
	got := l.KillToStart()
	// removed=3, rest="lo"
	// \b\b\b + "lo" + "   " + \b\b\b\b\b
	want := []byte{ASCII_BS, ASCII_BS, ASCII_BS, 'l', 'o', ' ', ' ', ' ',
		ASCII_BS, ASCII_BS, ASCII_BS, ASCII_BS, ASCII_BS}
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = %q, want %q", got, want)
	}
	if string(l.Buf) != "lo" || l.Cursor != 0 {
		t.Fatalf("state = %q cur=%d", l.Buf, l.Cursor)
	}
}

func TestLineEditKillToEnd(t *testing.T) {
	l := LineEdit{Buf: []byte("hello"), Cursor: 2}
	got := l.KillToEnd()
	// removed=3 ("llo")
	want := []byte{' ', ' ', ' ', ASCII_BS, ASCII_BS, ASCII_BS}
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = %q, want %q", got, want)
	}
	if string(l.Buf) != "he" || l.Cursor != 2 {
		t.Fatalf("state = %q cur=%d", l.Buf, l.Cursor)
	}
}

func TestLineEditKillPrevWord(t *testing.T) {
	l := LineEdit{Buf: []byte("look north  "), Cursor: 12}
	l.KillPrevWord()
	if string(l.Buf) != "look " {
		t.Fatalf("buf = %q, want %q", l.Buf, "look ")
	}
	if l.Cursor != 5 {
		t.Fatalf("cursor = %d, want 5", l.Cursor)
	}
}

func TestLineEditReplace(t *testing.T) {
	l := LineEdit{Buf: []byte("ab"), Cursor: 1}
	got := l.Replace("xyz")
	// \b (move to 0), "  " (erase 2), \b\b (back to 0), "xyz"
	want := []byte{ASCII_BS, ' ', ' ', ASCII_BS, ASCII_BS, 'x', 'y', 'z'}
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = %q, want %q", got, want)
	}
	if string(l.Buf) != "xyz" || l.Cursor != 3 {
		t.Fatalf("state = %q cur=%d", l.Buf, l.Cursor)
	}
}

func TestLineEditReset(t *testing.T) {
	l := LineEdit{Buf: []byte("abc"), Cursor: 2}
	l.Reset()
	if len(l.Buf) != 0 || l.Cursor != 0 {
		t.Fatalf("state = %q cur=%d, want empty", l.Buf, l.Cursor)
	}
}
