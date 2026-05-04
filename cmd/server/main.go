package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Jasrags/WheelMUD/internal/cmd"
	"github.com/Jasrags/WheelMUD/internal/db"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/mob"
	"github.com/Jasrags/WheelMUD/internal/mode"
	"github.com/Jasrags/WheelMUD/internal/persist"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/safego"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/tick"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

const (
	defaultListenAddr     = ":2323"
	defaultDBDSN          = "wheelmud.db"
	shutdownDrainTimeout  = 10 * time.Second
	defaultPromptTemplate = "<%h/%H hp> "
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
	saves      *persist.Manager
	newInitial func() telnet.Mode

	wg     sync.WaitGroup
	closed chan struct{}
}

func main() {
	rawLevel := envOr("LOG_LEVEL", "debug")
	level, levelOK := parseLogLevel(rawLevel)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)
	if !levelOK {
		// Warn through the now-configured handler so format / level
		// match the rest of startup logging.
		slog.Warn("LOG_LEVEL: unknown value, defaulting to info", "value", rawLevel)
	}

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
	mobTemplates := repo.NewSQLiteMobTemplateRepo(conn)
	zones := repo.NewSQLiteZoneRepo(conn)
	channelRepo := repo.NewSQLiteChannelRepo(conn)
	channels, err := channelRepo.List(context.Background())
	if err != nil {
		slog.Error("Failed to load channel catalog", "error", err)
		os.Exit(1)
	}

	if err := world.LoadAndSync(context.Background(), conn, world.SourceFS()); err != nil {
		slog.Error("World load failed", "error", err)
		os.Exit(1)
	}

	sessions := session.NewRegistry()
	bus := eventbus.New()

	registry, err := buildRegistry(rooms, exits, items, mobs, zones, characters, sessions, bus, channels)
	if err != nil {
		slog.Error("Failed to build command registry", "error", err)
		os.Exit(1)
	}

	scheduler := tick.New()
	buckets := tick.NewBuckets(scheduler)

	saves := persist.New()
	saves.Register("character.lastPlayed", func(ctx context.Context) error {
		return savePlayTimes(ctx, sessions, characters)
	})
	buckets.Save.Subscribe(func(ctx context.Context) {
		saves.FlushAll(ctx)
	})

	wanderer := mob.NewWanderHandler(mobs, rooms, exits, mobTemplates, sessions)
	buckets.Wander.Subscribe(wanderer.Tick)

	gameMode := mode.NewGame(registry, characters, rooms, defaultPromptTemplate)
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
		saves:      saves,
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

	safego.Go("shutdown-watcher", func() {
		<-ctx.Done()
		slog.Info("Shutdown signal received, closing listener")
		close(srv.closed)
		_ = ln.Close()
	})

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
		safego.Go("session-"+s.RemoteAddress, func() {
			defer srv.wg.Done()
			srv.handleConnection(s)
		})
	}
}

func (srv *server) shutdown() {
	slog.Info("Draining active sessions", "timeout", shutdownDrainTimeout)
	done := make(chan struct{})
	safego.Go("shutdown-drain", func() {
		srv.wg.Wait()
		close(done)
	})
	select {
	case <-done:
		slog.Info("All sessions drained")
	case <-time.After(shutdownDrainTimeout):
		slog.Warn("Shutdown drain timed out", "timeout", shutdownDrainTimeout)
	}
	// Final autosave pass before stopping the scheduler. Sessions
	// have either drained (so their CurrentRoomID is already on
	// disk via RecordRoom) or been killed by the timeout — either
	// way, FlushAll captures any remaining last_played_at /
	// future-dirty-bit state under a hard 5s budget so a slow
	// repo can't hang shutdown.
	if srv.saves != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		srv.saves.FlushAll(ctx)
		cancel()
	}
	srv.buckets.Stop()
	srv.scheduler.Stop()
	srv.bus.Stop()
	slog.Info("Scheduler stopped")
}

// savePlayTimes is the concrete saver registered with the persist
// manager: it walks every authenticated session and stamps its
// character's last_played_at to now. Idempotent and bounded.
//
// Today this is the only thing the autosave loop does, because
// rooms / items / mobs / character core all already write through
// on every mutation. As combat (§11) and weave-resolution (§12)
// land, they'll register additional savers (dirty mob HP, dirty
// character HP, dirty affect tick state) on the same Manager.
func savePlayTimes(ctx context.Context, sessions *session.Registry, characters repo.CharacterRepo) error {
	now := time.Now().UTC()
	count := 0
	for _, s := range sessions.Snapshot() {
		if s.CharacterID == 0 {
			continue
		}
		if err := characters.RecordPlay(ctx, s.CharacterID, now); err != nil {
			slog.Warn("autosave: RecordPlay failed", "char", s.CharacterID, "error", err)
			continue
		}
		count++
	}
	if count > 0 {
		slog.Debug("autosave: last_played_at refreshed", "characters", count)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseLogLevel maps a LOG_LEVEL env value to slog.Level. Returns
// ok=false on unknown values so the caller can warn through the
// already-configured handler rather than the default one.
// Case-insensitive; "debug"/"info"/"warn"/"error".
func parseLogLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}

func closeDB(conn *sql.DB) {
	if err := conn.Close(); err != nil {
		slog.Warn("DB close error", "error", err)
	}
}

func buildRegistry(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, zones repo.ZoneRepo, characters repo.CharacterRepo, sessions *session.Registry, bus *eventbus.Bus, channels []repo.Channel) (*telnet.Registry, error) {
	r := telnet.NewRegistry()
	if err := r.Register(cmd.Quit, cmd.Colors); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewWho(sessions),
		cmd.NewSay(sessions, rooms),
		cmd.NewTell(sessions),
		cmd.NewReply(sessions),
	); err != nil {
		return nil, err
	}
	for _, ch := range channels {
		if err := r.Register(cmd.NewChannel(ch, sessions, characters)); err != nil {
			return nil, err
		}
	}
	if err := r.Register(cmd.NewChannelsList(channels)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewAlias(), cmd.NewUnalias()); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewPrompt(characters, defaultPromptTemplate)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewHelp(r)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewLook(rooms, exits, items, mobs)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewExamine(items, mobs)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewMoveFamily(rooms, exits, items, mobs, characters, bus)...); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewOpen(exits, sessions),
		cmd.NewClose(exits, sessions),
		cmd.NewLock(exits, items, sessions),
		cmd.NewUnlock(exits, items, sessions),
		cmd.NewPick(exits, sessions),
	); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewTeleport(rooms, exits, items, mobs, characters, sessions)); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewInventory(items, characters),
		cmd.NewGet(items, characters, sessions),
		cmd.NewDrop(items, characters, sessions),
		cmd.NewGive(items, characters, sessions),
	); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewMap(rooms, exits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewWhereAmI(rooms)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewTrack(mobs, rooms, exits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewZones(zones, rooms)); err != nil {
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
