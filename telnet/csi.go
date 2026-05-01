package telnet

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

// CSIOp identifies a recognized CSI escape sequence. Unknown sequences
// are reported as CSIUnknown so the caller can bell (or ignore) without
// special-casing the byte stream.
type CSIOp int

const (
	CSIUnknown CSIOp = iota
	CSIUp
	CSIDown
	CSIRight
	CSILeft
	CSIHome
	CSIEnd
	CSIDelete // forward-delete (ESC [ 3 ~)
)

// csiMaxLen bounds the parameter+intermediate run inside a CSI sequence
// so a malicious client can't spin the parser indefinitely.
const csiMaxLen = 32

// ReadCSI consumes a CSI-style escape sequence after the leading 0x1B
// has already been read off the wire. It returns the parsed op or
// CSIUnknown if the final byte / parameter combination isn't one we
// recognize. Bytes are always fully consumed regardless of recognition,
// so the read loop stays in sync.
func ReadCSI(r *bufio.Reader) (CSIOp, error) {
	// Single Shift sequences (ESC O <final>) — some xterms send arrows in
	// "application keypad" mode as ESC O A/B/C/D. Treat that the same.
	first, err := r.ReadByte()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return CSIUnknown, err
		}
		return CSIUnknown, fmt.Errorf("read CSI introducer: %w", err)
	}
	switch first {
	case '[':
		// fall through
	case 'O':
		final, err := r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return CSIUnknown, err
			}
			return CSIUnknown, fmt.Errorf("read SS3 final: %w", err)
		}
		return ss3Op(final), nil
	default:
		// Not a CSI sequence — treat as a stray ESC with one byte
		// already consumed. We don't try to back-fill; just report
		// unknown and let the caller move on.
		return CSIUnknown, nil
	}

	var (
		params [csiMaxLen]byte
		n      int
	)
	for n < csiMaxLen {
		b, err := r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return CSIUnknown, err
			}
			return CSIUnknown, fmt.Errorf("read CSI body: %w", err)
		}
		// Final byte: 0x40-0x7E.
		if b >= 0x40 && b <= 0x7E {
			return classifyCSI(string(params[:n]), b), nil
		}
		params[n] = b
		n++
	}
	return CSIUnknown, errors.New("telnet: CSI sequence exceeds max length")
}

func classifyCSI(params string, final byte) CSIOp {
	switch final {
	case 'A':
		return CSIUp
	case 'B':
		return CSIDown
	case 'C':
		return CSIRight
	case 'D':
		return CSILeft
	case 'H':
		return CSIHome
	case 'F':
		return CSIEnd
	case '~':
		switch params {
		case "1", "7":
			return CSIHome
		case "4", "8":
			return CSIEnd
		case "3":
			return CSIDelete
		}
	}
	return CSIUnknown
}

func ss3Op(final byte) CSIOp {
	switch final {
	case 'A':
		return CSIUp
	case 'B':
		return CSIDown
	case 'C':
		return CSIRight
	case 'D':
		return CSILeft
	case 'H':
		return CSIHome
	case 'F':
		return CSIEnd
	}
	return CSIUnknown
}
