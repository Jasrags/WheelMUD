package telnet

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
)

const (
	TELNET_IAC  byte = 255 // Interpret as command
	TELNET_DONT byte = 254 // Demand the other party stop performing the indicated option.
	TELNET_DO   byte = 253 // Request the other party perform the indicated option.
	TELNET_WONT byte = 252 // Refusal to perform the indicated option.
	TELNET_WILL byte = 251 // Desire to begin performing the indicated option.
	TELNET_SB   byte = 250 // Subnegotiation of the indicated option follows.
	TELNET_SE   byte = 240 // End of subnegotiation parameters.
	TELNET_GA   byte = 249 // Go ahead.
	TELNET_NOP  byte = 241 // No operation.
	TELNET_DM   byte = 242 // Data Mark.
	TELNET_BRK  byte = 243 // Break.
	TELNET_IP   byte = 244 // Interrupt Process.
	TELNET_AO   byte = 245 // Abort Output.
	TELNET_AYT  byte = 246 // Are You There.
	TELNET_EC   byte = 247 // Erase Character.
	TELNET_EL   byte = 248 // Erase Line.
	TELNET_EOR  byte = 239 // End of Record.

	// Common options
	TELNET_OPT_ECHO       byte = 1   // Echo: RFC 857
	TELNET_OPT_SUP_GO_AHD byte = 3   // Suppress Go Ahead: RFC 858
	TELNET_OPT_TERM_TYPE  byte = 24  // Terminal Type: RFC 1091
	TELNET_OPT_NAWS       byte = 31  // NAWS, Negotiate About Window Size: RFC 1073
	TELNET_OPT_CHARSET    byte = 42  // CHARSET: RFC 2066
	TELNET_OPT_MSSP       byte = 70  // MSSP: mssp.org
	TELNET_OPT_GMCP       byte = 201 // GMCP: mudlet.org/manual

	// CHARSET sub-option codes (RFC 2066 §3).
	CHARSET_REQUEST  byte = 1
	CHARSET_ACCEPTED byte = 2
	CHARSET_REJECTED byte = 3

	// MSSP sub-option codes (mssp.org spec).
	MSSP_VAR byte = 1
	MSSP_VAL byte = 2
)

// charsetSeparator is the single byte we send between REQUEST and the
// charset list. RFC 2066 lets the server pick any byte that doesn't
// appear in the names; ';' is the convention every major client expects.
const charsetSeparator = ';'

// maxSubnegotiationLen caps the bytes accepted between IAC SB and IAC SE so a
// malicious client cannot exhaust memory by streaming an unterminated SB.
const maxSubnegotiationLen = 4096

// ErrSubnegotiationTooLong is returned when a subnegotiation payload exceeds
// maxSubnegotiationLen before IAC SE is seen.
var ErrSubnegotiationTooLong = errors.New("telnet: subnegotiation exceeds max length")

var (
	TelnetRequestSuppressGoAhead     = []byte{TELNET_IAC, TELNET_DO, TELNET_OPT_SUP_GO_AHD}
	TelnetRequestDontSuppressGoAhead = []byte{TELNET_IAC, TELNET_DONT, TELNET_OPT_SUP_GO_AHD}
	TelnetRequestEcho                = []byte{TELNET_IAC, TELNET_WILL, TELNET_OPT_ECHO}
	TelnetRequestDontEcho            = []byte{TELNET_IAC, TELNET_WONT, TELNET_OPT_ECHO}
	TelnetRequestLineModeOff         = []byte{TELNET_IAC, TELNET_DONT, TELNET_OPT_ECHO}
	TelnetRequestNAWS                = []byte{TELNET_IAC, TELNET_DO, TELNET_OPT_NAWS}
	TelnetGoAhead                    = []byte{TELNET_IAC, TELNET_GA}
)

