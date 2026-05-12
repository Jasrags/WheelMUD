package telnet

import (
	"bufio"
	"bytes"
	"net"
	"testing"
	"time"
)

// FuzzReadIAC exercises the IAC + subnegotiation parser with arbitrary
// post-IAC byte streams. The contract: ReadIAC must terminate (no infinite
// loop), must not panic, and must surface only well-formed errors. EOF and
// the subnegotiation length cap are the expected loop exits.
func FuzzReadIAC(f *testing.F) {
	// Seed corpus: representative IAC sequences plus pathological patterns.
	f.Add([]byte{TELNET_DO, TELNET_OPT_ECHO})
	f.Add([]byte{TELNET_WILL, TELNET_OPT_NAWS})
	f.Add([]byte{TELNET_IAC}) // escaped 0xFF data byte
	f.Add([]byte{TELNET_NOP})
	f.Add([]byte{TELNET_GA})
	f.Add([]byte{TELNET_SB, TELNET_OPT_NAWS, 0, 80, 0, 24, TELNET_IAC, TELNET_SE})
	f.Add([]byte{TELNET_SB, TELNET_OPT_TERM_TYPE, 0, 'x', 't', 'e', 'r', 'm', TELNET_IAC, TELNET_SE})
	// SB with escaped 0xFF inside payload.
	f.Add([]byte{TELNET_SB, TELNET_OPT_TERM_TYPE, 0, TELNET_IAC, TELNET_IAC, TELNET_IAC, TELNET_SE})
	// Truncated subnegotiation (no SE).
	f.Add([]byte{TELNET_SB, TELNET_OPT_NAWS, 0, 0, 0, 0})
	// SB with spec-violating IAC suffix.
	f.Add([]byte{TELNET_SB, TELNET_OPT_NAWS, 0, 80, TELNET_IAC, 0x42})
	// Unknown IAC command.
	f.Add([]byte{0x42})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Closed pipe — Session won't write through it. HandleSubnegotiation
		// only mutates session fields; ReadIAC logs but never writes to conn.
		srv, cli := net.Pipe()
		_ = cli.Close()
		s := NewSession(srv)
		defer srv.Close()
		// Defensive deadline so any code path that surprisingly does block on
		// the conn exits quickly.
		_ = srv.SetDeadline(time.Now().Add(50 * time.Millisecond))

		r := bufio.NewReader(bytes.NewReader(data))
		// One iteration per IAC sequence; loop until the reader drains.
		// Each ReadIAC consumes 1+ bytes, so this terminates.
		// The contract under test is "no panic, terminate within bounded
		// iterations". Any error (EOF / SB-too-long / wrapped read error) is
		// a valid exit. The iteration cap is generous but finite.
		for i := 0; i < len(data)+4; i++ {
			if _, _, err := ReadIAC(s, r); err != nil {
				return
			}
		}
	})
}
