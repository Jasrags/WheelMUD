package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Jasrags/WheelMUD/internal/cmd"
	"github.com/Jasrags/WheelMUD/internal/db"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/mode"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/tick"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

const (
	defaultListenAddr    = ":2323"
	defaultDBDSN         = "wheelmud.db"
	shutdownDrainTimeout = 10 * time.Second
)

// server bundles the long-lived dependencies a connection needs. New
// dependencies (e.g. character repo, scheduler) belong here so the
// connection-handler signature stays stable.
//
// newInitial is a factory because login mode carries per-connection
// substate (current step, captured username) and must not be shared
// across sessions. Stateless modes (Game) can be reused; the factory
// just returns the same pointer.
type server struct {
	accounts   repo.AccountRepo
	characters repo.CharacterRepo
	rooms      repo.RoomRepo
	exits      repo.ExitRepo
	items      repo.ItemRepo
	mobs       repo.MobInstanceRepo
	sessions   *session.Registry
	scheduler  *tick.Scheduler
	buckets    *tick.Buckets
	bus        *eventbus.Bus
	newInitial func() telnet.Mode

	wg     sync.WaitGroup
	closed chan struct{}
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

	accounts := repo.NewSQLiteAccountRepo(conn)
	characters := repo.NewSQLiteCharacterRepo(conn)
	rooms := repo.NewSQLiteRoomRepo(conn)
	exits := repo.NewSQLiteExitRepo(conn)
	items := repo.NewSQLiteItemRepo(conn)
	mobs := repo.NewSQLiteMobInstanceRepo(conn)

	if err := world.LoadAndSync(context.Background(), conn, world.SourceFS()); err != nil {
		slog.Error("World load failed", "error", err)
		os.Exit(1)
	}

	sessions := session.NewRegistry()
	bus := eventbus.New()

	registry, err := buildRegistry(rooms, exits, items, mobs, characters, sessions, bus)
	if err != nil {
		slog.Error("Failed to build command registry", "error", err)
		os.Exit(1)
	}

	scheduler := tick.New()
	buckets := tick.NewBuckets(scheduler)

	gameMode := mode.NewGame(registry)
	srv := &server{
		accounts:   accounts,
		characters: characters,
		rooms:      rooms,
		exits:      exits,
		items:      items,
		mobs:       mobs,
		sessions:   sessions,
		scheduler:  scheduler,
		buckets:    buckets,
		bus:        bus,
		closed:     make(chan struct{}),
		newInitial: func() telnet.Mode {
			return mode.NewLogin(accounts, characters, sessions, gameMode)
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	scheduler.Start(ctx)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}

	slog.Info("Server started", "address", addr, "db", dsn)

	go func() {
		<-ctx.Done()
		slog.Info("Shutdown signal received, closing listener")
		close(srv.closed)
		_ = ln.Close()
	}()

	srv.acceptLoop(ln)
	srv.shutdown()
}

func (srv *server) acceptLoop(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			select {
			case <-srv.closed:
				return
			default:
			}
			slog.Error("Failed to accept connection", "error", err)
			continue
		}

		s := telnet.NewSession(c)
		if s == nil {
			slog.Error("Failed to create telnet session", "remote", c.RemoteAddr().String())
			c.Close()
			continue
		}

		srv.wg.Add(1)
		go func() {
			defer srv.wg.Done()
			srv.handleConnection(s)
		}()
	}
}

func (srv *server) shutdown() {
	slog.Info("Draining active sessions", "timeout", shutdownDrainTimeout)
	done := make(chan struct{})
	go func() {
		srv.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("All sessions drained")
	case <-time.After(shutdownDrainTimeout):
		slog.Warn("Shutdown drain timed out", "timeout", shutdownDrainTimeout)
	}
	srv.buckets.Stop()
	srv.scheduler.Stop()
	srv.bus.Stop()
	slog.Info("Scheduler stopped")
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

func buildRegistry(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, sessions *session.Registry, bus *eventbus.Bus) (*telnet.Registry, error) {
	r := telnet.NewRegistry()
	if err := r.Register(cmd.Quit, cmd.Who, cmd.Colors); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewAlias(), cmd.NewUnalias()); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewHelp(r)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewLook(rooms, exits, items, mobs)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewMoveFamily(rooms, exits, items, mobs, characters, bus)...); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewTeleport(rooms, exits, items, mobs, characters, sessions)); err != nil {
		return nil, err
	}
	return r, nil
}

func (srv *server) handleConnection(s *telnet.Session) {
	defer s.Conn.Close()
	// Compare-and-delete unbind: if a newer login took over our
	// account, our binding has already been replaced and Unbind is a
	// no-op. Sessions that never authenticated have AccountID == 0
	// and the registry has no entry to remove.
	defer func() {
		if s.AccountID != 0 {
			srv.sessions.Unbind(s.AccountID, s)
		}
	}()
	slog.Info("Client connected", "remote", s.RemoteAddress)

	if err := writeBanner(s); err != nil {
		slog.Debug("Banner write failed", "remote", s.RemoteAddress, "error", err)
		return
	}

	initial := srv.newInitial()
	if err := s.PushMode(initial); err != nil {
		slog.Error("Failed to enter initial mode", "remote", s.RemoteAddress, "error", err)
		return
	}
	if err := s.WriteRaw([]byte(initial.Prompt(s))); err != nil {
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
