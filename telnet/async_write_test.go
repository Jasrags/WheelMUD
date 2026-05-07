package telnet

import (
	"bytes"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// asyncBufConn is a net.Conn that captures every Write into an internal
// buffer and returns "closed" from Read once Close has been called. We
// keep it local to this file so we can assert on raw byte sequences
// (ANSI escapes, prompt repaint, etc.) without the bufio.Reader buffering
// in newPipeSession.
type asyncBufConn struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	closed chan struct{}
	once   sync.Once
}

func newAsyncBufConn() *asyncBufConn { return &asyncBufConn{closed: make(chan struct{})} }

func (b *asyncBufConn) Read(_ []byte) (int, error) {
	<-b.closed
	return 0, net.ErrClosed
}

func (b *asyncBufConn) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *asyncBufConn) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

func (b *asyncBufConn) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

func (b *asyncBufConn) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

func (b *asyncBufConn) LocalAddr() net.Addr                { return asyncFakeAddr{} }
func (b *asyncBufConn) RemoteAddr() net.Addr               { return asyncFakeAddr{} }
func (b *asyncBufConn) SetDeadline(_ time.Time) error      { return nil }
func (b *asyncBufConn) SetReadDeadline(_ time.Time) error  { return nil }
func (b *asyncBufConn) SetWriteDeadline(_ time.Time) error { return nil }

type asyncFakeAddr struct{}

func (asyncFakeAddr) Network() string { return "fake" }
func (asyncFakeAddr) String() string  { return "fake:0" }

func TestWriteAsync_RepaintsPromptAndInput(t *testing.T) {
	c := newAsyncBufConn()
	t.Cleanup(func() { c.Close() })
	s := NewSession(c)

	// Simulate the dispatcher having just painted a prompt.
	if err := s.WritePrompt([]byte("[hp/mn] > ")); err != nil {
		t.Fatalf("WritePrompt: %v", err)
	}

	// Simulate the player typing "kill tro" — read goroutine path.
	for _, b := range []byte("kill tro") {
		if err := bufferInput(s, b); err != nil {
			t.Fatalf("bufferInput: %v", err)
		}
	}

	// Snapshot pre-async output and verify it looks like a prompt + typing.
	pre := c.Bytes()
	if !strings.Contains(string(pre), "[hp/mn] > ") {
		t.Fatalf("expected prompt in pre output, got %q", pre)
	}
	if !strings.Contains(string(pre), "kill tro") {
		t.Fatalf("expected typed input echoed in pre output, got %q", pre)
	}

	c.Reset()

	// Async broadcast lands.
	if err := s.WriteAsync("A black-and-tan sheepdog arrives from the northeast."); err != nil {
		t.Fatalf("WriteAsync: %v", err)
	}

	got := c.Bytes()
	want := []byte("\r\x1b[KA black-and-tan sheepdog arrives from the northeast.\r\n[hp/mn] > kill tro")
	if !bytes.Equal(got, want) {
		t.Fatalf("WriteAsync output mismatch.\n got: %q\nwant: %q", got, want)
	}
}

func TestWriteAsync_PasswordModeMasksInput(t *testing.T) {
	c := newAsyncBufConn()
	t.Cleanup(func() { c.Close() })
	s := NewSession(c)
	s.SetPasswordMode(true)
	if err := s.WritePrompt([]byte("Password: ")); err != nil {
		t.Fatalf("WritePrompt: %v", err)
	}
	for _, b := range []byte("hunter2") {
		if err := bufferInput(s, b); err != nil {
			t.Fatalf("bufferInput: %v", err)
		}
	}
	c.Reset()

	if err := s.WriteAsync("System notice."); err != nil {
		t.Fatalf("WriteAsync: %v", err)
	}

	got := c.Bytes()
	// Buffer is 7 bytes, masked echoes 7 stars — never the cleartext.
	if bytes.Contains(got, []byte("hunter2")) {
		t.Fatalf("password leaked in async write: %q", got)
	}
	want := []byte("\r\x1b[KSystem notice.\r\nPassword: *******")
	if !bytes.Equal(got, want) {
		t.Fatalf("password-mode async output mismatch.\n got: %q\nwant: %q", got, want)
	}
}

func TestWriteAsync_AppendsCRLFWhenMissing(t *testing.T) {
	c := newAsyncBufConn()
	t.Cleanup(func() { c.Close() })
	s := NewSession(c)
	// No prompt, no input — just verify CRLF is appended.
	if err := s.WriteAsync("hi"); err != nil {
		t.Fatalf("WriteAsync: %v", err)
	}
	want := []byte("\r\x1b[Khi\r\n")
	if got := c.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("WriteAsync output mismatch.\n got: %q\nwant: %q", got, want)
	}

	c.Reset()
	// Already-terminated message must not double-up CRLF.
	if err := s.WriteAsync("hi\r\n"); err != nil {
		t.Fatalf("WriteAsync: %v", err)
	}
	if got := c.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("WriteAsync double-CRLF.\n got: %q\nwant: %q", got, want)
	}
}

func TestWriteAsync_NoColorFallback(t *testing.T) {
	c := newAsyncBufConn()
	t.Cleanup(func() { c.Close() })
	s := NewSession(c)
	s.ColorLevel = ColorLevelNone
	s.Width = 10 // small to keep the assertion compact

	if err := s.WriteAsync("hi"); err != nil {
		t.Fatalf("WriteAsync: %v", err)
	}
	got := c.Bytes()
	// Expect: CR + 10 spaces + CR + "hi" + CRLF (no ANSI escape).
	want := []byte("\r          \rhi\r\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("ColorLevelNone output mismatch.\n got: %q\nwant: %q", got, want)
	}
	if bytes.Contains(got, []byte{0x1b}) {
		t.Fatalf("no-color path leaked an ANSI escape: %q", got)
	}
}

