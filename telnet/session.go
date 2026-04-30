package telnet

import (
	"fmt"
	"net"
	"sync"

	"github.com/i582/cfmt/cmd/cfmt"
)

type Session struct {
	Conn           net.Conn
	RemoteAddress  string
	TerminalType   string
	Width          int
	Height         int
	InputBuffer    []byte
	InPasswordMode bool
	ColorLevel     int

	writeMu sync.Mutex
}

func NewSession(conn net.Conn) *Session {
	if conn == nil {
		return nil
	}

	return &Session{
		Conn:          conn,
		RemoteAddress: conn.RemoteAddr().String(),
		InputBuffer:   make([]byte, 0),
		Width:         80,
		Height:        24,
		ColorLevel:    ColorLevel16,
	}
}

// WriteString renders cfmt tags on `text` and writes the result to the
// connection. The write is serialized so concurrent callers do not interleave
// bytes on the wire. Any I/O error is returned so the caller can tear down the
// session.
//
// Note: cfmt interprets `{{...}}::style` tokens, so callers MUST NOT pass
// untrusted input directly. Use WriteRaw for client-derived strings.
func (s *Session) WriteString(text string) error {
	rendered := cfmt.Sprint(text)
	return s.WriteRaw([]byte(rendered))
}

// WriteRaw writes the bytes verbatim, with no template rendering.
func (s *Session) WriteRaw(b []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.Conn.Write(b); err != nil {
		return fmt.Errorf("session write: %w", err)
	}
	return nil
}
