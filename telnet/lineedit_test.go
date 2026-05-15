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

// CJK / cell-width coverage. The character `中` is 0xE4 0xB8 0xAD in
// UTF-8 (3 bytes) and 2 cells wide. Tests assert that primitives
// edit it as one unit and emit cell-count backspaces.

var cjkZhong = []byte{0xE4, 0xB8, 0xAD} // "中"
var cjkWen = []byte{0xE6, 0x96, 0x87}   // "文"

func TestLineEditInsertRune_CJKAtEnd(t *testing.T) {
	l := LineEdit{}
	got := l.InsertRune('中')
	if !bytes.Equal(got, cjkZhong) {
		t.Fatalf("echo = % x, want % x", got, cjkZhong)
	}
	if !bytes.Equal(l.Buf, cjkZhong) || l.Cursor != 3 {
		t.Fatalf("state = % x cur=%d, want %x cur=3", l.Buf, l.Cursor, cjkZhong)
	}
}

func TestLineEditInsertRune_CJKMidBuffer(t *testing.T) {
	l := LineEdit{Buf: []byte("ab"), Cursor: 1}
	got := l.InsertRune('中')
	// Buf becomes "a" + 0xE4 0xB8 0xAD + "b". Echo: rune bytes + "b"
	// + 1 BS (suffix "b" is 1 cell wide).
	wantBuf := append(append([]byte("a"), cjkZhong...), 'b')
	if !bytes.Equal(l.Buf, wantBuf) || l.Cursor != 4 {
		t.Fatalf("state = % x cur=%d, want % x cur=4", l.Buf, l.Cursor, wantBuf)
	}
	wantEcho := append(append([]byte{}, cjkZhong...), 'b', ASCII_BS)
	if !bytes.Equal(got, wantEcho) {
		t.Fatalf("echo = % x, want % x", got, wantEcho)
	}
}

func TestLineEditBackspace_CJKAtEnd(t *testing.T) {
	l := LineEdit{Buf: append([]byte{}, cjkZhong...), Cursor: 3}
	got := l.Backspace()
	// 2-cell glyph erase: \b\b + (empty suffix) + "  " + \b\b
	want := []byte{ASCII_BS, ASCII_BS, ' ', ' ', ASCII_BS, ASCII_BS}
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = % x, want % x", got, want)
	}
	if len(l.Buf) != 0 || l.Cursor != 0 {
		t.Fatalf("state = % x cur=%d, want empty", l.Buf, l.Cursor)
	}
}

func TestLineEditBackspace_CJKMidBuffer(t *testing.T) {
	// "a中b" cursor=4 (past 中). Backspace removes 中.
	buf := append(append([]byte("a"), cjkZhong...), 'b')
	l := LineEdit{Buf: append([]byte{}, buf...), Cursor: 4}
	got := l.Backspace()
	// w=2, suffix="b" (1 cell). Echo: \b\b + b + "  " + \b\b\b (suffixCells+w).
	want := []byte{ASCII_BS, ASCII_BS, 'b', ' ', ' ', ASCII_BS, ASCII_BS, ASCII_BS}
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = % x, want % x", got, want)
	}
	if string(l.Buf) != "ab" || l.Cursor != 1 {
		t.Fatalf("state = %q cur=%d, want ab cur=1", l.Buf, l.Cursor)
	}
}

func TestLineEditDelete_CJK(t *testing.T) {
	buf := append(append([]byte("a"), cjkZhong...), 'b')
	l := LineEdit{Buf: append([]byte{}, buf...), Cursor: 1}
	got := l.Delete()
	// w=2, suffix="b" (1 cell). Echo: b + "  " + \b\b\b.
	want := []byte{'b', ' ', ' ', ASCII_BS, ASCII_BS, ASCII_BS}
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = % x, want % x", got, want)
	}
	if string(l.Buf) != "ab" || l.Cursor != 1 {
		t.Fatalf("state = %q cur=%d", l.Buf, l.Cursor)
	}
}

func TestLineEditMoveLeft_CJK(t *testing.T) {
	l := LineEdit{Buf: append([]byte{}, cjkZhong...), Cursor: 3}
	got := l.MoveLeft()
	want := []byte{ASCII_BS, ASCII_BS} // 2 cells
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = % x, want % x", got, want)
	}
	if l.Cursor != 0 {
		t.Fatalf("cursor = %d, want 0", l.Cursor)
	}
}

func TestLineEditMoveRight_CJK(t *testing.T) {
	l := LineEdit{Buf: append([]byte{}, cjkZhong...), Cursor: 0}
	got := l.MoveRight()
	if !bytes.Equal(got, cjkZhong) {
		t.Fatalf("echo = % x, want % x", got, cjkZhong)
	}
	if l.Cursor != 3 {
		t.Fatalf("cursor = %d, want 3", l.Cursor)
	}
}

func TestLineEditMoveHome_AfterCJK(t *testing.T) {
	buf := append(append([]byte{}, cjkZhong...), cjkWen...)
	l := LineEdit{Buf: append([]byte{}, buf...), Cursor: 6}
	got := l.MoveHome()
	// 2 glyphs × 2 cells = 4 BS.
	want := bytes.Repeat([]byte{ASCII_BS}, 4)
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = % x, want % x", got, want)
	}
	if l.Cursor != 0 {
		t.Fatalf("cursor = %d, want 0", l.Cursor)
	}
}

func TestLineEditBackspace_CombiningMark(t *testing.T) {
	// U+0301 COMBINING ACUTE ACCENT is 2 UTF-8 bytes (0xCC 0x81) and
	// runewidth.RuneWidth returns 0 for it. runeCellWidth's floor of
	// 1 bumps that to one cell so the cursor still advances on
	// backspace. A future refactor that removes the floor would
	// regress this case; pin it.
	l := LineEdit{}
	_ = l.InsertRune('́')
	got := l.Backspace()
	// w = 1 (floored), suffix empty. Echo: \b + (empty) + ' ' + \b.
	want := []byte{ASCII_BS, ' ', ASCII_BS}
	if !bytes.Equal(got, want) {
		t.Fatalf("echo = % x, want % x", got, want)
	}
	if len(l.Buf) != 0 || l.Cursor != 0 {
		t.Fatalf("state = % x cur=%d, want empty cur=0", l.Buf, l.Cursor)
	}
}

func TestLineEditKillPrevWord_CJK(t *testing.T) {
	// "ab 中文 " (cursor at end after the trailing space).
	// KillPrevWord skips the trailing space then walks back through
	// the non-space "中文" run, leaving "ab ".
	buf := append(append([]byte("ab "), append(cjkZhong, cjkWen...)...), ' ')
	l := LineEdit{Buf: append([]byte{}, buf...), Cursor: len(buf)}
	_ = l.KillPrevWord()
	if string(l.Buf) != "ab " || l.Cursor != 3 {
		t.Fatalf("state = %q cur=%d, want %q cur=3", l.Buf, l.Cursor, "ab ")
	}
}
