package telnet

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"time"
	"unicode"
)

// idleTimeout bounds how long a session may sit without any input.
const idleTimeout = 10 * time.Minute

// CommandHandler processes a fully-buffered line of user input.
type CommandHandler func(s *Session, input string) error

// RunSession drives the per-connection read loop: telnet negotiation, IAC
// dispatch, ANSI escape skipping, line buffering, and command dispatch. It
// returns when the connection is closed or a non-recoverable error occurs.
func RunSession(s *Session, handle CommandHandler) error {
	if err := NegotiateTelnet(s.Conn); err != nil {
		return err
	}
	if err := RequestTerminalType(s.Conn); err != nil {
		return err
	}

	reader := bufio.NewReader(s.Conn)
	for {
		if err := s.Conn.SetReadDeadline(time.Now().Add(idleTimeout)); err != nil {
			return err
		}
		b, err := reader.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if err := dispatchByte(s, reader, b, handle); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func dispatchByte(s *Session, r *bufio.Reader, b byte, handle CommandHandler) error {
	switch {
	case b == TELNET_IAC:
		data, hasData, err := ReadIAC(s, r)
		if err != nil {
			return err
		}
		if hasData {
			return bufferInput(s, data)
		}
		return nil
	case b == 0x1B:
		return DiscardANSI(r)
	case b == '\r' || b == '\n':
		return handleLineBreak(s, handle)
	case b == ASCII_BS || b == ASCII_DEL:
		return handleBackspace(s)
	case unicode.IsPrint(rune(b)):
		return bufferInput(s, b)
	default:
		slog.Debug("Received unhandled byte", "byte", b)
		return nil
	}
}

func handleLineBreak(s *Session, handle CommandHandler) error {
	if len(s.InputBuffer) > 0 {
		input := string(s.InputBuffer)
		slog.Info("User entered command", "input", input, "remote", s.RemoteAddress)
		s.InputBuffer = s.InputBuffer[:0]
		if handle != nil {
			if err := handle(s, input); err != nil {
				return err
			}
		}
	}
	return s.WriteRaw([]byte("\r\n> "))
}

func handleBackspace(s *Session) error {
	if len(s.InputBuffer) == 0 {
		return nil
	}
	s.InputBuffer = s.InputBuffer[:len(s.InputBuffer)-1]
	return s.WriteRaw([]byte("\b \b"))
}

func bufferInput(s *Session, b byte) error {
	s.InputBuffer = append(s.InputBuffer, b)
	if s.InPasswordMode {
		return s.WriteRaw([]byte("*"))
	}
	return s.WriteRaw([]byte{b})
}
