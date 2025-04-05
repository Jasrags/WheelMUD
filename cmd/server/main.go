package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"unicode"

	"github.com/Jasrags/WheelMUD/telnet"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	ln, err := net.Listen("tcp", ":2323")
	if err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
	defer ln.Close()

	slog.Info("Server started", "address", ":2323")

	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Error("Failed to accept connection", "error", err)
			continue
		}

		session := telnet.NewSession(conn)
		if session == nil {
			slog.Error("Failed to create telnet session", "remote", conn.RemoteAddr().String())
			conn.Close()
			continue
		}

		go handleConnection(session)
	}
}

func handleConnection(s *telnet.Session) {
	defer s.Conn.Close()
	slog.Info("Client connected", "remote", s.RemoteAddress)

	telnet.NegotiateTelnet(s.Conn)
	telnet.RequestTerminalType(s.Conn)

	s.WriteString("Welcome to the Telnet server!\r\n")
	s.WriteString("{green}Welcome{/green}\r\n")
	s.WriteString("> ")
	slog.Info("Welcome message and prompt sent", "remote", s.RemoteAddress)

	reader := bufio.NewReader(s.Conn)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			if err != io.EOF {
				slog.Debug("Read error", "error", err)
			}
			break
		}

		if b == telnet.TELNET_IAC {
			cmd, _ := reader.ReadByte()
			if cmd == telnet.TELNET_SB {
				opt, _ := reader.ReadByte()
				data := make([]byte, 0)
				for {
					d, _ := reader.ReadByte()
					if d == telnet.TELNET_IAC {
						if se, _ := reader.ReadByte(); se == telnet.TELNET_SE {
							break
						}
					}
					data = append(data, d)
				}
				telnet.HandleSubnegotiation(s, opt, data)
			} else {
				opt, _ := reader.ReadByte()
				slog.Debug("Received IAC", "cmd", telnet.DescribeByte(cmd), "opt", telnet.DescribeByte(opt))
			}
		} else if b == 0x1B {
			seq, _ := reader.ReadBytes('m')
			slog.Debug("Received ANSI escape sequence", "seq", fmt.Sprintf("%c%s", b, string(seq)))
		} else if b == '\r' || b == '\n' {
			if len(s.InputBuffer) > 0 {
				input := string(s.InputBuffer)
				slog.Info("User entered command", "input", input)
				processCommand(s, input)
				s.InputBuffer = s.InputBuffer[:0]
			}
			s.Conn.Write([]byte("\r\n> "))
		} else if b == telnet.ASCII_BS || b == telnet.ASCII_DEL {
			if len(s.InputBuffer) > 0 {
				s.InputBuffer = s.InputBuffer[:len(s.InputBuffer)-1]
				s.Conn.Write([]byte("\b \b"))
			}
		} else if unicode.IsPrint(rune(b)) {
			s.InputBuffer = append(s.InputBuffer, b)
			if s.InPasswordMode {
				s.Conn.Write([]byte("*"))
			} else {
				s.Conn.Write([]byte{b})
			}
			slog.Debug("Buffered input", "buffer", string(s.InputBuffer))
		} else {
			slog.Debug("Received unhandled byte", "byte", b)
		}
	}
}

func processCommand(s *telnet.Session, input string) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "toggle password":
		s.InPasswordMode = !s.InPasswordMode
		status := "off"
		if s.InPasswordMode {
			status = "on"
		}
		s.Conn.Write([]byte(fmt.Sprintf("Password mode %s\r\n", status)))
	default:
		s.Conn.Write([]byte("Unknown command\r\n"))
	}
}

// func requestTerminalType(conn net.Conn) {
// 	seq := []byte{TELNET_IAC, TELNET_SB, TELNET_OPT_TERM_TYPE, 1, TELNET_IAC, TELNET_SE}
// 	if _, err := conn.Write(seq); err != nil {
// 		slog.Warn("Failed to request terminal type", "error", err)
// 	} else {
// 		slog.Debug("Requested terminal type using SB SEND")
// 	}
// }

