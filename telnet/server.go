package telnet

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// idleTimeout bounds how long a session may sit without any input.
const idleTimeout = 10 * time.Minute

// RunSession drives the per-connection read loop and a sibling dispatcher
// goroutine. Lines parsed off the wire flow through the session inbox; the
// dispatcher pops them and invokes the current mode's Handle. Returns when
// the connection closes, the read deadline expires, or a non-recoverable
// error occurs. The session must already have a mode pushed.
func RunSession(s *Session) error {
	if err := NegotiateTelnet(s.Conn); err != nil {
		return err
	}
	if err := RequestTerminalType(s.Conn); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dispatcherDone := make(chan struct{})
	go runDispatcher(ctx, s, dispatcherDone)

	err := readLoop(s)

	// Signal in-flight handlers before closing the inbox so a slow Handle
	// observes cancellation rather than running to completion against a
	// dead connection.
	cancel()
	close(s.inbox)
	<-dispatcherDone
	return err
}

func readLoop(s *Session) error {
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
		if err := dispatchByte(s, reader, b); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func runDispatcher(ctx context.Context, s *Session, done chan<- struct{}) {
	defer close(done)
	for line := range s.inbox {
		mode := s.CurrentMode()
		if mode == nil {
			slog.Warn("No mode for input", "remote", s.RemoteAddress, "line", line)
			continue
		}
		err := mode.Handle(ctx, s, line)
		if shouldEndSession(err) {
			// Mode signaled termination (or wrote to a closed conn);
			// drain remaining lines without invoking handlers.
			for range s.inbox {
			}
			return
		}
		if err != nil {
			slog.Debug("Mode.Handle error", "remote", s.RemoteAddress, "error", err)
		}
		if mode := s.CurrentMode(); mode != nil {
			if prompt := mode.Prompt(s); prompt != "" {
				if werr := s.WriteRaw([]byte(prompt)); werr != nil {
					if !shouldEndSession(werr) {
						slog.Debug("Prompt write failed", "remote", s.RemoteAddress, "error", werr)
					}
					for range s.inbox {
					}
					return
				}
			}
		}
	}
}

// shouldEndSession reports whether err means the session is over and the
// dispatcher should stop without further prompt writes. ErrSessionEnded is
// the explicit signal; the rest are I/O errors against a closed connection.
func shouldEndSession(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrSessionEnded) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, io.EOF) {
		return true
	}
	return false
}

func dispatchByte(s *Session, r *bufio.Reader, b byte) error {
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
	case b == ASCII_ESC:
		return DiscardANSI(r)
	case b == '\r' || b == '\n':
		// Coalesce CRLF: telnet clients send CR LF, so on CR we consume a
		// following LF if it's already in the reader buffer to avoid
		// triggering a second (empty) line break.
		if b == '\r' {
			if next, err := r.Peek(1); err == nil && next[0] == '\n' {
				_, _ = r.ReadByte()
			}
		}
		return handleLineBreak(s)
	case b == ASCII_BS || b == ASCII_DEL:
		return handleBackspace(s)
	case b == ASCII_HT:
		return handleTab(s)
	case unicode.IsPrint(rune(b)):
		return bufferInput(s, b)
	default:
		slog.Debug("Received unhandled byte", "byte", b)
		return nil
	}
}

func handleLineBreak(s *Session) error {
	if len(s.InputBuffer) == 0 {
		// Bare Enter: redraw the current mode's prompt without dispatching.
		if mode := s.CurrentMode(); mode != nil {
			if prompt := mode.Prompt(s); prompt != "" {
				return s.WriteRaw([]byte("\r\n" + prompt))
			}
		}
		return s.WriteRaw([]byte("\r\n"))
	}
	input := string(s.InputBuffer)
	s.InputBuffer = s.InputBuffer[:0]
	// Never log raw input while password masking is active — login,
	// account-create, password-change, etc. all set InPasswordMode and
	// the cleartext password must not enter logs.
	if s.InPasswordMode {
		slog.Info("User entered command", "input", "(redacted)", "remote", s.RemoteAddress)
	} else {
		slog.Info("User entered command", "input", input, "remote", s.RemoteAddress)
	}

	if err := s.WriteRaw([]byte("\r\n")); err != nil {
		return err
	}
	select {
	case s.inbox <- input:
		return nil
	default:
		slog.Warn("Input flooded; closing session", "remote", s.RemoteAddress)
		return ErrInputFlooded
	}
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

// handleTab implements end-of-buffer tab completion. Behavior:
//   - Password mode: bell, never complete (would leak input through echo).
//   - Mode does not implement Completer: bell.
//   - Zero candidates: bell.
//   - One candidate: replace the partial in-place; append a trailing space.
//   - Multiple candidates with a longer common prefix: extend buffer to it.
//   - Multiple candidates with no further common prefix: list above the
//     prompt, columnized once past helpColumnThreshold, soft-capped at
//     maxCandidatesShown, then redraw the prompt and current buffer.
func handleTab(s *Session) error {
	if s.InPasswordMode {
		return s.WriteRaw([]byte{ASCII_BEL})
	}
	mode := s.CurrentMode()
	completer, ok := mode.(Completer)
	if !ok {
		return s.WriteRaw([]byte{ASCII_BEL})
	}
	buffer := string(s.InputBuffer)
	partial := completionPartial(buffer)
	cands := completer.Complete(s, buffer)
	return applyCompletion(s, mode, partial, cands)
}

// completionPartial extracts the trailing token from buffer. With argument
// completion deferred, this is currently equivalent to buffer when buffer
// has no whitespace and "" otherwise; the helper exists so the seam is
// already in the right place when arg completion lands.
func completionPartial(buffer string) string {
	idx := strings.LastIndexAny(buffer, " \t")
	if idx < 0 {
		return buffer
	}
	return buffer[idx+1:]
}

func applyCompletion(s *Session, mode Mode, partial string, cands []Candidate) error {
	switch len(cands) {
	case 0:
		return s.WriteRaw([]byte{ASCII_BEL})
	case 1:
		return extendBuffer(s, partial, cands[0].Text+" ")
	}

	texts := make([]string, len(cands))
	for i, c := range cands {
		texts[i] = c.Text
	}
	cp := commonPrefix(texts)
	if len(cp) > len(partial) {
		return extendBuffer(s, partial, cp)
	}
	return listAndRedraw(s, mode, cands)
}

// extendBuffer rewrites the trailing partial in-place and emits the diff.
// All editing is end-of-buffer only (per design scope), so we erase the old
// partial with backspaces (one per displayed rune) and write the replacement.
func extendBuffer(s *Session, partial, replacement string) error {
	if len(partial) > len(s.InputBuffer) {
		// Defensive: should never happen, but don't slice past the start.
		return nil
	}
	s.InputBuffer = s.InputBuffer[:len(s.InputBuffer)-len(partial)]
	s.InputBuffer = append(s.InputBuffer, replacement...)

	// Erase one display cell per rune of the old partial. ASCII verbs are
	// the steady state today; this stays correct if non-ASCII candidates
	// land later (and validVerb is loosened).
	out := strings.Repeat("\b \b", utf8.RuneCountInString(partial)) + replacement
	return s.WriteRaw([]byte(out))
}

func listAndRedraw(s *Session, mode Mode, cands []Candidate) error {
	listing := formatCandidates(cands, s.Width)
	prompt := ""
	if mode != nil {
		prompt = mode.Prompt(s)
	}

	var b strings.Builder
	b.WriteString("\r\n")
	b.WriteString(listing)
	b.WriteString(prompt)
	b.Write(s.InputBuffer)
	return s.WriteRaw([]byte(b.String()))
}
