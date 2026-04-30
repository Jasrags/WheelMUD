package telnet

import (
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/i582/cfmt/cmd/cfmt"
)

// inboxCap bounds the number of unprocessed input lines per session.
// Legitimate input never queues this deep; backpressure trips on flooding.
const inboxCap = 16

// ErrInputFlooded is returned by RunSession when the dispatcher cannot keep
// up with incoming lines.
var ErrInputFlooded = errors.New("telnet: input flooded")

// ErrSessionEnded is returned by Mode.Handle implementations that have
// closed the connection or otherwise want the dispatcher to terminate
// without writing a prompt afterward (e.g., a `quit` command).
var ErrSessionEnded = errors.New("telnet: session ended")

type Session struct {
	Conn          net.Conn
	RemoteAddress string
	TerminalType  string
	Width         int
	Height        int
	// InputBuffer accumulates raw printable bytes until a line terminator
	// arrives. It is owned by the read goroutine inside RunSession and must
	// not be mutated from anywhere else; reading it from another goroutine
	// is also unsafe. The slice retains its high-water-mark allocation for
	// the life of the session, which is acceptable at MUD scale.
	InputBuffer    []byte
	InPasswordMode bool
	ColorLevel     int

	writeMu sync.Mutex

	modeMu sync.Mutex
	modes  []Mode

	inbox chan string
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
		inbox:         make(chan string, inboxCap),
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

// PushMode adds m to the top of the mode stack and calls m.OnEnter.
func (s *Session) PushMode(m Mode) error {
	if m == nil {
		return errors.New("telnet: PushMode(nil)")
	}
	s.modeMu.Lock()
	s.modes = append(s.modes, m)
	s.modeMu.Unlock()
	return m.OnEnter(s)
}

// PopMode removes the top mode and calls its OnExit. Returns ErrNoMode if
// the stack is empty.
func (s *Session) PopMode() error {
	s.modeMu.Lock()
	if len(s.modes) == 0 {
		s.modeMu.Unlock()
		return ErrNoMode
	}
	top := s.modes[len(s.modes)-1]
	s.modes = s.modes[:len(s.modes)-1]
	s.modeMu.Unlock()
	return top.OnExit(s)
}

// ReplaceMode pops the current top mode (if any) and pushes m. OnExit is
// called for the popped mode and OnEnter for m.
func (s *Session) ReplaceMode(m Mode) error {
	if m == nil {
		return errors.New("telnet: ReplaceMode(nil)")
	}
	if err := s.PopMode(); err != nil && !errors.Is(err, ErrNoMode) {
		return err
	}
	return s.PushMode(m)
}

// CurrentMode returns the top of the mode stack, or nil if empty.
func (s *Session) CurrentMode() Mode {
	s.modeMu.Lock()
	defer s.modeMu.Unlock()
	if len(s.modes) == 0 {
		return nil
	}
	return s.modes[len(s.modes)-1]
}
