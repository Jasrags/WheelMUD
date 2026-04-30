package telnet

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"testing"
)

// The leading IAC byte is consumed by dispatchByte before ReadIAC runs, so
// the inputs below describe what comes *after* that leading IAC.

func TestReadIAC_EscapedIACReturnsLiteralByte(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte{TELNET_IAC}))
	b, hasData, err := ReadIAC(nil, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasData || b != TELNET_IAC {
		t.Fatalf("got (b=%d hasData=%v), want (255, true)", b, hasData)
	}
}

func TestReadIAC_StandaloneCommandsConsumeNoExtraBytes(t *testing.T) {
	cases := []struct {
		name string
		cmd  byte
	}{
		{"GA", TELNET_GA},
		{"NOP", TELNET_NOP},
		{"DM", TELNET_DM},
		{"BRK", TELNET_BRK},
		{"IP", TELNET_IP},
		{"AO", TELNET_AO},
		{"AYT", TELNET_AYT},
		{"EC", TELNET_EC},
		{"EL", TELNET_EL},
		{"EOR", TELNET_EOR},
		{"SE", TELNET_SE},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Trailing 'X' verifies ReadIAC did not over-consume.
			r := bufio.NewReader(bytes.NewReader([]byte{tc.cmd, 'X'}))
			b, hasData, err := ReadIAC(nil, r)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hasData || b != 0 {
				t.Fatalf("got (b=%d hasData=%v), want (0, false)", b, hasData)
			}
			next, err := r.ReadByte()
			if err != nil || next != 'X' {
				t.Fatalf("trailing byte = %q err=%v, want 'X'", next, err)
			}
		})
	}
}

func TestReadIAC_NegotiationConsumesOptionByte(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"WILL NAWS", []byte{TELNET_WILL, TELNET_OPT_NAWS}},
		{"WONT ECHO", []byte{TELNET_WONT, TELNET_OPT_ECHO}},
		{"DO TERM_TYPE", []byte{TELNET_DO, TELNET_OPT_TERM_TYPE}},
		{"DONT NAWS", []byte{TELNET_DONT, TELNET_OPT_NAWS}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := bufio.NewReader(bytes.NewReader(append(tc.in, 'Z')))
			b, hasData, err := ReadIAC(nil, r)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hasData || b != 0 {
				t.Fatalf("got (b=%d hasData=%v), want (0, false)", b, hasData)
			}
			next, err := r.ReadByte()
			if err != nil || next != 'Z' {
				t.Fatalf("trailing byte = %q err=%v, want 'Z'", next, err)
			}
		})
	}
}

func TestReadIAC_TruncatedNegotiationReturnsError(t *testing.T) {
	// IAC WILL <EOF>: option byte is missing.
	r := bufio.NewReader(bytes.NewReader([]byte{TELNET_WILL}))
	_, _, err := ReadIAC(nil, r)
	if err == nil || !errors.Is(err, io.EOF) {
		t.Fatalf("err = %v, want EOF wrapped", err)
	}
}

func TestReadSubnegotiation_EscapedIACInPayload(t *testing.T) {
	// SB NAWS <0xFF as data via IAC IAC> 0x50 IAC SE -> data should contain 0xFF, 0x50.
	s, _ := newPipeSession(t)

	payload := []byte{
		TELNET_SB, TELNET_OPT_NAWS,
		TELNET_IAC, TELNET_IAC, // escaped 0xFF
		0x50,       // 80
		0x00, 0x18, // bogus tail to fill to >= 4 bytes for NAWS handler
		TELNET_IAC, TELNET_SE,
	}
	r := bufio.NewReader(bytes.NewReader(payload))
	if _, _, err := ReadIAC(s, r); err != nil {
		t.Fatalf("ReadIAC err: %v", err)
	}
	// HandleSubnegotiation parsed NAWS: width = 0xFF50 = 65360, height = 0x0018 = 24.
	if s.Width != 0xFF50 || s.Height != 0x18 {
		t.Fatalf("NAWS = %dx%d, want 65360x24 (escaped IAC must survive)", s.Width, s.Height)
	}
}

func TestReadSubnegotiation_RejectsRunawayPayload(t *testing.T) {
	s, _ := newPipeSession(t)

	// SB NAWS <maxSubnegotiationLen+8 garbage bytes, no IAC SE>
	buf := make([]byte, 0, maxSubnegotiationLen+16)
	buf = append(buf, TELNET_SB, TELNET_OPT_NAWS)
	for i := 0; i < maxSubnegotiationLen+8; i++ {
		buf = append(buf, 'a')
	}
	buf = append(buf, TELNET_IAC, TELNET_SE)

	r := bufio.NewReader(bytes.NewReader(buf))
	_, _, err := ReadIAC(s, r)
	if !errors.Is(err, ErrSubnegotiationTooLong) {
		t.Fatalf("err = %v, want ErrSubnegotiationTooLong", err)
	}
}