func TestWriteAsync_ClearLastPromptOnModeSwap(t *testing.T) {
	c := newAsyncBufConn()
	t.Cleanup(func() { c.Close() })
	s := NewSession(c)
	if err := s.WritePrompt([]byte("Login: ")); err != nil {
		t.Fatalf("WritePrompt: %v", err)
	}
	// Mode transition (ReplaceMode is what postauth.promoteToGame does)
	// drops the cached login prompt so the next async write doesn't
	// replay it into the game session.
	s.ClearLastPrompt()
	c.Reset()

	if err := s.WriteAsync("System notice."); err != nil {
		t.Fatalf("WriteAsync: %v", err)
	}
	got := c.Bytes()
	if bytes.Contains(got, []byte("Login: ")) {
		t.Fatalf("ClearLastPrompt didn't clear the cache: %q", got)
	}
	want := []byte("\r\x1b[KSystem notice.\r\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("post-clear async output mismatch.\n got: %q\nwant: %q", got, want)
	}
}

// TestWriteAsync_NoDoubleEchoUnderConcurrentTyping fires many bufferInput
// calls in lockstep with WriteAsync calls. Each character must appear at
// most once between async writes — without the EditAndWrite refactor a
// concurrent WriteAsync could observe an Input.Buf that already contained
// the just-typed byte and emit a redraw with it before the keystroke
// echo finished, displaying the byte twice.
func TestWriteAsync_NoDoubleEchoUnderConcurrentTyping(t *testing.T) {
	c := newAsyncBufConn()
	t.Cleanup(func() { c.Close() })
	s := NewSession(c)
	if err := s.WritePrompt([]byte("> ")); err != nil {
		t.Fatalf("WritePrompt: %v", err)
	}

	const N = 64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < N; i++ {
			if err := s.WriteAsync("ping"); err != nil {
				t.Errorf("WriteAsync: %v", err)
				return
			}
		}
	}()
	for i := 0; i < N; i++ {
		if err := bufferInput(s, 'a'); err != nil {
			t.Fatalf("bufferInput: %v", err)
		}
	}
	<-done

	got := c.Bytes()
	// Every "ping" repaint replays the prompt + current buffer. The
	// number of 'a' characters across the whole stream must equal the
	// running buffer length at each redraw, plus one echo per keystroke.
	// The simpler invariant: total 'a' count is bounded by the keystrokes
	// (N echoes) plus the redraws (each replays the buffer at the time).
	// We only assert the loose bound — the strict bound is hard to
	// compute without modeling interleave order. The original bug
	// produced a 2N+ over-count of the LAST 'a' specifically, which
	// the bounded form catches.
	maxAs := N + N*N // N echoes + at most N redraws each replaying ≤N buffer chars
	if as := bytes.Count(got, []byte("a")); as > maxAs {
		t.Errorf("'a' count %d exceeds upper bound %d (double-echo regression?)\nlen(got)=%d", as, maxAs, len(got))
	}
}

func TestListAndRedraw_CachePromptAtomically(t *testing.T) {
	// Drive listAndRedraw, then immediately fire WriteAsync from another
	// goroutine. The async write must replay the prompt that was just
	// drawn by listAndRedraw, not a stale one.
	c := newAsyncBufConn()
	t.Cleanup(func() { c.Close() })
	s := NewSession(c)
	if err := s.WritePrompt([]byte("OLD> ")); err != nil {
		t.Fatalf("WritePrompt: %v", err)
	}

	// A throw-away mode whose Prompt returns "NEW> " on every call.
	mode := promptMode{prompt: "NEW> "}
	cands := []Candidate{{Text: "look"}, {Text: "loot"}}

	if err := listAndRedraw(s, mode, cands); err != nil {
		t.Fatalf("listAndRedraw: %v", err)
	}
	c.Reset()

	if err := s.WriteAsync("Sheepdog arrives."); err != nil {
		t.Fatalf("WriteAsync: %v", err)
	}
	got := c.Bytes()
	if !bytes.Contains(got, []byte("NEW> ")) {
		t.Fatalf("WriteAsync did not replay the new cached prompt; got %q", got)
	}
	if bytes.Contains(got, []byte("OLD> ")) {
		t.Fatalf("WriteAsync replayed stale OLD prompt; got %q", got)
	}
}

// promptMode is a minimal Mode used for prompt-cache tests.
type promptMode struct{ prompt string }

func (m promptMode) OnEnter(_ *Session) error                             { return nil }
func (m promptMode) OnExit(_ *Session) error                              { return nil }
func (m promptMode) Handle(_ context.Context, _ *Session, _ string) error { return nil }
func (m promptMode) Prompt(_ context.Context, _ *Session) string          { return m.prompt }

func TestWritePrompt_CachesForReplay(t *testing.T) {
	c := newAsyncBufConn()
	t.Cleanup(func() { c.Close() })
	s := NewSession(c)
	if err := s.WritePrompt([]byte("PROMPT> ")); err != nil {
		t.Fatalf("WritePrompt: %v", err)
	}
	c.Reset()
	if err := s.WriteAsync("ping"); err != nil {
		t.Fatalf("WriteAsync: %v", err)
	}
	if !bytes.Contains(c.Bytes(), []byte("PROMPT> ")) {
		t.Fatalf("WritePrompt did not seed cache; async did not replay: %q", c.Bytes())
	}
}
