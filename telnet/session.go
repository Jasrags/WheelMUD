package telnet

import "net"

type Session struct {
	Conn           net.Conn
	RemoteAddress  string
	TerminalType   string
	Width          int
	Height         int
	InputBuffer    []byte
	InPasswordMode bool
	ColorLevel     int
}

func NewSession(conn net.Conn) *Session {
	if conn == nil {
		return nil
	}

	s := &Session{
		Conn:          conn,
		RemoteAddress: conn.RemoteAddr().String(),
		InputBuffer:   make([]byte, 0),
		Width:         80, // Default width
		Height:        24, // Default height
		ColorLevel:    ColorLevel16,
	}

	return s
}

func (s *Session) WriteString(text string) {
	formatted := RenderColorTags(text, s)
	s.Conn.Write([]byte(formatted))
}
