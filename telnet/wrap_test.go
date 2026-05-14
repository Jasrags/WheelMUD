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

func TestWrapText_BreaksLongToken(t *testing.T) {
	// "supercalifragilistic" is 20 runes — at width=8 it must chunk
	// into 8 + 8 + 4. The surrounding short words wrap normally.
	in := "tiny supercalifragilistic tail"
	got := WrapText(in, 8, false)
	want := "tiny\nsupercal\nifragili\nstic\ntail"
	if got != want {
		t.Fatalf("WrapText\n got:  %q\n want: %q", got, want)
	}
}

func TestWrapText_BreaksLongTokenCJK(t *testing.T) {
	// Six CJK glyphs at 2 cells each = 12 cells. At width=6 (3
	// glyphs per line), the token chunks into two lines of three
	// glyphs.
	in := "中文中文中文"
	got := WrapText(in, 6, true)
	want := "中文中\n文中文"
	if got != want {
		t.Fatalf("WrapText CJK chunk\n got:  %q\n want: %q", got, want)
	}
}

func TestWrapText_TokenExactlyWidthNoBreak(t *testing.T) {
	// A token whose width matches the wrap width exactly fits on
	// one line — no break.
	in := "exactfit"
	got := WrapText(in, 8, false)
	if got != in {
		t.Fatalf("WrapText exact-fit\n got:  %q\n want: %q", got, in)
	}
}

func TestWrapText_PathologicalWidthOneOverflows(t *testing.T) {
	// width=1 with a 2-cell CJK glyph: the glyph cannot fit, but
	// emitting nothing would loop forever. Contract: the glyph
	// ships alone and overflows by one cell. The next glyph lands
	// on its own line, etc.
	in := "中文"
	got := WrapText(in, 1, true)
	want := "中\n文"
	if got != want {
		t.Fatalf("WrapText width=1 CJK\n got:  %q\n want: %q", got, want)
	}
}

func TestWrapText_LongTokenInSentence(t *testing.T) {
	// A long URL embedded mid-sentence breaks at the column
	// boundary; the trailing word continues on its own line.
	in := "see https://example.com/very-long-path for details"
	got := WrapText(in, 12, false)
	// "see " (col=4) — "https://example.com/very-long-path" is 34
	// chars at width 12 → 12 + 12 + 10. "for" starts col 0 after
	// the chunk; with "details" appended it's 11 chars → fits on
	// the same line.
	want := "see\nhttps://exam\nple.com/very\n-long-path\nfor details"
	if got != want {
		t.Fatalf("WrapText long URL\n got:  %q\n want: %q", got, want)
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

// TestSessionWriteWrapped_PreCRLFAtNonZeroWidth locks in the
// "WrapText drops CR, ReplaceAll emits CR back exactly once" chain
// for the non-zero-width path. Speculated bug in
// terminal_rendering_followups item #6: WriteWrapped's
// strings.ReplaceAll("\n","\r\n") was thought to double pre-CRLF
// input. The current code path is correct because WrapText strips
// bare CR (wrap.go:49-52); this test ensures a future change that
// preserves CR fails loud here.
func TestSessionWriteWrapped_PreCRLFAtNonZeroWidth(t *testing.T) {
	s, peer := newPipeSession(t)
	s.Width = 80

	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		n, _ := peer.Read(buf)
		got <- string(buf[:n])
	}()

	if err := s.WriteWrapped("line one\r\nline two"); err != nil {
		t.Fatalf("WriteWrapped: %v", err)
	}
	out := <-got
	if strings.Contains(out, "\r\r\n") {
		t.Fatalf("CRLF doubled at non-zero width: %q", out)
	}
	if out != "line one\r\nline two" {
		t.Fatalf("got %q, want %q", out, "line one\r\nline two")
	}
}

// TestSessionWritePagedWrapped_PreCRLFAtNonZeroWidth covers the
// second code path that shares the suspect ReplaceAll line
// (session.go::WritePagedWrapped). Height=0 means the pager doesn't
// push so we can read the raw wire bytes.
func TestSessionWritePagedWrapped_PreCRLFAtNonZeroWidth(t *testing.T) {
	s, peer := newPipeSession(t)
	s.Width = 80
	s.Height = 0 // skip pager so the bytes land on the wire directly

	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		n, _ := peer.Read(buf)
		got <- string(buf[:n])
	}()

	if err := s.WritePagedWrapped("line one\r\nline two"); err != nil {
		t.Fatalf("WritePagedWrapped: %v", err)
	}
	out := <-got
	if strings.Contains(out, "\r\r\n") {
		t.Fatalf("CRLF doubled: %q", out)
	}
	if out != "line one\r\nline two" {
		t.Fatalf("got %q, want %q", out, "line one\r\nline two")
	}
}

// TestSessionWriteWrapped_PreCRLFThroughActualWrap exercises the
// interaction case: a body that BOTH contains a pre-CRLF break AND
// hits a width-driven wrap break, so the input's `\r\n` and
// WrapText's own `\n` end up in the same output buffer feeding
// ReplaceAll. Every line break in the output must be exactly
// `\r\n` — no doubled CR, no bare LF.
func TestSessionWriteWrapped_PreCRLFThroughActualWrap(t *testing.T) {
	s, peer := newPipeSession(t)
	s.Width = 12

	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		n, _ := peer.Read(buf)
		got <- string(buf[:n])
	}()

	// "alpha" is one input line (CRLF), then a longer line that
	// WrapText itself splits into "bravo charlie" + "delta" at
	// width 12.
	if err := s.WriteWrapped("alpha\r\nbravo charlie delta"); err != nil {
		t.Fatalf("WriteWrapped: %v", err)
	}
	out := <-got

	if strings.Contains(out, "\r\r\n") {
		t.Fatalf("CRLF doubled: %q", out)
	}
	// Walk the bytes: every `\r` must be immediately followed by
	// `\n`, and every `\n` must be immediately preceded by `\r`.
	// Catches stray bare LFs slipping through the ReplaceAll.
	for i := 0; i < len(out); i++ {
		if out[i] == '\r' {
			if i+1 >= len(out) || out[i+1] != '\n' {
				t.Fatalf("bare CR at byte %d: %q", i, out)
			}
		}
		if out[i] == '\n' {
			if i == 0 || out[i-1] != '\r' {
				t.Fatalf("bare LF at byte %d: %q", i, out)
			}
		}
	}
}
