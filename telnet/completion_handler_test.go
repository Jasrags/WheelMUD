package telnet

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// completingMode is a Mode that completes against a fixed candidate list.
type completingMode struct {
	candidates []Candidate
	prompt     string
}

func (m *completingMode) Handle(_ context.Context, _ *Session, _ string) error { return nil }
func (m *completingMode) Prompt(_ *Session) string {
	if m.prompt == "" {
		return "> "
	}
	return m.prompt
}
func (m *completingMode) OnEnter(_ *Session) error { return nil }
func (m *completingMode) OnExit(_ *Session) error  { return nil }
func (m *completingMode) Complete(_ *Session, buffer string) []Candidate {
	if strings.ContainsAny(buffer, " \t") {
		return nil
	}
	var out []Candidate
	for _, c := range m.candidates {
		if strings.HasPrefix(c.Text, buffer) {
			out = append(out, c)
		}
	}
	return out
}

// quietMode does not implement Completer.
type quietMode struct{}

func (m *quietMode) Handle(_ context.Context, _ *Session, _ string) error { return nil }
func (m *quietMode) Prompt(_ *Session) string                             { return "> " }
func (m *quietMode) OnEnter(_ *Session) error                             { return nil }
func (m *quietMode) OnExit(_ *Session) error                              { return nil }

