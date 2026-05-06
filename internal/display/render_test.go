package display_test

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/telnet"
)

// renderHarness pairs a *telnet.Session with a goroutine draining its
// peer connection, mirroring internal/mode/chargen_render_test.go's
// helper. Tests that need to observe write output use this rather
// than reaching for net.Pipe directly.
type safeBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *safeBuf) write(p []byte) { s.mu.Lock(); s.b.Write(p); s.mu.Unlock() }
func (s *safeBuf) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func renderHarness(t *testing.T, width, color int) (*telnet.Session, *safeBuf, func()) {
	t.Helper()
	server, client := net.Pipe()
	s := telnet.NewSession(server)
	s.Width = width
	s.ColorLevel = color

	buf := &safeBuf{}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		b := make([]byte, 4096)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = client.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			n, err := client.Read(b)
			if n > 0 {
				buf.write(b[:n])
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
	return s, buf, cleanup
}

func flush() { time.Sleep(75 * time.Millisecond) }

func TestRuleWidth_ClampsToBounds(t *testing.T) {
	tests := []struct {
		w    int
		want int
	}{
		{0, display.RuleMinWidth},
		{20, 20}, // narrow but positive widths pass through
		{60, 60},
		{200, display.RuleMaxWidth},
	}
	for _, tc := range tests {
		s, _, done := renderHarness(t, tc.w, telnet.ColorLevelNone)
		if got := display.RuleWidth(s); got != tc.want {
			t.Errorf("RuleWidth(w=%d) = %d, want %d", tc.w, got, tc.want)
		}
		done()
	}
}

func TestRuleWidth_NilSession(t *testing.T) {
	if got := display.RuleWidth(nil); got != display.RuleMinWidth {
		t.Errorf("RuleWidth(nil) = %d, want %d", got, display.RuleMinWidth)
	}
}

func TestSectionHeader_PayloadAndWidth(t *testing.T) {
	s, buf, done := renderHarness(t, 60, telnet.ColorLevel16)
	defer done()
	if err := display.SectionHeader(s, "Score"); err != nil {
		t.Fatalf("SectionHeader: %v", err)
	}
	flush()
	if !strings.Contains(buf.String(), "Score") {
		t.Fatalf("missing payload: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "─── ") {
		t.Fatalf("missing rule lead: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "\x1b[") {
		t.Fatalf("expected SGR on color session: %q", buf.String())
	}
}

func TestSectionHeader_StripsOnColorLevelNone(t *testing.T) {
	s, buf, done := renderHarness(t, 60, telnet.ColorLevelNone)
	defer done()
	if err := display.SectionHeader(s, "Score"); err != nil {
		t.Fatalf("SectionHeader: %v", err)
	}
	flush()
	if strings.ContainsRune(buf.String(), 0x1b) {
		t.Fatalf("ColorLevelNone leaked SGR: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "Score") {
		t.Fatalf("missing payload: %q", buf.String())
	}
}

func TestFieldRow_DefangsValueInjection(t *testing.T) {
	s, buf, done := renderHarness(t, 60, telnet.ColorLevelNone)
	defer done()
	hostile := "Lan}}::red OWNED"
	if err := display.FieldRow(s, "Name", hostile, 14); err != nil {
		t.Fatalf("FieldRow: %v", err)
	}
	flush()
	out := buf.String()
	if strings.Contains(out, "}}::red") {
		t.Fatalf("defang failed, raw }}:: leaked: %q", out)
	}
	if !strings.Contains(out, "Name:") {
		t.Fatalf("missing label: %q", out)
	}
}

func TestErrorAndOK_Glyphs(t *testing.T) {
	s, buf, done := renderHarness(t, 60, telnet.ColorLevel16)
	defer done()
	if err := display.Error(s, "bad"); err != nil {
		t.Fatalf("Error: %v", err)
	}
	if err := display.OK(s, "ok"); err != nil {
		t.Fatalf("OK: %v", err)
	}
	flush()
	out := buf.String()
	if !strings.Contains(out, ">> ") {
		t.Fatalf("missing >> glyph: %q", out)
	}
	if !strings.Contains(out, "✓ ") {
		t.Fatalf("missing ✓ glyph: %q", out)
	}
}
