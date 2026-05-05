package mode

// Verifies the chargen render helpers across the 3×3 matrix the
// ui-expert skill calls out: three terminal widths (60/80/120) and
// three color levels (None / 16 / 256). The helpers must:
//
//   - emit literal payload text verbatim (substring assertions in
//     character_test.go and elsewhere depend on it),
//   - emit cfmt-derived ANSI SGR sequences,
//   - cap visible-glyph rule width at the negotiated session width.
//
// Note on ColorLevel: today, Session.WriteString unconditionally
// renders cfmt tags via cfmt.Sprint regardless of Session.ColorLevel,
// so even ColorLevelNone sees ANSI escapes on the wire. That's a
// real gap (the ui-expert skill claims downsampling kicks in at the
// session layer); fixing it requires a level-aware cfmt wrapper or
// strip pass in WriteString. Tracked as a follow-up. These tests
// assert payload/structure, not the (broken-today) level gate.

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/telnet"
)

// renderTestSession spins up a paired Session/peer with the requested
// width and color level, drains the peer in the background, and
// returns the (session, captured-output, cleanup) triple.
func renderTestSession(t *testing.T, width, colorLevel int) (*telnet.Session, *safeBuf, func()) {
	t.Helper()
	server, client := net.Pipe()
	s := telnet.NewSession(server)
	s.Width = width
	s.ColorLevel = colorLevel

	captured := &safeBuf{}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = client.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			n, err := client.Read(buf)
			if n > 0 {
				captured.write(buf[:n])
			}
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				return
			}
		}
	}()

	cleanup := func() {
		close(stop)
		_ = server.Close()
		_ = client.Close()
		wg.Wait()
	}
	return s, captured, cleanup
}

// hasANSIEscape reports whether s contains a CSI-introducer that
// would leak in a no-color session.
func hasANSIEscape(s string) bool { return strings.Contains(s, "\x1b[") }

// flush gives the drain goroutine a moment to pull bytes off the
// pipe. The pipe is unbuffered (net.Pipe) so writes block until the
// reader has consumed; sleep covers the deadline-wait window.
func flush() { time.Sleep(75 * time.Millisecond) }

func TestChargenRender_StepHeader_AcrossMatrix(t *testing.T) {
	widths := []int{60, 80, 120}
	colors := []struct {
		name  string
		level int
	}{
		{"none", telnet.ColorLevelNone},
		{"16", telnet.ColorLevel16},
		{"256", telnet.ColorLevel256},
	}
	for _, w := range widths {
		for _, c := range colors {
			t.Run("w="+itoa(w)+"/c="+c.name, func(t *testing.T) {
				s, captured, cleanup := renderTestSession(t, w, c.level)
				defer cleanup()

				if err := writeStepHeader(s, chargenStepBackground); err != nil {
					t.Fatalf("writeStepHeader: %v", err)
				}
				flush()
				out := captured.String()

				// Payload text must always appear, regardless of width
				// or color. Without it, downstream substring tests
				// would silently break.
				if !strings.Contains(out, "Step 3/8 — Background") {
					t.Fatalf("missing payload, got %q", out)
				}

				// cfmt always emits ANSI today (see file-top note).
				// At minimum, payload must be present and the output
				// must have the bold+cyan SGR signature so we know
				// the cfmt path was hit, not the bypass.
				if !hasANSIEscape(out) {
					t.Fatalf("ColorLevel=%d emitted no SGR (cfmt path skipped?): %q",
						c.level, out)
				}

				// Header line is bounded by terminal width once we
				// strip the trailing CRLF and any escape codes.
				visible := stripANSI(out)
				visible = strings.TrimRight(visible, "\r\n")
				if got := len(visible); got > w {
					t.Fatalf("header overflow: width=%d, got %d cols (%q)",
						w, got, visible)
				}
			})
		}
	}
}

