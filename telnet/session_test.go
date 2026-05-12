package telnet

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// failingEnterMode returns errSentinelEnter from OnEnter and records whether
// OnExit ran (it should NOT, since the push was rolled back).
type failingEnterMode struct {
	exitCalled bool
}

var errSentinelEnter = errors.New("on-enter failed")

func (m *failingEnterMode) Handle(_ context.Context, _ *Session, _ string) error { return nil }
func (m *failingEnterMode) Prompt(_ context.Context, _ *Session) string          { return "" }
func (m *failingEnterMode) OnEnter(_ *Session) error                             { return errSentinelEnter }
func (m *failingEnterMode) OnExit(_ *Session) error                              { m.exitCalled = true; return nil }

func TestPushMode_RollsBackOnOnEnterError(t *testing.T) {
	s, _ := newPipeSession(t)
	base := &scriptedMode{}
	if err := s.PushMode(base); err != nil {
		t.Fatalf("seed push: %v", err)
	}

	failing := &failingEnterMode{}
	err := s.PushMode(failing)
	if !errors.Is(err, errSentinelEnter) {
		t.Fatalf("PushMode err = %v, want errSentinelEnter", err)
	}
	if got := s.CurrentMode(); got != base {
		t.Fatalf("CurrentMode = %v, want base scripted mode (failing mode leaked onto stack)", got)
	}
	if failing.exitCalled {
		t.Fatal("OnExit was called for a mode whose push failed")
	}
}

func TestReplaceMode_RollsBackOnOnEnterError(t *testing.T) {
	s, _ := newPipeSession(t)
	base := &scriptedMode{}
	if err := s.PushMode(base); err != nil {
		t.Fatalf("seed push: %v", err)
	}

	failing := &failingEnterMode{}
	err := s.ReplaceMode(failing)
	if !errors.Is(err, errSentinelEnter) {
		t.Fatalf("ReplaceMode err = %v, want errSentinelEnter", err)
	}
	// ReplaceMode pops the prior mode before pushing; on OnEnter failure the
	// stack ends up empty, not retaining either mode.
	if got := s.CurrentMode(); got != nil {
		t.Fatalf("CurrentMode = %v, want nil (failing mode must not leak)", got)
	}
	if failing.exitCalled {
		t.Fatal("OnExit was called for a mode whose push failed")
	}
}

// drainOnce reads from peer for up to 200ms, returning whatever was
// captured. The pipe is unbuffered so a write blocks until a read; we
// read a single chunk then stop.
func drainOnce(t *testing.T, peer io.Reader) []byte {
	t.Helper()
	buf := make([]byte, 4096)
	if c, ok := peer.(interface {
		SetReadDeadline(time.Time) error
	}); ok {
		_ = c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	}
	n, _ := peer.Read(buf)
	return buf[:n]
}

func TestWriteString_StripsANSI_OnColorLevelNone(t *testing.T) {
	tests := []struct {
		name       string
		level      int
		wantHasEsc bool
	}{
		{"none", ColorLevelNone, false},
		{"16", ColorLevel16, true},
		{"256", ColorLevel256, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, peer := newPipeSession(t)
			s.ColorLevel = tc.level

			done := make(chan []byte, 1)
			go func() { done <- drainOnce(t, peer) }()

			if err := s.WriteString("{{hello}}::red|bold"); err != nil {
				t.Fatalf("WriteString: %v", err)
			}
			out := <-done

			if !bytes.Contains(out, []byte("hello")) {
				t.Fatalf("missing payload: %q", out)
			}
			hasEsc := bytes.Contains(out, []byte{0x1b})
			if hasEsc != tc.wantHasEsc {
				t.Fatalf("level=%d hasEsc=%v, want %v (%q)", tc.level, hasEsc, tc.wantHasEsc, out)
			}
		})
	}
}

func TestWriteWrapped_StripsANSI_OnColorLevelNone(t *testing.T) {
	s, peer := newPipeSession(t)
	s.ColorLevel = ColorLevelNone
	s.Width = 40

	done := make(chan []byte, 1)
	go func() { done <- drainOnce(t, peer) }()

	if err := s.WriteWrapped("{{this is a fairly long line of red text that should wrap at forty}}::red"); err != nil {
		t.Fatalf("WriteWrapped: %v", err)
	}
	out := <-done

	if bytes.Contains(out, []byte{0x1b}) {
		t.Fatalf("ColorLevelNone leaked SGR: %q", out)
	}
	if !strings.Contains(string(out), "fairly long line") {
		t.Fatalf("missing payload: %q", out)
	}
}
