package telnet

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
)

const (
	TELNET_IAC  byte = 255 // Interpret as command
	TELNET_DONT byte = 254 // Indicates the demand that the other party stop performing, or confirmation that you are no longer expecting the other party to perform, the indicated option.
	TELNET_DO   byte = 253 // Indicates the request that the other party perform, or confirmation that you are expecting the other party to perform, the indicated option.
	TELNET_WONT byte = 252 // Indicates the refusal to perform, or continue performing, the indicated option.
	TELNET_WILL byte = 251 // Indicates the desire to begin performing, or confirmation that you are now performing, the indicated option.
	TELNET_SB   byte = 250 // Subnegotiation of the indicated option follows.
	TELNET_SE   byte = 240 // End of subnegotiation parameters

	// Common...
	TELNET_OPT_ECHO       byte = 1  // Echo RFC: http://pcmicro.com/netfoss/RFC857.html
	TELNET_OPT_SUP_GO_AHD byte = 3  // Suppress Go Ahead RFC: http://pcmicro.com/netfoss/RFC858.html
	TELNET_OPT_TERM_TYPE  byte = 24 // Terminal Type RFC: https://www.ietf.org/rfc/rfc1091.txt
	TELNET_OPT_NAWS       byte = 31 // NAWS, Negotiate About Window Size. RFC: https://www.ietf.org/rfc/rfc1073.txt
)

func HandleSubnegotiation(s *Session, opt byte, data []byte) {
	switch opt {
	case TELNET_OPT_TERM_TYPE:
		if len(data) > 1 && data[0] == 0 {
			s.TerminalType = string(data[1:])
			s.ColorLevel = DetectColorLevel(s.TerminalType)
			slog.Info("Received terminal type", "type", s.TerminalType, "color", s.ColorLevel)
			slog.Debug("Detected color support level", "terminal", s.TerminalType, "level", s.ColorLevel)
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

func NegotiateTelnet(conn net.Conn) {
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
			slog.Warn("Failed to write Telnet IAC", "cmd", DescribeIAC(cmd), "error", err)
			continue
		}
		slog.Debug("Sent IAC", "cmd", DescribeIAC(cmd))
	}
	writer.Flush()
	slog.Info("Sent Telnet negotiation commands")
}

func RequestTerminalType(conn net.Conn) {
	seq := []byte{TELNET_IAC, TELNET_SB, TELNET_OPT_TERM_TYPE, 1, TELNET_IAC, TELNET_SE}
	if _, err := conn.Write(seq); err != nil {
		slog.Warn("Failed to request terminal type", "error", err)
	} else {
		slog.Debug("Requested terminal type using SB SEND")
	}
}