// ReadIAC handles a telnet command after the leading IAC byte has been read.
// It dispatches subnegotiation to HandleSubnegotiation, logs other commands,
// and unescapes a literal 0xFF (IAC IAC) as a regular data byte returned via
// the dataByte/hasData return values. The caller passes the byte they read.
func ReadIAC(s *Session, r *bufio.Reader) (dataByte byte, hasData bool, err error) {
	cmd, err := r.ReadByte()
	if err != nil {
		return 0, false, fmt.Errorf("read IAC command: %w", err)
	}
	switch cmd {
	case TELNET_IAC:
		// Escaped 0xFF in the data stream.
		return TELNET_IAC, true, nil
	case TELNET_SB:
		return 0, false, readSubnegotiation(s, r)
	case TELNET_WILL, TELNET_WONT, TELNET_DO, TELNET_DONT:
		opt, oerr := r.ReadByte()
		if oerr != nil {
			return 0, false, fmt.Errorf("read IAC option: %w", oerr)
		}
		slog.Debug("Received IAC", "cmd", DescribeByte(cmd), "opt", DescribeByte(opt))
		handleOptionNegotiation(s, cmd, opt)
		return 0, false, nil
	case TELNET_GA, TELNET_NOP, TELNET_DM, TELNET_BRK, TELNET_IP,
		TELNET_AO, TELNET_AYT, TELNET_EC, TELNET_EL, TELNET_EOR, TELNET_SE:
		slog.Debug("Received IAC", "cmd", DescribeByte(cmd))
		return 0, false, nil
	default:
		slog.Debug("Received unknown IAC command", "cmd", DescribeByte(cmd))
		return 0, false, nil
	}
}

func readSubnegotiation(s *Session, r *bufio.Reader) error {
	opt, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read SB option: %w", err)
	}
	data := make([]byte, 0, 32)
	for {
		if len(data) > maxSubnegotiationLen {
			return ErrSubnegotiationTooLong
		}
		d, derr := r.ReadByte()
		if derr != nil {
			return fmt.Errorf("read SB payload: %w", derr)
		}
		if d != TELNET_IAC {
			data = append(data, d)
			continue
		}
		next, nerr := r.ReadByte()
		if nerr != nil {
			return fmt.Errorf("read SB IAC suffix: %w", nerr)
		}
		switch next {
		case TELNET_SE:
			HandleSubnegotiation(s, opt, data)
			return nil
		case TELNET_IAC:
			// Escaped 0xFF inside subnegotiation payload.
			data = append(data, TELNET_IAC)
		default:
			// Spec-violating client: treat as terminator and log.
			slog.Debug("Unexpected IAC suffix in SB", "byte", DescribeByte(next))
			HandleSubnegotiation(s, opt, data)
			return nil
		}
	}
}

func HandleSubnegotiation(s *Session, opt byte, data []byte) {
	switch opt {
	case TELNET_OPT_TERM_TYPE:
		if len(data) > 1 && data[0] == 0 {
			s.TerminalType = string(data[1:])
			s.ColorLevel = DetectColorLevel(s.TerminalType)
			slog.Info("Received terminal type", "type", s.TerminalType, "color", s.ColorLevel)
		}
	case TELNET_OPT_NAWS:
		if len(data) >= 4 {
			s.Width = int(binary.BigEndian.Uint16(data[:2]))
			s.Height = int(binary.BigEndian.Uint16(data[2:4]))
			slog.Info("Received terminal window size", "width", s.Width, "height", s.Height)
		}
	case TELNET_OPT_CHARSET:
		handleCharsetSubnegotiation(s, data)
	case TELNET_OPT_GMCP:
		handleGMCPSubnegotiation(s, data)
	default:
		slog.Info("Unhandled subnegotiation option", "option", DescribeByte(opt), "data", data)
	}
}