// func handleSubnegotiation(s *telnet.Session, opt byte, data []byte) {
// 	switch opt {
// 	case TELNET_OPT_TERM_TYPE:
// 		if len(data) > 1 && data[0] == 0 {
// 			s.TerminalType = string(data[1:])
// 			s.ColorLevel = telnet.DetectColorLevel(s.TerminalType)
// 			slog.Info("Received terminal type", "type", s.TerminalType, "color", s.ColorLevel)
// 			slog.Debug("Detected color support level", "terminal", s.TerminalType, "level", s.ColorLevel)
// 		}
// 	case TELNET_OPT_NAWS:
// 		if len(data) >= 4 {
// 			s.Width = int(binary.BigEndian.Uint16(data[:2]))
// 			s.Height = int(binary.BigEndian.Uint16(data[2:4]))
// 			slog.Info("Received terminal window size", "width", s.Width, "height", s.Height)
// 		}
// 	default:
// 		slog.Info("Unhandled subnegotiation option", "option", telnet.DescribeByte(opt), "data", data)
// 	}
// }

// func detectColorLevel(term string) int {
// 	switch strings.ToLower(term) {
// 	case "xterm-256color", "rxvt-unicode-256color", "screen-256color", "mudlet":
// 		return 256
// 	case "xterm", "vt100", "ansi", "linux":
// 		return 16
// 	case "dumb", "unknown":
// 		return 0
// 	default:
// 		if strings.Contains(term, "256") {
// 			return 256
// 		} else if strings.Contains(term, "color") {
// 			return 16
// 		}
// 		return 16
// 	}
// }

// func negotiateTelnet(conn net.Conn) {
// 	writer := bufio.NewWriter(conn)
// 	commands := [][]byte{
// 		{TELNET_IAC, TELNET_WILL, TELNET_OPT_SUPPRESS_GA},
// 		{TELNET_IAC, TELNET_DO, TELNET_OPT_SUPPRESS_GA},
// 		{TELNET_IAC, TELNET_DO, TELNET_OPT_TERM_TYPE},
// 		{TELNET_IAC, TELNET_DO, TELNET_OPT_NAWS},
// 		{TELNET_IAC, TELNET_WILL, TELNET_OPT_ECHO},
// 	}
// 	for _, cmd := range commands {
// 		if _, err := writer.Write(cmd); err != nil {
// 			slog.Warn("Failed to write Telnet IAC", "cmd", telnet.DescribeIAC(cmd), "error", err)
// 			continue
// 		}
// 		slog.Debug("Sent IAC", "cmd", telnet.DescribeIAC(cmd))
// 	}
// 	writer.Flush()
// 	slog.Info("Sent Telnet negotiation commands")
// }

// // func describeIAC(cmd []byte) string {
// 	if len(cmd) < 3 {
// 		return fmt.Sprintf("invalid IAC: %v", cmd)
// 	}
// 	return fmt.Sprintf("IAC %s %s", describeByte(cmd[1]), describeByte(cmd[2]))
// }

// func describeByte(b byte) string {
// 	switch b {
// 	case TELNET_IAC:
// 		return "IAC"
// 	case TELNET_WILL:
// 		return "WILL"
// 	case TELNET_WONT:
// 		return "WONT"
// 	case TELNET_DO:
// 		return "DO"
// 	case TELNET_DONT:
// 		return "DONT"
// 	case TELNET_SB:
// 		return "SB"
// 	case TELNET_SE:
// 		return "SE"
// 	case TELNET_OPT_ECHO:
// 		return "ECHO"
// 	case TELNET_OPT_SUPPRESS_GA:
// 		return "SUPPRESS-GO-AHEAD"
// 	case TELNET_OPT_TERM_TYPE:
// 		return "TERMINAL-TYPE"
// 	case TELNET_OPT_NAWS:
// 		return "NAWS"
// 	default:
// 		return fmt.Sprintf("UNKNOWN(%d)", b)
// 	}
// }
