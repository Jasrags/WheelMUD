package telnet

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
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
	TELNET_OPT_ECHO       byte = 1  // Echo: RFC 857
	TELNET_OPT_SUP_GO_AHD byte = 3  // Suppress Go Ahead: RFC 858
	TELNET_OPT_TERM_TYPE  byte = 24 // Terminal Type: RFC 1091
	TELNET_OPT_NAWS       byte = 31 // NAWS, Negotiate About Window Size: RFC 1073
)

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
	TelnetRequestChangeCharset       = []byte{TELNET_IAC, TELNET_SB, 0x42, 0x01, TELNET_IAC, TELNET_SE}
	TelnetResponseAcceptedCharset    = []byte{TELNET_IAC, TELNET_SB, 0x42, 0x00, TELNET_IAC, TELNET_SE}
	TelnetResponseRejectedCharset    = []byte{TELNET_IAC, TELNET_SB, 0x42, 0x02, TELNET_IAC, TELNET_SE}
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
	default:
		slog.Info("Unhandled subnegotiation option", "option", DescribeByte(opt), "data", data)
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
