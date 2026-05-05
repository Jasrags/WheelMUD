package telnet

import (
	"bytes"
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// pagerTestSession wires a Session to an in-process net.Pipe and
// drains the client side into a thread-safe buffer in the
// background. Returns the session, the captured-output snapshot
// fn, and a cleanup. Mirrors the spirit of newPipeSession but with
// the read side automated.
func pagerTestSession(t *testing.T) (*Session, func() string) {
	t.Helper()
	server, client := net.Pipe()
	s := NewSession(server)

	var (
		mu  sync.Mutex
		buf bytes.Buffer
	)
	doneRead := make(chan struct{})
	go func() {
		defer close(doneRead)
		tmp := make([]byte, 1024)
		for {
			n, err := client.Read(tmp)
			if n > 0 {
				mu.Lock()
				buf.Write(tmp[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		server.Close()
		client.Close()
		<-doneRead
	})

	snapshot := func() string {
		// Brief settle for the reader goroutine. Pipe writes are
		// synchronous so by the time WriteRaw returns the bytes have
		// been read, but Go scheduling can leave them sitting in the
		// reader's local var briefly. A 5ms yield is generous.
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		return buf.String()
	}
	return s, snapshot
}

func TestSplitCRLFLines(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"abc\r\n", []string{"abc\r\n"}},
		{"a\r\nb\r\n", []string{"a\r\n", "b\r\n"}},
		{"a\r\nb", []string{"a\r\n", "b"}}, // no trailing CRLF on last
		{"\r\n", []string{"\r\n"}},
		{"\r\n\r\n", []string{"\r\n", "\r\n"}},
	}
	for _, tt := range tests {
		got := splitCRLFLines([]byte(tt.in))
		if len(got) != len(tt.want) {
			t.Errorf("split %q: len = %d, want %d (%v)", tt.in, len(got), len(tt.want), got)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("split %q [%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
			}
		}
	}
}

// makeBody returns n CRLF-terminated lines numbered 1..n.
func makeBody(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("line")
		b.WriteString(strconv.Itoa(i))
		b.WriteString("\r\n")
	}
	return b.String()
}

func TestWritePaged_ShortBodyWritesDirect(t *testing.T) {
	s, out := pagerTestSession(t)
	s.Height = 24
	body := makeBody(5)
	if err := s.WritePaged([]byte(body)); err != nil {
		t.Fatalf("WritePaged: %v", err)
	}
	if got := out(); got != body {
		t.Errorf("got %q\nwant %q", got, body)
	}
	if s.CurrentMode() != nil {
		t.Errorf("expected no mode pushed for short body")
	}
}

func TestWritePaged_HeightZeroFallsThrough(t *testing.T) {
	s, out := pagerTestSession(t)
	s.Height = 0
	body := makeBody(100)
	if err := s.WritePaged([]byte(body)); err != nil {
		t.Fatalf("WritePaged: %v", err)
	}
	if got := out(); got != body {
		t.Errorf("got len=%d want full body len=%d", len(got), len(body))
	}
	if s.CurrentMode() != nil {
		t.Errorf("Height=0 should not push pager")
	}
}

func TestWritePaged_LongBodyPushesPager(t *testing.T) {
	s, out := pagerTestSession(t)
	s.Height = 10
	body := makeBody(25)
	if err := s.WritePaged([]byte(body)); err != nil {
		t.Fatalf("WritePaged: %v", err)
	}
	got := out()
	// First page: Height-1 = 9 lines.
	wantPrefix := strings.Join(strings.SplitAfter(body, "\r\n")[:9], "")
	if got != wantPrefix {
		t.Errorf("first page mismatch:\n got %q\nwant %q", got, wantPrefix)
	}
	mode := s.CurrentMode()
	if mode == nil {
		t.Fatal("expected pager pushed")
	}
	if _, ok := mode.(*pagerMode); !ok {
		t.Errorf("top mode is %T, want *pagerMode", mode)
	}
	if p := mode.Prompt(context.Background(), s); p != pagerMoreLine {
		t.Errorf("Prompt = %q, want %q", p, pagerMoreLine)
	}
}

func TestPager_SpaceAdvancesPage(t *testing.T) {
	s, out := pagerTestSession(t)
	s.Height = 10
	body := makeBody(25)
	if err := s.WritePaged([]byte(body)); err != nil {
		t.Fatalf("WritePaged: %v", err)
	}
	mode := s.CurrentMode().(*pagerMode)
	// Drive one page advance.
	if err := mode.Handle(context.Background(), s, ""); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	got := out()
	// Now should have 18 lines on the wire (two 9-line pages).
	wantLines := strings.Join(strings.SplitAfter(body, "\r\n")[:18], "")
	if got != wantLines {
		t.Errorf("after 1 advance:\n got %q\nwant %q", got, wantLines)
	}
	if s.CurrentMode() == nil {
		t.Errorf("pager should still be active (25 lines, 2 pages = 18 written, 7 remaining)")
	}
}

func TestPager_QuitPopsModeAndDiscardsRemainder(t *testing.T) {
	s, out := pagerTestSession(t)
	s.Height = 10
	body := makeBody(25)
	if err := s.WritePaged([]byte(body)); err != nil {
		t.Fatalf("WritePaged: %v", err)
	}
	mode := s.CurrentMode().(*pagerMode)
	if err := mode.Handle(context.Background(), s, "q"); err != nil {
		t.Fatalf("Handle q: %v", err)
	}
	if s.CurrentMode() != nil {
		t.Errorf("expected pager popped on q")
	}
	// Output should be exactly the first page (9 lines), nothing more.
	got := out()
	wantLines := strings.Join(strings.SplitAfter(body, "\r\n")[:9], "")
	if got != wantLines {
		t.Errorf("q should not advance:\n got %q\nwant %q", got, wantLines)
	}
}

func TestPager_FinalPageAutoPops(t *testing.T) {
	s, _ := pagerTestSession(t)
	s.Height = 10 // page = 9
	body := makeBody(15)
	if err := s.WritePaged([]byte(body)); err != nil {
		t.Fatalf("WritePaged: %v", err)
	}
	if s.CurrentMode() == nil {
		t.Fatal("expected pager pushed")
	}
	mode := s.CurrentMode().(*pagerMode)
	// One advance writes the remaining 6 lines and should auto-pop.
	if err := mode.Handle(context.Background(), s, ""); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if s.CurrentMode() != nil {
		t.Errorf("expected pager auto-popped after final page; top is %T", s.CurrentMode())
	}
}

func TestPager_PromptEmptyWhenDrained(t *testing.T) {
	// Defensive: even if Handle hadn't auto-popped, Prompt should
	// return "" once remaining is empty.
	p := newPagerMode(nil, 10)
	if got := p.Prompt(context.Background(), nil); got != "" {
		t.Errorf("Prompt drained = %q, want empty", got)
	}
}

// Ensure the pager doesn't trip the io.EOF / closed-pipe path during
// teardown. Smoke test for the t.Cleanup race.
func TestPagerTestSession_Smoke(t *testing.T) {
	s, _ := pagerTestSession(t)
	if err := s.WriteRaw([]byte("hi\r\n")); err != nil && err != io.EOF {
		t.Errorf("WriteRaw: %v", err)
	}
}
