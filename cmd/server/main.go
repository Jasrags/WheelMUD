package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	"github.com/Jasrags/WheelMUD/telnet"
)

const defaultListenAddr = ":2323"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = defaultListenAddr
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
	defer ln.Close()

	slog.Info("Server started", "address", addr)

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

	if err := s.WriteString("Welcome to the Telnet server!\r\n"); err != nil {
		slog.Debug("Welcome write failed", "remote", s.RemoteAddress, "error", err)
		return
	}
	if err := s.WriteString("{{Welcome}}::green|bold\r\n"); err != nil {
		return
	}
	if err := s.WriteString("{{Underlined Bold}}::underline|bold\r\n"); err != nil {
		return
	}
	if err := s.WriteRaw([]byte("> ")); err != nil {
		return
	}

	if err := telnet.RunSession(s, processCommand); err != nil {
		// EOF and idle timeout are expected; log others at debug level.
		if !errors.Is(err, net.ErrClosed) {
			slog.Debug("Session ended", "remote", s.RemoteAddress, "error", err)
		}
	}
	slog.Info("Client disconnected", "remote", s.RemoteAddress)
}

func processCommand(s *telnet.Session, input string) error {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "toggle password":
		s.InPasswordMode = !s.InPasswordMode
		status := "off"
		if s.InPasswordMode {
			status = "on"
		}
		return s.WriteRaw([]byte(fmt.Sprintf("Password mode %s\r\n", status)))
	default:
		return s.WriteRaw([]byte("Unknown command\r\n"))
	}
}
