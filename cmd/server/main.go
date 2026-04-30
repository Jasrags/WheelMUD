package main

import (
	"errors"
	"log/slog"
	"net"
	"os"

	"github.com/Jasrags/WheelMUD/internal/cmd"
	"github.com/Jasrags/WheelMUD/internal/mode"
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

	registry, err := buildRegistry()
	if err != nil {
		slog.Error("Failed to build command registry", "error", err)
		os.Exit(1)
	}
	gameMode := mode.NewGame(registry)

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

		go handleConnection(session, gameMode)
	}
}

func buildRegistry() (*telnet.Registry, error) {
	r := telnet.NewRegistry()
	if err := r.Register(cmd.Quit, cmd.Who, cmd.TogglePassword); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewHelp(r)); err != nil {
		return nil, err
	}
	return r, nil
}

func handleConnection(s *telnet.Session, initialMode telnet.Mode) {
	defer s.Conn.Close()
	slog.Info("Client connected", "remote", s.RemoteAddress)

	if err := writeBanner(s); err != nil {
		slog.Debug("Banner write failed", "remote", s.RemoteAddress, "error", err)
		return
	}

	if err := s.PushMode(initialMode); err != nil {
		slog.Error("Failed to enter initial mode", "remote", s.RemoteAddress, "error", err)
		return
	}
	if err := s.WriteRaw([]byte(initialMode.Prompt(s))); err != nil {
		return
	}

	if err := telnet.RunSession(s); err != nil && !errors.Is(err, net.ErrClosed) {
		slog.Debug("Session ended", "remote", s.RemoteAddress, "error", err)
	}
	slog.Info("Client disconnected", "remote", s.RemoteAddress)
}

func writeBanner(s *telnet.Session) error {
	if err := s.WriteString("Welcome to the Telnet server!\r\n"); err != nil {
		return err
	}
	if err := s.WriteString("{{Welcome}}::green|bold\r\n"); err != nil {
		return err
	}
	return s.WriteString("{{Underlined Bold}}::underline|bold\r\n")
}
