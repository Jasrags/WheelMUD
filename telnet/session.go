package telnet

import (
	"errors"
	"fmt"
	"net"
	"strings"
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
	// AuthLevel is the privilege the session has earned. Defaults to
	// AuthGuest until login mode promotes it. Registry.Dispatch checks
	// this against Command.Auth.
	AuthLevel AuthLevel

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

// WriteWrapped renders cfmt tags and reflows the result to the session's
// current width before writing. Output newlines are emitted as CRLF so
// telnet clients render them correctly. A width of 0 falls back to
// WriteString without reflowing.
func (s *Session) WriteWrapped(text string) error {
	if s.Width <= 0 {
		return s.WriteString(text)
	}
	rendered := cfmt.Sprint(text)
	wrapped := WrapText(rendered, s.Width)
	// WrapText emits LF-only line breaks; convert to CRLF for the wire.
	wrapped = strings.ReplaceAll(wrapped, "\n", "\r\n")
	return s.WriteRaw([]byte(wrapped))
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

// PushMode adds m to the top of the mode stack and calls m.OnEnter. If
// OnEnter returns an error the push is rolled back so the stack stays
// consistent (the caller treats a failed push as "not on the stack").
func (s *Session) PushMode(m Mode) error {
	if m == nil {
		return errors.New("telnet: PushMode(nil)")
	}
	s.modeMu.Lock()
	s.modes = append(s.modes, m)
	s.modeMu.Unlock()
	if err := m.OnEnter(s); err != nil {
		s.modeMu.Lock()
		// Defensive: only trim if our mode is still on top — a concurrent
		// Pop could have already removed it.
		if n := len(s.modes); n > 0 && s.modes[n-1] == m {
			s.modes = s.modes[:n-1]
		}
		s.modeMu.Unlock()
		return err
	}
	return nil
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
