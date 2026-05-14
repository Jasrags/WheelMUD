package telnet

import (
	"bufio"
	"bytes"
	"io"
	"testing"
)

// drainPeerBytes reads everything sendable from the test pipe peer until n
// bytes are available, the peer EOFs, or the buffer caps out.
func drainPeerBytes(t *testing.T, r io.Reader, max int) []byte {
	t.Helper()
	buf := make([]byte, max)
	total := 0
	for total < max {
		n, err := r.Read(buf[total:])
		if n > 0 {
			total += n
		}
		if err != nil {
			break
		}
	}
	return buf[:total]
}

// readIACBytes feeds bytes to ReadIAC by wrapping them in a bufio.Reader.
// The first byte fed must be the byte AFTER the leading IAC, as ReadIAC
// itself assumes the IAC has already been consumed by the caller.
func readIACBytes(t *testing.T, s *Session, after ...byte) {
	t.Helper()
	r := bufio.NewReader(bytes.NewReader(after))
	if _, _, err := ReadIAC(s, r); err != nil {
		t.Fatalf("ReadIAC: %v", err)
	}
}

func TestHandleOptionNegotiation_DOCharsetSendsRequest(t *testing.T) {
	s, peer := newPipeSession(t)

	// Read the wire bytes that handleOptionNegotiation writes back.
	gotCh := make(chan []byte, 1)
	go func() { gotCh <- drainPeerBytes(t, peer, 128) }()

	// Client sent: IAC DO CHARSET. Feed [DO, CHARSET] (the IAC was
	// already consumed by the byte-dispatcher in the real readLoop).
	readIACBytes(t, s, TELNET_DO, TELNET_OPT_CHARSET)

	// Close the server side so the peer reader unblocks.
	s.Conn.Close()
	got := <-gotCh

	want := []byte{
		TELNET_IAC, TELNET_SB, TELNET_OPT_CHARSET, CHARSET_REQUEST,
		';', 'U', 'T', 'F', '-', '8',
		TELNET_IAC, TELNET_SE,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("CHARSET REQUEST\n got:  %v\n want: %v", got, want)
	}
}

func TestHandleCharsetSubnegotiation_AcceptedUTF8StampsCharset(t *testing.T) {
	s, _ := newPipeSession(t)
	// Payload: ACCEPTED "UTF-8". The caller of readSubnegotiation
	// strips the SB framing; HandleSubnegotiation receives just the
	// option byte + payload, but for testing we drive
	// handleCharsetSubnegotiation directly to keep the assertion
	// focused.
	handleCharsetSubnegotiation(s, append([]byte{CHARSET_ACCEPTED}, []byte("UTF-8")...))
	if got := s.Charset(); got != "UTF-8" {
		t.Fatalf("Charset() = %q, want %q", got, "UTF-8")
	}
}

func TestHandleCharsetSubnegotiation_AcceptedLowercaseUTF8(t *testing.T) {
	s, _ := newPipeSession(t)
	handleCharsetSubnegotiation(s, append([]byte{CHARSET_ACCEPTED}, []byte("utf-8")...))
	if got := s.Charset(); got != "UTF-8" {
		t.Fatalf("Charset() = %q, want %q", got, "UTF-8")
	}
}

func TestHandleCharsetSubnegotiation_RejectedLeavesEmpty(t *testing.T) {
	s, _ := newPipeSession(t)
	handleCharsetSubnegotiation(s, []byte{CHARSET_REJECTED})
	if got := s.Charset(); got != "" {
		t.Fatalf("Charset() = %q, want empty after REJECTED", got)
	}
}

func TestHandleCharsetSubnegotiation_UnknownCharsetIgnored(t *testing.T) {
	s, _ := newPipeSession(t)
	handleCharsetSubnegotiation(s, append([]byte{CHARSET_ACCEPTED}, []byte("LATIN-1")...))
	if got := s.Charset(); got != "" {
		t.Fatalf("Charset() = %q, want empty for unsupported encoding", got)
	}
}

func TestHandleOptionNegotiation_DOMsspWritesProviderBlock(t *testing.T) {
	s, peer := newPipeSession(t)
	s.MSSPProvider = func() []MSSPVar {
		return []MSSPVar{
			{Name: "NAME", Value: "WheelMUD"},
			{Name: "PLAYERS", Value: "0"},
		}
	}

	gotCh := make(chan []byte, 1)
	go func() { gotCh <- drainPeerBytes(t, peer, 256) }()

	readIACBytes(t, s, TELNET_DO, TELNET_OPT_MSSP)
	s.Conn.Close()
	got := <-gotCh

	// Verify framing + at least the two expected var/val pairs are
	// present. Strict byte-equality is covered by TestEncodeMSSP; here
	// we want a smoke test that the negotiation path wires through.
	if !bytes.HasPrefix(got, []byte{TELNET_IAC, TELNET_SB, TELNET_OPT_MSSP}) {
		t.Fatalf("missing MSSP SB prefix: %v", got)
	}
	if !bytes.HasSuffix(got, []byte{TELNET_IAC, TELNET_SE}) {
		t.Fatalf("missing IAC SE: %v", got)
	}
	if !bytes.Contains(got, []byte("WheelMUD")) {
		t.Fatalf("payload missing NAME value: %v", got)
	}
}

func TestHandleOptionNegotiation_DOMsspWithNilProviderIsNoOp(t *testing.T) {
	s, peer := newPipeSession(t)
	// MSSPProvider deliberately not set.

	// Use a non-blocking peer read with a deadline-style helper: any
	// write would arrive immediately; if no bytes appear after the
	// negotiation call we know the path returned cleanly.
	doneCh := make(chan int, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := peer.Read(buf)
		doneCh <- n
	}()

	readIACBytes(t, s, TELNET_DO, TELNET_OPT_MSSP)
	s.Conn.Close() // force the peer reader to return

	n := <-doneCh
	if n != 0 {
		t.Fatalf("nil provider should not write; got %d bytes", n)
	}
}