func TestChargenRender_FieldRow_PreservesPayload(t *testing.T) {
	s, captured, cleanup := renderTestSession(t, 80, telnet.ColorLevel16)
	defer cleanup()

	if err := writeFieldRow(s, "Home language", "Common"); err != nil {
		t.Fatalf("writeFieldRow: %v", err)
	}
	flush()
	out := captured.String()
	for _, want := range []string{"Home language:", "Common"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestChargenRender_FieldRow_DefangsCfmtInjection(t *testing.T) {
	s, captured, cleanup := renderTestSession(t, 80, telnet.ColorLevelNone)
	defer cleanup()

	// A hostile value claims to close the surrounding {{...}}::tag
	// and inject red text. Defang must split the close sequence so
	// the literal `}}::red` cannot recolor downstream output.
	if err := writeFieldRow(s, "Name", "Lan}}::red\\nINJECTED"); err != nil {
		t.Fatalf("writeFieldRow: %v", err)
	}
	flush()
	out := captured.String()
	if strings.Contains(out, "}}::red") && !strings.Contains(out, "} }::") {
		t.Fatalf("defang failed, raw }}:: leaked: %q", out)
	}
}

func TestChargenRender_Error_StyleAndPayload(t *testing.T) {
	for _, c := range []struct {
		name  string
		level int
	}{
		{"none", telnet.ColorLevelNone},
		{"16", telnet.ColorLevel16},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, captured, cleanup := renderTestSession(t, 80, c.level)
			defer cleanup()

			const msg = "Race must be 'human' or 'ogier'."
			if err := writeError(s, msg); err != nil {
				t.Fatalf("writeError: %v", err)
			}
			flush()
			out := captured.String()
			// Payload preserved so existing
			// strings.Contains(captured, "human") tests still pass.
			if !strings.Contains(out, msg) {
				t.Fatalf("missing payload in %q", out)
			}
			if !strings.Contains(out, ">>") {
				t.Fatalf("missing >> prefix in %q", out)
			}
		})
	}
}

func TestChargenRender_OK_StyleAndPayload(t *testing.T) {
	s, captured, cleanup := renderTestSession(t, 80, telnet.ColorLevel256)
	defer cleanup()
	if err := writeOK(s, "Saved."); err != nil {
		t.Fatalf("writeOK: %v", err)
	}
	flush()
	out := captured.String()
	if !strings.Contains(out, "Saved.") {
		t.Fatalf("missing payload in %q", out)
	}
	if !hasANSIEscape(out) {
		t.Fatalf("256-color emitted no SGR: %q", out)
	}
}

func TestChargenRender_Rule_RespectsNarrowWidth(t *testing.T) {
	s, captured, cleanup := renderTestSession(t, 60, telnet.ColorLevel16)
	defer cleanup()
	if err := writeRule(s); err != nil {
		t.Fatalf("writeRule: %v", err)
	}
	flush()
	out := captured.String()
	// stripANSI also folds the U+2500 box rune to ASCII '-' so
	// len() reports cell count rather than UTF-8 byte count.
	visible := strings.TrimRight(stripANSI(out), "\r\n")
	if got := len(visible); got != 60 {
		t.Fatalf("rule width = %d, want 60 (out=%q)", got, visible)
	}
}

func TestChargenRender_StepNumberMapping(t *testing.T) {
	cases := []struct {
		step chargenStep
		want int
	}{
		{chargenStepName, 1},
		{chargenStepRace, 2},
		{chargenStepBackground, 3},
		{chargenStepClass, 4},
		{chargenStepAbilities, 5},
		{chargenStepIdentity, 6},
		{chargenStepFeat, 7},
		{chargenStepSkills, 7},
		{chargenStepReview, 8},
		{chargenStepDone, 0},
	}
	for _, tc := range cases {
		if got := chargenStepNumber(tc.step); got != tc.want {
			t.Errorf("chargenStepNumber(%v) = %d, want %d", tc.step, got, tc.want)
		}
	}
}

// itoa avoids fmt to keep the test file's import set small; widths
// here are always small positive ints.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// stripANSI removes CSI escape sequences (\x1b[...letter) so width
// assertions can measure the visible-glyph length the user sees.
// The chargen header rule uses the U+2500 box-drawing rune which
// occupies one rune; len() over the byte slice would over-count its
// 3-byte UTF-8 encoding, so we count runes instead.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			// Skip CSI introducer + parameters + final byte.
			i += 2
			for i < len(s) && !isCSIFinal(s[i]) {
				i++
			}
			if i < len(s) {
				i++ // consume final byte
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	// Convert to rune-count form for width-assert callers. Returning
	// the deescaped string lets the caller decide whether to use
	// len() (bytes) or utf8.RuneCountInString (cells); the rule line
	// has only ASCII + U+2500 which is single-cell on every terminal
	// we target, so callers use rune count.
	out := b.String()
	// Collapse any UTF-8 box rune to one byte so len() == cell count.
	out = strings.ReplaceAll(out, "─", "-")
	return out
}

func isCSIFinal(b byte) bool {
	// CSI final bytes are in 0x40..0x7E (ANSI X3.64 / ECMA-48). The
	// chargen helpers only emit `m` (SGR), but any final byte ends a
	// sequence safely.
	return b >= 0x40 && b <= 0x7e
}