// drainPeer reads everything from peer until it returns an error. Returns
// the full byte stream captured up to that point.
func drainPeer(peer net.Conn, wg *sync.WaitGroup) *strings.Builder {
	var buf strings.Builder
	var mu sync.Mutex
	wg.Add(1)
	go func() {
		defer wg.Done()
		b := make([]byte, 1024)
		for {
			_ = peer.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
			n, err := peer.Read(b)
			if n > 0 {
				mu.Lock()
				buf.Write(b[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	return &buf
}

func TestHandleTab_NoMode(t *testing.T) {
	s, peer := newPipeSession(t)

	var wg sync.WaitGroup
	out := drainPeer(peer, &wg)
	defer wg.Wait()

	if err := handleTab(s); err != nil {
		t.Fatalf("handleTab: %v", err)
	}
	peer.Close()
	wg.Wait()

	if !strings.Contains(out.String(), "\x07") {
		t.Fatalf("expected bell when no mode, got %q", out.String())
	}
}

func TestHandleTab_ModeWithoutCompleter(t *testing.T) {
	s, peer := newPipeSession(t)
	if err := s.PushMode(&quietMode{}); err != nil {
		t.Fatalf("push: %v", err)
	}

	var wg sync.WaitGroup
	out := drainPeer(peer, &wg)
	defer wg.Wait()

	if err := handleTab(s); err != nil {
		t.Fatalf("handleTab: %v", err)
	}
	peer.Close()
	wg.Wait()

	if !strings.Contains(out.String(), "\x07") {
		t.Fatalf("expected bell, got %q", out.String())
	}
}

func TestHandleTab_PasswordMode(t *testing.T) {
	s, peer := newPipeSession(t)
	if err := s.PushMode(&completingMode{candidates: []Candidate{{Text: "look"}}}); err != nil {
		t.Fatalf("push: %v", err)
	}
	s.InPasswordMode = true
	s.Input = LineEdit{Buf: []byte("lo"), Cursor: 2}

	var wg sync.WaitGroup
	out := drainPeer(peer, &wg)
	defer wg.Wait()

	if err := handleTab(s); err != nil {
		t.Fatalf("handleTab: %v", err)
	}
	peer.Close()
	wg.Wait()

	if !strings.Contains(out.String(), "\x07") {
		t.Fatalf("expected bell in password mode, got %q", out.String())
	}
	if string(s.Input.Buf) != "lo" {
		t.Fatalf("password mode must not mutate buffer: %q", s.Input.Buf)
	}
}

func TestHandleTab_ZeroCandidates(t *testing.T) {
	s, peer := newPipeSession(t)
	mode := &completingMode{candidates: []Candidate{{Text: "look"}}}
	if err := s.PushMode(mode); err != nil {
		t.Fatalf("push: %v", err)
	}
	s.Input = LineEdit{Buf: []byte("xyz"), Cursor: 3}

	var wg sync.WaitGroup
	out := drainPeer(peer, &wg)
	defer wg.Wait()

	if err := handleTab(s); err != nil {
		t.Fatalf("handleTab: %v", err)
	}
	peer.Close()
	wg.Wait()

	if !strings.Contains(out.String(), "\x07") {
		t.Fatalf("expected bell on zero candidates, got %q", out.String())
	}
	if string(s.Input.Buf) != "xyz" {
		t.Fatalf("buffer should be unchanged: %q", s.Input.Buf)
	}
}

func TestHandleTab_SingleCandidate(t *testing.T) {
	s, peer := newPipeSession(t)
	mode := &completingMode{candidates: []Candidate{{Text: "quit"}}}
	if err := s.PushMode(mode); err != nil {
		t.Fatalf("push: %v", err)
	}
	s.Input = LineEdit{Buf: []byte("q"), Cursor: 1}

	var wg sync.WaitGroup
	out := drainPeer(peer, &wg)
	defer wg.Wait()

	if err := handleTab(s); err != nil {
		t.Fatalf("handleTab: %v", err)
	}
	peer.Close()
	wg.Wait()

	if string(s.Input.Buf) != "quit " {
		t.Fatalf("buffer = %q, want %q", s.Input.Buf, "quit ")
	}
	wire := out.String()
	// One \b \b for the partial 'q', then 'quit '.
	if !strings.Contains(wire, "\b \b") {
		t.Fatalf("expected backspace erase in output: %q", wire)
	}
	if !strings.Contains(wire, "quit ") {
		t.Fatalf("expected 'quit ' in output: %q", wire)
	}
}

func TestHandleTab_ExtendsToCommonPrefix(t *testing.T) {
	s, peer := newPipeSession(t)
	mode := &completingMode{
		candidates: []Candidate{
			{Text: "look"}, {Text: "loot"}, {Text: "loom"},
		},
	}
	if err := s.PushMode(mode); err != nil {
		t.Fatalf("push: %v", err)
	}
	s.Input = LineEdit{Buf: []byte("l"), Cursor: 1}

	var wg sync.WaitGroup
	out := drainPeer(peer, &wg)
	defer wg.Wait()

	if err := handleTab(s); err != nil {
		t.Fatalf("handleTab: %v", err)
	}
	peer.Close()
	wg.Wait()

	if string(s.Input.Buf) != "loo" {
		t.Fatalf("buffer = %q, want %q", s.Input.Buf, "loo")
	}
	if strings.Contains(out.String(), "look") {
		t.Fatalf("listing should not appear when prefix can be extended: %q", out.String())
	}
}

func TestHandleTab_ListsAndRedraws(t *testing.T) {
	s, peer := newPipeSession(t)
	mode := &completingMode{
		candidates: []Candidate{
			{Text: "look", Help: "Examine"},
			{Text: "loot", Help: "Take items"},
		},
	}
	if err := s.PushMode(mode); err != nil {
		t.Fatalf("push: %v", err)
	}
	s.Input = LineEdit{Buf: []byte("loo"), Cursor: 3} // already at the common prefix

	var wg sync.WaitGroup
	out := drainPeer(peer, &wg)
	defer wg.Wait()

	if err := handleTab(s); err != nil {
		t.Fatalf("handleTab: %v", err)
	}
	peer.Close()
	wg.Wait()

	wire := out.String()
	if !strings.Contains(wire, "look") || !strings.Contains(wire, "loot") {
		t.Fatalf("expected both candidates listed: %q", wire)
	}
	if !strings.Contains(wire, "> loo") {
		t.Fatalf("expected redraw of prompt + buffer: %q", wire)
	}
	if string(s.Input.Buf) != "loo" {
		t.Fatalf("buffer must not change on listing: %q", s.Input.Buf)
	}
}

func TestHandleTab_EmptyBufferListsAll(t *testing.T) {
	s, peer := newPipeSession(t)
	cands := make([]Candidate, 0, 12)
	names := []string{"east", "go", "help", "look", "north", "quit", "say", "shout", "sit", "south", "west", "who"}
	for _, n := range names {
		cands = append(cands, Candidate{Text: n})
	}
	mode := &completingMode{candidates: cands}
	if err := s.PushMode(mode); err != nil {
		t.Fatalf("push: %v", err)
	}
	// Empty buffer.

	var wg sync.WaitGroup
	out := drainPeer(peer, &wg)
	defer wg.Wait()

	if err := handleTab(s); err != nil {
		t.Fatalf("handleTab: %v", err)
	}
	peer.Close()
	wg.Wait()

	wire := out.String()
	for _, n := range names {
		if !strings.Contains(wire, n) {
			t.Fatalf("missing %q in listing: %q", n, wire)
		}
	}
}
