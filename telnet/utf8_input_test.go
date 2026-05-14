package telnet

import (
	"bufio"
	"bytes"
	"testing"
)

// drainDispatchBytes feeds bytes one at a time through dispatchByte
// (mirroring the read-loop's behaviour) and returns the collected
// echo bytes from the peer side of the pipe.
func drainDispatchBytes(t *testing.T, s *Session, peer interface {
	Read([]byte) (int, error)
}, in []byte) []byte {
	t.Helper()
	got := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 256)
		acc := []byte{}
		for {
			n, err := peer.Read(buf)
			if n > 0 {
				acc = append(acc, buf[:n]...)
			}
			if err != nil {
				break
			}
			if len(acc) >= 1 && err != nil {
				break
			}
		}
		got <- acc
	}()
	r := bufio.NewReader(bytes.NewReader(in))
	for _, b := range in {
		_, _ = r.ReadByte() // keep the reader interface honest
		if err := dispatchByte(s, r, b); err != nil {
			t.Fatalf("dispatchByte(%#x): %v", b, err)
		}
	}
	s.Conn.Close()
	return <-got
}

func TestDispatcher_CJKLiveInput(t *testing.T) {
	s, peer := newPipeSession(t)
	// `中` = E4 B8 AD
	out := drainDispatchBytes(t, s, peer, []byte{0xE4, 0xB8, 0xAD})

	// LineEdit.InsertRune at end emits just the 3 UTF-8 bytes.
	want := []byte{0xE4, 0xB8, 0xAD}
	if !bytes.Equal(out, want) {
		t.Fatalf("wire bytes = % x, want % x", out, want)
	}
	if !bytes.Equal(s.Input.Buf, want) {
		t.Fatalf("Buf = % x, want % x", s.Input.Buf, want)
	}
	if s.Input.Cursor != 3 {
		t.Fatalf("Cursor = %d, want 3", s.Input.Cursor)
	}
	if s.utf8Have != 0 {
		t.Fatalf("utf8Have = %d after complete rune, want 0", s.utf8Have)
	}
}

func TestDispatcher_PasswordModeCJKEchoesOneAsterisk(t *testing.T) {
	s, peer := newPipeSession(t)
	s.SetPasswordMode(true)
	out := drainDispatchBytes(t, s, peer, []byte{0xE4, 0xB8, 0xAD})

	if string(out) != "*" {
		t.Fatalf("wire bytes = %q, want exactly one asterisk", out)
	}
	if !bytes.Equal(s.Input.Buf, []byte{0xE4, 0xB8, 0xAD}) {
		t.Fatalf("Buf = % x, want CJK 3-byte encoding", s.Input.Buf)
	}
}

func TestDispatcher_MalformedUTF8DropsAndContinues(t *testing.T) {
	s, peer := newPipeSession(t)
	// Lead 0xC2 (start of 2-byte sequence) followed by ASCII 'A'.
	// The lead resets when 'A' arrives via the top-level guard, so
	// the lead's byte is dropped and 'A' dispatches as a normal
	// ASCII insert.
	out := drainDispatchBytes(t, s, peer, []byte{0xC2, 'A'})

	if string(out) != "A" {
		t.Fatalf("wire bytes = %q, want \"A\" only", out)
	}
	if string(s.Input.Buf) != "A" {
		t.Fatalf("Buf = %q, want \"A\"", s.Input.Buf)
	}
	if s.utf8Have != 0 {
		t.Fatalf("utf8Have = %d after malformed sequence, want 0", s.utf8Have)
	}
}

func TestDispatcher_ContinuationWithoutLeadIsDropped(t *testing.T) {
	s, peer := newPipeSession(t)
	// Stray continuation byte 0xB8 with no lead. Should be silently
	// dropped — no echo, no buffer change.
	out := drainDispatchBytes(t, s, peer, []byte{0xB8, 'A'})

	if string(out) != "A" {
		t.Fatalf("wire bytes = %q, want \"A\"", out)
	}
	if string(s.Input.Buf) != "A" {
		t.Fatalf("Buf = %q, want \"A\"", s.Input.Buf)
	}
}

func TestExtendBuffer_CJKPartialUsesCellWidth(t *testing.T) {
	s, peer := newPipeSession(t)

	// Stage Input buffer with a CJK partial token (`中`, 3 bytes,
	// 2 cells). Then call extendBuffer to replace it with an
	// ASCII candidate. Echo must emit \b \b TWICE (cell count),
	// not once (rune count).
	s.Input.Buf = append(s.Input.Buf, cjkZhong...)
	s.Input.Cursor = 3

	got := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := peer.Read(buf)
		got <- append([]byte{}, buf[:n]...)
	}()

	if err := extendBuffer(s, string(cjkZhong), "look "); err != nil {
		t.Fatalf("extendBuffer: %v", err)
	}
	s.Conn.Close()
	out := <-got

	// Expected: `\b \b\b \b` (2 erase pairs) + "look ".
	want := []byte("\b \b\b \blook ")
	if !bytes.Equal(out, want) {
		t.Fatalf("wire bytes = % x, want % x", out, want)
	}
}

func TestUTF8ExpectedLen(t *testing.T) {
	cases := []struct {
		lead byte
		want int
	}{
		{'A', 1},
		{0x7F, 1},
		{0xC2, 2}, {0xDF, 2}, // 2-byte range
		{0xE0, 3}, {0xEF, 3}, // 3-byte range
		{0xF0, 4}, {0xF4, 4}, // 4-byte range
		{0x80, 0}, {0xBF, 0}, // continuation bytes, not valid leads
		{0xC0, 0}, {0xC1, 0}, // overlong 2-byte, invalid
		{0xF5, 0}, {0xFF, 0}, // invalid leads beyond Unicode max
	}
	for _, tc := range cases {
		if got := utf8ExpectedLen(tc.lead); got != tc.want {
			t.Errorf("utf8ExpectedLen(%#x) = %d, want %d", tc.lead, got, tc.want)
		}
	}
}