// handleOptionNegotiation is the WILL/WONT/DO/DONT response dispatcher.
// It runs on the read goroutine after ReadIAC consumes the option byte
// and writes any required response through s.WriteRaw (which takes
// writeMu). DONT/WONT are logged only — we don't proactively renegotiate.
//
// CHARSET (RFC 2066): when the client says DO CHARSET we send a REQUEST
// listing UTF-8. The client replies with SB CHARSET ACCEPTED <name> SE
// or SB CHARSET REJECTED SE, handled in handleCharsetSubnegotiation.
//
// MSSP (mssp.org): when the client says DO MSSP we emit the full
// variable block built by the closure on Session.MSSPProvider. Nil
// provider = silently no-op (test fixtures and pre-wire codepaths).
func handleOptionNegotiation(s *Session, cmd, opt byte) {
	if cmd != TELNET_DO {
		// WILL/WONT/DONT have nothing we currently need to do beyond
		// the debug log emitted by the caller. Future MCCP / MSDP /
		// GMCP handlers may want to track WILL/WONT here.
		return
	}
	switch opt {
	case TELNET_OPT_CHARSET:
		if err := s.WriteRaw(buildCharsetRequest("UTF-8")); err != nil {
			slog.Debug("CHARSET REQUEST write failed", "remote", s.RemoteAddress, "error", err)
		}
	case TELNET_OPT_MSSP:
		provider := s.MSSPProvider
		if provider == nil {
			return
		}
		// Respond once per session. CAS-to-true on the first DO; any
		// subsequent DO MSSP is dropped silently per RFC 855 and to
		// blunt amplification by a misbehaving client.
		if !s.msspSent.CompareAndSwap(false, true) {
			return
		}
		vars := provider()
		if len(vars) == 0 {
			return
		}
		if err := s.WriteRaw(EncodeMSSP(vars)); err != nil {
			slog.Debug("MSSP response write failed", "remote", s.RemoteAddress, "error", err)
		}
	case TELNET_OPT_GMCP:
		// Flip the per-session GMCP flag. The client now drives the
		// session via inbound Core.Hello + Core.Supports.Set frames;
		// the server has nothing to write in response to bare DO GMCP.
		// Idempotent — repeated DOs are harmless.
		s.SetGMCPEnabled(true)
	}
}

// buildCharsetRequest assembles a `IAC SB CHARSET REQUEST <sep><name>
// IAC SE` block offering exactly one charset. RFC 2066 §3 lets us pick
// any separator byte that doesn't appear in the names; we use ';'.
func buildCharsetRequest(charset string) []byte {
	out := make([]byte, 0, 6+1+len(charset)+2)
	out = append(out, TELNET_IAC, TELNET_SB, TELNET_OPT_CHARSET, CHARSET_REQUEST)
	out = append(out, charsetSeparator)
	out = append(out, charset...)
	out = append(out, TELNET_IAC, TELNET_SE)
	return out
}

// handleGMCPSubnegotiation processes a GMCP subnegotiation payload.
// Wire shape: `<package-name> <space> <json-body>` (the SB framing
// has already been stripped by readSubnegotiation). The first ASCII
// space splits the name from the body; a missing space means the
// frame is name-only (body defaults to "null"). Hands off to the
// session's GMCPHandler closure if one is wired; otherwise drops.
func handleGMCPSubnegotiation(s *Session, data []byte) {
	if len(data) == 0 {
		slog.Debug("Empty GMCP subnegotiation", "remote", s.RemoteAddress)
		return
	}
	if s.GMCPHandler == nil {
		// Pre-wire path (tests, account-menu-only sessions) — drop
		// silently. A nil handler is not an error.
		return
	}
	var pkg string
	var body []byte
	if sp := indexSpace(data); sp >= 0 {
		pkg = string(data[:sp])
		body = data[sp+1:]
	} else {
		pkg = string(data)
		body = []byte("null")
	}
	s.GMCPHandler(s, pkg, body)
}

// indexSpace returns the position of the first 0x20 byte or -1.
// Lighter than bytes.IndexByte for this hot path.
func indexSpace(b []byte) int {
	for i, c := range b {
		if c == ' ' {
			return i
		}
	}
	return -1
}

