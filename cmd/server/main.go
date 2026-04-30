package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net"
	"os"

	"github.com/Jasrags/WheelMUD/internal/cmd"
	"github.com/Jasrags/WheelMUD/internal/db"
	"github.com/Jasrags/WheelMUD/internal/mode"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

const (
	defaultListenAddr = ":2323"
	defaultDBDSN      = "wheelmud.db"
)

// server bundles the long-lived dependencies a connection needs. New
// dependencies (e.g. character repo, scheduler) belong here so the
// connection-handler signature stays stable.
type server struct {
	accounts repo.AccountRepo
	initial  telnet.Mode // pushed onto every new session; today: gameMode
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	addr := envOr("LISTEN_ADDR", defaultListenAddr)
	dsn := envOr("DB_DSN", defaultDBDSN)

	conn, err := db.Open(context.Background(), dsn)
	if err != nil {
		slog.Error("Failed to open database", "dsn", dsn, "error", err)
		os.Exit(1)
	}
	defer closeDB(conn)

	registry, err := buildRegistry()
	if err != nil {
		slog.Error("Failed to build command registry", "error", err)
		os.Exit(1)
	}

	srv := &server{
		accounts: repo.NewSQLiteAccountRepo(conn),
		initial:  mode.NewGame(registry),
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
	defer ln.Close()

	slog.Info("Server started", "address", addr, "db", dsn)

	for {
		c, err := ln.Accept()
		if err != nil {
			slog.Error("Failed to accept connection", "error", err)
			continue
		}

		session := telnet.NewSession(c)
		if session == nil {
			slog.Error("Failed to create telnet session", "remote", c.RemoteAddr().String())
			c.Close()
			continue
		}

		go srv.handleConnection(session)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func closeDB(conn *sql.DB) {
	if err := conn.Close(); err != nil {
		slog.Warn("DB close error", "error", err)
	}
}

func buildRegistry() (*telnet.Registry, error) {
	r := telnet.NewRegistry()
	if err := r.Register(cmd.Quit, cmd.Who, cmd.TogglePassword, cmd.Colors); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewHelp(r)); err != nil {
		return nil, err
	}
	return r, nil
}

func (srv *server) handleConnection(s *telnet.Session) {
	defer s.Conn.Close()
	slog.Info("Client connected", "remote", s.RemoteAddress)

	if err := writeBanner(s); err != nil {
		slog.Debug("Banner write failed", "remote", s.RemoteAddress, "error", err)
		return
	}

	if err := s.PushMode(srv.initial); err != nil {
		slog.Error("Failed to enter initial mode", "remote", s.RemoteAddress, "error", err)
		return
	}
	if err := s.WriteRaw([]byte(srv.initial.Prompt(s))); err != nil {
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
