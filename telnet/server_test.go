package telnet

import (
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// scriptedMode collects every line it sees and writes a fixed prompt.
type scriptedMode struct {
	mu     sync.Mutex
	lines  []string
	closed atomic.Bool
}

func (m *scriptedMode) Handle(s *Session, line string) error {
	m.mu.Lock()
	m.lines = append(m.lines, line)
	m.mu.Unlock()
	return s.WriteRaw([]byte("ack:" + line + "\r\n"))
}
func (m *scriptedMode) Prompt(_ *Session) string { return "> " }
func (m *scriptedMode) OnEnter(_ *Session) error { return nil }
func (m *scriptedMode) OnExit(_ *Session) error  { m.closed.Store(true); return nil }

func (m *scriptedMode) snapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.lines))
	copy(out, m.lines)
	return out
}

func TestRunSession_DispatchesLines(t *testing.T) {
	s, peer := newPipeSession(t)
	mode := &scriptedMode{}
	if err := s.PushMode(mode); err != nil {
		t.Fatalf("push mode: %v", err)
	}

	// Drain whatever RunSession writes (negotiation + acks).
	var captured strings.Builder
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			_ = peer.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			n, err := peer.Read(buf)
			if n > 0 {
				captured.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	done := make(chan error, 1)
	go func() { done <- RunSession(s) }()

	// Send two lines. The server treats CR or LF as a line terminator.
	if _, err := peer.Write([]byte("hello\r\nworld\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Give the dispatcher a moment to process.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(mode.snapshot()) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	got := mode.snapshot()
	if len(got) != 2 || got[0] != "hello" || got[1] != "world" {
		t.Fatalf("dispatched lines = %v, want [hello world]", got)
	}

	// Tear down: closing the peer EOFs the read loop.
	_ = peer.Close()
	select {
	case err := <-done:
		if err != nil && err != io.EOF {
			t.Fatalf("RunSession err: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunSession did not return after peer close")
	}
	wg.Wait()

	if !strings.Contains(captured.String(), "ack:hello") {
		t.Fatalf("expected ack:hello in output, got %q", captured.String())
	}
}

func TestRunSession_InputFlooded(t *testing.T) {
	s, peer := newPipeSession(t)

	// Mode that blocks until released so the inbox fills up.
	blocker := &blockingMode{released: make(chan struct{})}
	if err := s.PushMode(blocker); err != nil {
		t.Fatalf("push mode: %v", err)
	}

	go func() {
		buf := make([]byte, 1024)
		for {
			_ = peer.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			if _, err := peer.Read(buf); err != nil {
				return
			}
		}
	}()

	done := make(chan error, 1)
	go func() { done <- RunSession(s) }()

	// Pump well past inboxCap to guarantee a flooded send.
	flood := strings.Repeat("x\r\n", inboxCap*4)
	_, _ = peer.Write([]byte(flood))

	// Give readLoop time to detect the flood and exit, then release the
	// dispatcher so RunSession can finish its drain wait.
	time.Sleep(200 * time.Millisecond)
	close(blocker.released)

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "flooded") {
			t.Fatalf("expected ErrInputFlooded, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunSession did not detect flood")
	}
}

type blockingMode struct {
	released chan struct{}
}

func (b *blockingMode) Handle(_ *Session, _ string) error { <-b.released; return nil }
func (b *blockingMode) Prompt(_ *Session) string          { return "" }
func (b *blockingMode) OnEnter(_ *Session) error          { return nil }
func (b *blockingMode) OnExit(_ *Session) error           { return nil }

// terminalMode signals end-of-session from Handle without writing a prompt.
type terminalMode struct{ closed bool }

func (m *terminalMode) Handle(s *Session, _ string) error {
	m.closed = true
	_ = s.Conn.Close()
	return ErrSessionEnded
}
func (m *terminalMode) Prompt(_ *Session) string { return "should-not-write> " }
func (m *terminalMode) OnEnter(_ *Session) error { return nil }
func (m *terminalMode) OnExit(_ *Session) error  { return nil }

func TestRunSession_ErrSessionEndedStopsDispatcher(t *testing.T) {
	s, peer := newPipeSession(t)
	mode := &terminalMode{}
	if err := s.PushMode(mode); err != nil {
		t.Fatalf("push: %v", err)
	}

	var captured strings.Builder
	var capturedMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			_ = peer.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			n, err := peer.Read(buf)
			if n > 0 {
				capturedMu.Lock()
				captured.Write(buf[:n])
				capturedMu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	done := make(chan error, 1)
	go func() { done <- RunSession(s) }()

	if _, err := peer.Write([]byte("anything\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunSession did not return after ErrSessionEnded")
	}
	wg.Wait()

	capturedMu.Lock()
	got := captured.String()
	capturedMu.Unlock()
	if strings.Contains(got, "should-not-write>") {
		t.Fatalf("dispatcher wrote prompt after ErrSessionEnded: %q", got)
	}
}

func TestRunSession_TabCompletes(t *testing.T) {
	s, peer := newPipeSession(t)
	mode := &completingMode{candidates: []Candidate{{Text: "quit"}}}
	if err := s.PushMode(mode); err != nil {
		t.Fatalf("push mode: %v", err)
	}

	var captured strings.Builder
	var capturedMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			_ = peer.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			n, err := peer.Read(buf)
			if n > 0 {
				capturedMu.Lock()
				captured.Write(buf[:n])
				capturedMu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	done := make(chan error, 1)
	go func() { done <- RunSession(s) }()

	// Type "q" then press Tab.
	if _, err := peer.Write([]byte("q\t")); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Wait for the wire to show the completion. Don't poll InputBuffer
	// directly — that's owned by the read goroutine.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		capturedMu.Lock()
		got := captured.String()
		capturedMu.Unlock()
		if strings.Contains(got, "quit ") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	_ = peer.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunSession did not return")
	}
	wg.Wait()

	capturedMu.Lock()
	final := captured.String()
	capturedMu.Unlock()
	if !strings.Contains(final, "quit ") {
		t.Fatalf("expected 'quit ' on the wire, got %q", final)
	}
	if !strings.Contains(final, "\b \b") {
		t.Fatalf("expected backspace erase on the wire, got %q", final)
	}
}