// handleCharsetSubnegotiation processes a CHARSET subnegotiation
// payload (the bytes between SB CHARSET and IAC SE). The first byte is
// the sub-option code; on ACCEPTED the rest is the chosen charset
// name. We only stamp Session.Charset when the client picked UTF-8 —
// anything else is logged and left empty (the WrapText fast-path keeps
// rune counting, which is the safe default for non-UTF-8 transports).
func handleCharsetSubnegotiation(s *Session, data []byte) {
	if len(data) == 0 {
		slog.Debug("Empty CHARSET subnegotiation", "remote", s.RemoteAddress)
		return
	}
	switch data[0] {
	case CHARSET_ACCEPTED:
		name := string(data[1:])
		// IANA charset names are case-insensitive (RFC 2278) and the
		// CHARSET option (RFC 2066 §2) inherits that — accept any
		// casing of "UTF-8" the client chose to echo back.
		if strings.EqualFold(name, "UTF-8") {
			s.SetCharset("UTF-8")
			slog.Info("CHARSET negotiated", "charset", "UTF-8", "remote", s.RemoteAddress)
			return
		}
		slog.Debug("CHARSET ACCEPTED with unrecognized name", "name", name, "remote", s.RemoteAddress)
	case CHARSET_REJECTED:
		slog.Debug("CHARSET REJECTED by client", "remote", s.RemoteAddress)
	default:
		slog.Debug("Unhandled CHARSET sub-option", "code", data[0], "remote", s.RemoteAddress)
	}
}

func DescribeIAC(cmd []byte) string {
	if len(cmd) < 3 {
		return fmt.Sprintf("invalid IAC: %v", cmd)
	}
	return fmt.Sprintf("IAC %s %s", DescribeByte(cmd[1]), DescribeByte(cmd[2]))
}

func DescribeByte(b byte) string {
	switch b {
	case TELNET_IAC:
		return "IAC"
	case TELNET_WILL:
		return "WILL"
	case TELNET_WONT:
		return "WONT"
	case TELNET_DO:
		return "DO"
	case TELNET_DONT:
		return "DONT"
	case TELNET_SB:
		return "SB"
	case TELNET_SE:
		return "SE"
	case TELNET_GA:
		return "GA"
	case TELNET_NOP:
		return "NOP"
	case TELNET_DM:
		return "DM"
	case TELNET_BRK:
		return "BRK"
	case TELNET_IP:
		return "IP"
	case TELNET_AO:
		return "AO"
	case TELNET_AYT:
		return "AYT"
	case TELNET_EC:
		return "EC"
	case TELNET_EL:
		return "EL"
	case TELNET_EOR:
		return "EOR"
	case TELNET_OPT_ECHO:
		return "ECHO"
	case TELNET_OPT_SUP_GO_AHD:
		return "SUPPRESS-GO-AHEAD"
	case TELNET_OPT_TERM_TYPE:
		return "TERMINAL-TYPE"
	case TELNET_OPT_NAWS:
		return "NAWS"
	case TELNET_OPT_CHARSET:
		return "CHARSET"
	case TELNET_OPT_MSSP:
		return "MSSP"
	case TELNET_OPT_GMCP:
		return "GMCP"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", b)
	}
}

func NegotiateTelnet(conn net.Conn) error {
	writer := bufio.NewWriter(conn)
	commands := [][]byte{
		{TELNET_IAC, TELNET_WILL, TELNET_OPT_SUP_GO_AHD},
		{TELNET_IAC, TELNET_DO, TELNET_OPT_SUP_GO_AHD},
		{TELNET_IAC, TELNET_DO, TELNET_OPT_TERM_TYPE},
		{TELNET_IAC, TELNET_DO, TELNET_OPT_NAWS},
		{TELNET_IAC, TELNET_WILL, TELNET_OPT_ECHO},
		{TELNET_IAC, TELNET_WILL, TELNET_OPT_CHARSET},
		{TELNET_IAC, TELNET_WILL, TELNET_OPT_MSSP},
		{TELNET_IAC, TELNET_WILL, TELNET_OPT_GMCP},
	}
	for _, cmd := range commands {
		if _, err := writer.Write(cmd); err != nil {
			return fmt.Errorf("write telnet IAC %s: %w", DescribeIAC(cmd), err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush telnet negotiation: %w", err)
	}
	slog.Info("Sent Telnet negotiation commands")
	return nil
}

func RequestTerminalType(conn net.Conn) error {
	seq := []byte{TELNET_IAC, TELNET_SB, TELNET_OPT_TERM_TYPE, 1, TELNET_IAC, TELNET_SE}
	if _, err := conn.Write(seq); err != nil {
		return fmt.Errorf("request terminal type: %w", err)
	}
	return nil
}
