package telnet

import (
	"strings"
	"testing"
)

func TestWrapText_Basic(t *testing.T) {
	in := "the quick brown fox jumps over the lazy dog"
	got := WrapText(in, 12, false)
	want := "the quick\nbrown fox\njumps over\nthe lazy dog"
	if got != want {
		t.Fatalf("WrapText\n got:  %q\n want: %q", got, want)
	}
}

func TestWrapText_PreservesExistingNewlines(t *testing.T) {
	in := "para one is short.\npara two is also short."
	got := WrapText(in, 30, false)
	if got != in {
		t.Fatalf("WrapText changed text under width:\n got: %q\n in:  %q", got, in)
	}
}

func TestWrapText_ZeroOrNegativeWidthDisables(t *testing.T) {
	in := "anything goes here without limit"
	for _, w := range []int{0, -1, -100} {
		if got := WrapText(in, w, false); got != in {
			t.Errorf("width=%d: got %q, want unchanged", w, got)
		}
	}
}

func TestWrapText_WordLongerThanWidthOverflows(t *testing.T) {
	in := "tiny supercalifragilistic tail"
	got := WrapText(in, 8, false)
	want := "tiny\nsupercalifragilistic\ntail"
	if got != want {
		t.Fatalf("WrapText\n got:  %q\n want: %q", got, want)
	}
}

func TestWrapText_ANSIEscapesAreZeroWidth(t *testing.T) {
	red := "\x1b[31m"
	reset := "\x1b[0m"
	in := red + "redred redred" + reset + " plain"
	got := WrapText(in, 13, false)
	// "redred redred" is exactly 13 visible chars; escape codes don't push it
	// over the limit, so the line breaks before "plain".
	want := red + "redred redred" + reset + "\nplain"
	if got != want {
		t.Fatalf("WrapText with ANSI\n got:  %q\n want: %q", got, want)
	}
}

func TestWrapText_DropsBareCR(t *testing.T) {
	in := "alpha\r\nbeta\r\ngamma"
	got := WrapText(in, 80, false)
	if strings.Contains(got, "\r") {
		t.Fatalf("output retained CR: %q", got)
	}
	if got != "alpha\nbeta\ngamma" {
		t.Fatalf("got %q", got)
	}
}

func TestWrapText_CellWidthCountsCJKAsTwo(t *testing.T) {
	// Each CJK glyph occupies 2 display cells. At width 10, two glyphs
	// (= 4 cells) plus a space plus the next two glyphs (= 9 cells)
	// fit; adding a third pair (4 more cells = 13) wraps.
	in := "中文 中文 中文"
	got := WrapText(in, 10, true)
	want := "中文 中文\n中文"
	if got != want {
		t.Fatalf("WrapText CJK cellWidth=true\n got:  %q\n want: %q", got, want)
	}
}

func TestWrapText_RuneCountModeUndercountsCJK(t *testing.T) {
	// Defensive regression: in legacy rune-count mode, CJK glyphs still
	// count as 1 rune each. This documents the pre-CHARSET behavior the
	// `cellWidth=false` branch preserves.
	in := "中文中文中文"
	got := WrapText(in, 6, false)
	if got != in {
		t.Fatalf("WrapText CJK cellWidth=false should pass through under width 6\n got: %q\n in:  %q", got, in)
	}
}

func TestSessionWriteWrapped_EmitsCRLFAndReflows(t *testing.T) {
	s, peer := newPipeSession(t)
	s.Width = 12

	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		n, _ := peer.Read(buf)
		got <- string(buf[:n])
	}()

	if err := s.WriteWrapped("the quick brown fox"); err != nil {
		t.Fatalf("WriteWrapped: %v", err)
	}
	out := <-got
	want := "the quick\r\nbrown fox"
	if out != want {
		t.Fatalf("WriteWrapped wire bytes\n got:  %q\n want: %q", out, want)
	}
}

func TestSessionWriteWrapped_ZeroWidthFallsBackToWriteString(t *testing.T) {
	s, peer := newPipeSession(t)
	s.Width = 0

	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		n, _ := peer.Read(buf)
		got <- string(buf[:n])
	}()

	// CRLF-bearing input must not be re-doubled when wrapping is disabled.
	if err := s.WriteWrapped("line one\r\nline two"); err != nil {
		t.Fatalf("WriteWrapped: %v", err)
	}
	out := <-got
	if strings.Contains(out, "\r\r\n") {
		t.Fatalf("CRLF was doubled: %q", out)
	}
	if out != "line one\r\nline two" {
		t.Fatalf("got %q", out)
	}
}
