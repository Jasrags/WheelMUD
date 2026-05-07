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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/cmd"
	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/group"
	"github.com/Jasrags/WheelMUD/internal/db"
	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/help"
	"github.com/Jasrags/WheelMUD/internal/mob"
	"github.com/Jasrags/WheelMUD/internal/mode"
	"github.com/Jasrags/WheelMUD/internal/news"
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
	audits     repo.AdminAuditRepo
	sessions   *session.Registry
	scheduler  *tick.Scheduler
	buckets    *tick.Buckets
	bus        *eventbus.Bus
	saves      *persist.Manager
	news       *news.Catalog
	chargen    *chargen.Catalog
	combat     *combat.Manager
	groups     *group.Manager
	newInitial func() telnet.Mode

	wg     sync.WaitGroup
	closed chan struct{}

	// stopSignal cancels the root signal-context used by the
	// shutdown-watcher; it is the same callback returned by
	// signal.NotifyContext (also fires on SIGINT/SIGTERM). The
	// shutdown / reboot admin commands invoke it through
	// RequestShutdown so the in-game path takes the same teardown
	// route as a kill -TERM.
	stopSignal context.CancelFunc

	// rebootOnExit, when true, causes main() to syscall.Exec itself
	// after srv.shutdown() returns. Set by RequestReboot.
	rebootOnExit atomic.Bool

	// shutdown coordinator state. shutdownCancel is closed by
	// RequestAbort to interrupt an in-flight countdown.
	shutdownMu      sync.Mutex
	shutdownPending bool
	shutdownCancel  chan struct{}
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
	worldState := repo.NewSQLiteWorldStateRepo(conn)
	audits := repo.NewSQLiteAdminAuditRepo(conn)
	logins := repo.NewSQLiteAccountLoginRepo(conn)
	shops := repo.NewSQLiteShopRepo(conn)
	bankers := repo.NewSQLiteBankerRepo(conn)

	if err := world.LoadAndSync(context.Background(), conn, world.SourceFS()); err != nil {
		slog.Error("World load failed", "error", err)
		os.Exit(1)
	}

	// Auto-derive coords for non-anchor rooms (see migration 0026 +
	// internal/world/coords_derive.go). Runs before listener accept
	// so the very first `look` / `map` / future `zonemap` already
	// sees consistent coords. The summary lands in the boot log so a
	// builder can spot orphans / conflicts without an extra command.
	if summary, err := world.DeriveCoords(context.Background(), rooms, exits); err != nil {
		slog.Error("Coord derivation failed", "error", err)
		os.Exit(1)
	} else {
		slog.Info("Coord derivation complete",
			"anchors", summary.Anchors,
			"synthetic_anchor", summary.SyntheticAnchor,
			"assigned", summary.Assigned,
			"conflicts", len(summary.Conflicts),
			"orphans", len(summary.Orphans))
	}

	baseTicks, err := worldState.GetTicks(context.Background())
	if err != nil {
		slog.Warn("World clock load failed; defaulting to noon", "error", err)
		baseTicks = 675
	}
	clock := world.NewClock(baseTicks)

	sessions := session.NewRegistry()
	bus := eventbus.New()

	newsCatalog, err := news.Load()
	if err != nil {
		slog.Error("Failed to load news catalog", "error", err)
		os.Exit(1)
	}

	helpCatalog, err := help.Load()
	if err != nil {
		slog.Error("Failed to load help catalog", "error", err)
		os.Exit(1)
	}

	chargenFS, err := chargen.SourceFS()
	if err != nil {
		slog.Error("Failed to resolve chargen source", "error", err)
		os.Exit(1)
	}
	chargenCatalog, err := chargen.Load(chargenFS)
	if err != nil {
		slog.Error("Failed to load chargen catalog", "error", err)
		os.Exit(1)
	}
	slog.Info("Chargen catalog loaded",
		"backgrounds", len(chargenCatalog.Backgrounds()),
		"classes", len(chargenCatalog.Classes()),
		"feats", len(chargenCatalog.Feats()),
		"skills", len(chargenCatalog.Skills()),
		"weaves", len(chargenCatalog.Weaves()))

	scheduler := tick.New()
	buckets := tick.NewBuckets(scheduler)

	saves := persist.New()
	saves.Register("character.lastPlayed", func(ctx context.Context) error {
		return savePlayTimes(ctx, sessions, characters)
	})
	saves.Register("world.ticks", func(ctx context.Context) error {
		return worldState.SetTicks(ctx, clock.Ticks())
	})
	buckets.Save.Subscribe(func(ctx context.Context) {
		saves.FlushAll(ctx)
	})

	wanderer := mob.NewWanderHandler(mobs, rooms, exits, mobTemplates, sessions)
	buckets.Wander.Subscribe(wanderer.Tick)

	phaseAmbients := world.NewPhaseAmbientWatcher(clock, rooms, sessions)
	buckets.Phase.Subscribe(phaseAmbients.Tick)

	// §14 shop restocker: refills sub-max stock lines on the
	// AreaReset bucket cadence (5min default).
	restocker := world.NewRestocker(shops)
	buckets.AreaReset.Subscribe(restocker.Tick)

	// §11 / Phase D #16 combat tick spine. Manager owns per-room
	// Fight aggregates, advances Round on every Combat-bucket pulse,
	// and emits CombatStarted / RoundStarted / CombatEnded events.
	// Verbs (#18) and damage math (#17/#18) consume this spine but
	// are not in this slice.
	combatMgr := combat.New(bus, characters, mobs, mobTemplates, items)
	buckets.Combat.Subscribe(combatMgr.Tick)

	// Phase D #22: in-memory party manager. State is process-level
	// and dropped on restart, mirroring the in-flight Fight model.
	// Threaded into the group/follow/attack verbs and the combat
	// XP-split path.
	groups := group.New()
	combatMgr.SetGroupResolver(func(charID, roomID int64) []int64 {
		return groups.MembersInRoom(charID, roomID, sessions)
	})

	// Phase D #19 slice 2: corpse decay. Sweeper deletes corpse rows
	// 5 min after they spawn (constant lives in internal/combat) and
	// emits a "crumble" line via WriteAsync to room peers.
	corpseDecay := combat.NewDecayer(items, func(roomID int64, msg string) {
		for _, peer := range sessions.Snapshot() {
			if peer == nil || peer.CurrentRoomID != roomID {
				continue
			}
			if err := peer.WriteAsync(msg); err != nil {
				slog.Debug("decay: write peer failed", "error", err)
			}
		}
	}, bus)
	combatMgr.SetDecayer(corpseDecay)
	buckets.Decay.Subscribe(corpseDecay.Tick)

	// Phase D #18 follow-up: flee verb. The mover lives in cmd/ so it
	// has access to sessions + RenderRoom; combat invokes it from
	// resolveAction without holding the manager lock. RNG defaults to
	// time-seeded; tests override.
	combatMgr.SetFleeMover(cmd.NewFleeMover(rooms, exits, items, mobs, characters, sessions, bus, clock, nil))

	// Phase D #18 follow-up: combat broadcasts for parry / stance /
	// flee. CombatHit / CombatMiss subscribers landed in #18 slice 1;
	// these three add the new room-visible lines. Each subscriber
	// snapshots names via best-effort repo lookups so a despawned
	// participant still produces readable output.
	combatBroadcast := func(roomID int64, msg string) {
		for _, peer := range sessions.Snapshot() {
			if peer == nil || peer.CurrentRoomID != roomID {
				continue
			}
			if err := peer.WriteAsync(msg); err != nil {
				slog.Debug("combat: broadcast write failed", "error", err)
			}
		}
	}
	eventbus.Subscribe[combat.CombatStance](bus, func(ctx context.Context, ev combat.CombatStance) {
		name := combatActorName(ctx, ev.Actor, characters, mobs)
		switch ev.Kind {
		case "parry":
			combatBroadcast(ev.RoomID, "{{"+name+" raises a weapon to parry.}}::yellow\r\n")
		}
	})
	eventbus.Subscribe[combat.CombatParry](bus, func(ctx context.Context, ev combat.CombatParry) {
		def := combatActorName(ctx, ev.Defender, characters, mobs)
		atk := combatActorName(ctx, ev.Attacker, characters, mobs)
		combatBroadcast(ev.RoomID, "{{"+def+" parries "+atk+"'s blow!}}::cyan\r\n")
	})
	eventbus.Subscribe[combat.CombatFlee](bus, func(ctx context.Context, ev combat.CombatFlee) {
		if ev.Success {
			// Source-room broadcast lands inline from the FleeMover so
			// the third-person line is colocated with the actual move.
			// The CombatFlee subscriber only emits failure feedback so
			// peers see the tense beat ("an orc tries to flee but is
			// cut off!").
			return
		}
		name := combatActorName(ctx, ev.Actor, characters, mobs)
		combatBroadcast(ev.RoomID, "{{"+name+" tries to flee but is cut off!}}::yellow\r\n")
	})

	// srv is constructed before buildRegistry so the shutdown / reboot
	// admin commands can wire to srv as a ShutdownController. newInitial
	// is filled in below once gameMode (which depends on the registry)
	// exists.
	srv := &server{
		accounts:   accounts,
		characters: characters,
		rooms:      rooms,
		exits:      exits,
		items:      items,
		mobs:       mobs,
		audits:     audits,
		sessions:   sessions,
		scheduler:  scheduler,
		buckets:    buckets,
		bus:        bus,
		saves:      saves,
		news:       newsCatalog,
		chargen:    chargenCatalog,
		combat:     combatMgr,
		groups:     groups,
		closed:     make(chan struct{}),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	srv.stopSignal = stop

	registry, err := buildRegistry(rooms, exits, items, mobs, mobTemplates, zones, characters, audits, shops, bankers, sessions, bus, channels, clock, newsCatalog, helpCatalog, chargenCatalog, combatMgr, groups, srv)
	if err != nil {
		slog.Error("Failed to build command registry", "error", err)
		os.Exit(1)
	}

	gameMode := mode.NewGame(registry, characters, rooms, defaultPromptTemplate)
	srv.newInitial = func() telnet.Mode {
		login := mode.NewLogin(accounts, characters, sessions, gameMode)
		login.SetMOTD(func(s *telnet.Session, lastSeen time.Time) error {
			return newsCatalog.WriteMOTDBlock(s, lastSeen)
		})
		login.SetCatalog(chargenCatalog)
		// Slice 1b: account-menu needs item + audit repos to cascade
		// owned items on character delete and to record an
		// account-mode audit row. sessions is already on Login for the
		// single-session policy; AccountMenu reuses it for the
		// live-session check.
		login.SetItems(items)
		login.SetAudits(audits)
		// Slice 4: account-menu security view + per-login audit log.
		login.SetLogins(logins)
		return login
	}

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

	// Reboot path: re-exec ourselves so a `reboot` admin command
	// brings the server back up without operator intervention. The
	// listener is already closed (free port), DB connections were
	// closed by the deferred closeDB, and the persist manager has
	// flushed. POSIX-only — Windows admins use `shutdown` with an
	// external supervisor.
	if srv.rebootOnExit.Load() {
		bin, err := os.Executable()
		if err != nil {
			// A silent fallback to os.Args[0] would re-exec a path
			// that may no longer resolve (relative path + cwd
			// change, replaced symlink). Fail loud so the operator
			// can diagnose; supervisord/systemd will restart us.
			slog.Error("Reboot: cannot resolve executable; aborting re-exec", "error", err)
			os.Exit(1)
		}
		slog.Info("Re-execing for reboot", "bin", bin, "args", os.Args)
		if err := syscall.Exec(bin, os.Args, os.Environ()); err != nil {
			slog.Error("Re-exec failed", "error", err)
			os.Exit(1)
		}
	}
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
	// End every active fight so subscribers (eventbus, future
	// HP-persist) see CombatEnded before the bus stops below.
	// Bounded context matches the saves.FlushAll budget — a
	// misbehaving subscriber must not be able to stall shutdown.
	if srv.combat != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		srv.combat.Stop(ctx)
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
		charID, _, _ := s.InWorld()
		if charID == 0 {
			continue
		}
		if err := characters.RecordPlay(ctx, charID, now); err != nil {
			slog.Warn("autosave: RecordPlay failed", "char", charID, "error", err)
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

// combatActorName resolves an ActorRef to a display name for combat
// broadcasts. Best-effort: a despawned/logged-out participant falls
// back to "Someone" / "A creature" so the broadcast still reads.
// Routes through display.Defang so a builder-authored mob name (or
// future loosened character-name policy) can't smuggle cfmt markers
// or control bytes into a {{...}} broadcast.
func combatActorName(ctx context.Context, ref combat.ActorRef, chars repo.CharacterRepo, mobs repo.MobInstanceRepo) string {
	switch ref.Kind {
	case combat.ActorKindCharacter:
		ch, err := chars.GetByID(ctx, ref.ID)
		if err == nil {
			return display.Defang(ch.Core.Name, "Someone")
		}
		return "Someone"
	case combat.ActorKindMob:
		mob, err := mobs.GetByID(ctx, ref.ID)
		if err == nil {
			return display.Defang(mob.Core.Name, "A creature")
		}
		return "A creature"
	}
	return "Someone"
}

func closeDB(conn *sql.DB) {
	if err := conn.Close(); err != nil {
		slog.Warn("DB close error", "error", err)
	}
}

func buildRegistry(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, mobTemplates repo.MobTemplateRepo, zones repo.ZoneRepo, characters repo.CharacterRepo, audits repo.AdminAuditRepo, shops repo.ShopRepo, bankers repo.BankerRepo, sessions *session.Registry, bus *eventbus.Bus, channels []repo.Channel, clock *world.Clock, newsCatalog *news.Catalog, helpCatalog *help.Catalog, chargenCatalog *chargen.Catalog, combatMgr *combat.Manager, groups *group.Manager, shutdownCtl cmd.ShutdownController) (*telnet.Registry, error) {
	r := telnet.NewRegistry()
	if err := r.Register(cmd.Quit, cmd.Colors); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewWho(sessions, characters),
		cmd.NewSay(sessions, rooms),
		cmd.NewShout(sessions, rooms),
		cmd.NewYell(sessions, rooms),
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
	if err := r.Register(cmd.NewPvP(characters)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewHelp(r, helpCatalog)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewLook(rooms, exits, items, mobs, clock)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewExamine(items, mobs)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewMoveFamily(rooms, exits, items, mobs, characters, bus, clock, sessions)...); err != nil {
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
	if err := r.Register(cmd.NewTeleport(rooms, exits, items, mobs, characters, sessions, clock, audits)); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewGoto(rooms, exits, items, mobs, characters, sessions, clock, audits),
		cmd.NewTransfer(rooms, exits, items, mobs, characters, sessions, clock, audits),
		cmd.NewSummon(rooms, exits, items, mobs, characters, sessions, clock, audits),
		cmd.NewWizinvis(audits),
		cmd.NewShutdown(shutdownCtl, audits),
		cmd.NewReboot(shutdownCtl, audits),
	); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewInventory(items, characters),
		cmd.NewGet(items, characters, sessions),
		cmd.NewDrop(items, characters, sessions),
		cmd.NewGive(items, characters, sessions),
		cmd.NewPut(items, characters, sessions),
		cmd.NewWear(items, characters, sessions),
		cmd.NewWield(items, characters, sessions),
		cmd.NewRemove(items, characters, sessions),
		cmd.NewEquipment(items, characters),
		cmd.NewSpawn(items, mobTemplates, mobs, characters, sessions, audits),
	); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewAttack(combatMgr, rooms, mobs, characters, sessions, groups)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewGroup(groups, sessions)); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewFollow(groups, sessions),
		cmd.NewUnfollow(),
	); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewFlee(combatMgr),
		cmd.NewParry(combatMgr, characters, sessions),
	); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewList(items, mobs, mobTemplates, shops, clock),
		cmd.NewBuy(items, characters, mobs, mobTemplates, shops, clock, sessions),
		cmd.NewSell(items, characters, mobs, mobTemplates, shops, clock, sessions),
		cmd.NewValue(items, mobs, mobTemplates, shops, clock),
		cmd.NewBalance(characters, mobs, mobTemplates, bankers, clock),
		cmd.NewDeposit(characters, mobs, mobTemplates, bankers, clock, audits),
		cmd.NewWithdraw(characters, mobs, mobTemplates, bankers, clock, audits),
	); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewMap(rooms, exits, zones)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewWhereAmI(rooms, clock)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewTime(clock)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewTrack(mobs, rooms, exits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewZones(zones, rooms)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewCoords(rooms, exits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewZoneMap(rooms, exits, zones)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewNews(newsCatalog, characters)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewScore(characters, chargenCatalog)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewXP(characters)); err != nil {
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
		// Phase D #22: drop any party membership / pending invite
		// the disconnecting character was party to. Leader-leaves
		// disbands; a member departure shrinks. No-op for guests.
		if s.CharacterID != 0 && srv.groups != nil {
			srv.groups.ClearForCharacter(s.CharacterID)
		}
	}()
	slog.Info("Client connected", "remote", s.RemoteAddress)

	if err := srv.news.WriteSplash(s); err != nil {
		slog.Debug("Splash write failed", "remote", s.RemoteAddress, "error", err)
		return
	}

	initial := srv.newInitial()
	if err := s.PushMode(initial); err != nil {
		slog.Error("Failed to enter initial mode", "remote", s.RemoteAddress, "error", err)
		return
	}
	if err := s.WriteRaw([]byte(initial.Prompt(s.Context(), s))); err != nil {
		return
	}

	if err := telnet.RunSession(s); err != nil && !errors.Is(err, net.ErrClosed) {
		slog.Debug("Session ended", "remote", s.RemoteAddress, "error", err)
	}
	slog.Info("Client disconnected", "remote", s.RemoteAddress)
}

// RequestShutdown schedules a graceful shutdown after delay, with an
// optional reason broadcast to all sessions during the countdown.
// Returns cmd.ErrShutdownPending if a countdown is already in flight.
//
// Implements cmd.ShutdownController.
func (srv *server) RequestShutdown(reason string, delay time.Duration) error {
	return srv.scheduleStop(reason, delay, false)
}

// RequestReboot is RequestShutdown plus rebootOnExit, so main()
// re-execs the binary after the drain/flush sequence.
//
// Implements cmd.ShutdownController.
func (srv *server) RequestReboot(reason string, delay time.Duration) error {
	return srv.scheduleStop(reason, delay, true)
}

// RequestAbort cancels an in-flight countdown. Returns an error
// (not nil-terminated) if no countdown is pending so the operator
// gets explicit feedback rather than silent acceptance.
//
// Implements cmd.ShutdownController.
func (srv *server) RequestAbort() error {
	srv.shutdownMu.Lock()
	if !srv.shutdownPending {
		srv.shutdownMu.Unlock()
		return errors.New("no shutdown pending")
	}
	cancel := srv.shutdownCancel
	srv.shutdownPending = false
	srv.shutdownCancel = nil
	srv.rebootOnExit.Store(false)
	srv.shutdownMu.Unlock()

	close(cancel)
	srv.broadcast("{{*** Shutdown cancelled. ***}}::green")
	return nil
}

func (srv *server) scheduleStop(reason string, delay time.Duration, reboot bool) error {
	srv.shutdownMu.Lock()
	if srv.shutdownPending {
		srv.shutdownMu.Unlock()
		return cmd.ErrShutdownPending
	}
	cancel := make(chan struct{})
	srv.shutdownPending = true
	srv.shutdownCancel = cancel
	// Stamp rebootOnExit while still holding shutdownMu so a racing
	// RequestAbort that runs between this unlock and the goroutine
	// spawn cannot leave a stale `true` set after the abort cleared
	// it.
	if reboot {
		srv.rebootOnExit.Store(true)
	}
	srv.shutdownMu.Unlock()

	verb := "Shutdown"
	if reboot {
		verb = "Reboot"
	}
	srv.announceCountdownStart(verb, reason, delay)

	safego.Go("shutdown-countdown", func() {
		srv.runCountdown(verb, reason, delay, cancel)
	})
	return nil
}

func (srv *server) announceCountdownStart(verb, reason string, delay time.Duration) {
	msg := verb + " in " + delay.Round(time.Second).String() + "."
	if reason != "" {
		msg = verb + " in " + delay.Round(time.Second).String() + ": " + reason
	}
	srv.broadcast("{{*** " + msg + " ***}}::yellow")
}

// runCountdown sleeps the remaining delay in chunks, broadcasting at
// the standard {60,30,10,5..0}s marks. Returns early if cancel fires.
// On natural completion it triggers stopSignal, which feeds into the
// existing shutdown-watcher path.
func (srv *server) runCountdown(verb, reason string, delay time.Duration, cancel <-chan struct{}) {
	marks := []time.Duration{
		60 * time.Second, 30 * time.Second, 10 * time.Second,
		5 * time.Second, 4 * time.Second, 3 * time.Second,
		2 * time.Second, 1 * time.Second,
	}
	deadline := time.Now().Add(delay)

	for _, m := range marks {
		if m >= delay {
			continue
		}
		wait := time.Until(deadline.Add(-m))
		if wait <= 0 {
			continue
		}
		select {
		case <-time.After(wait):
		case <-cancel:
			return
		}
		tag := verb + " in " + m.String() + "."
		if reason != "" {
			tag = verb + " in " + m.String() + ": " + reason
		}
		srv.broadcast("{{*** " + tag + " ***}}::yellow")
	}

	// Sleep any remaining tail to the deadline.
	if rem := time.Until(deadline); rem > 0 {
		select {
		case <-time.After(rem):
		case <-cancel:
			return
		}
	}

	srv.broadcast("{{*** " + verb + " now. ***}}::red")

	// Mark the request as no longer pending before triggering the
	// stop signal. The pending guard is only there to reject a
	// second concurrent shutdown, not to gate teardown.
	srv.shutdownMu.Lock()
	srv.shutdownPending = false
	srv.shutdownCancel = nil
	srv.shutdownMu.Unlock()

	if srv.stopSignal != nil {
		srv.stopSignal()
	}
}

// broadcast sends msg to every live session via WriteAsync (the only
// safe cross-session write path; see CLAUDE.md). Failures are logged
// at Debug — a closed connection is not a coordinator-level error.
func (srv *server) broadcast(msg string) {
	for _, s := range srv.sessions.Snapshot() {
		if err := s.WriteAsync(msg); err != nil {
			slog.Debug("shutdown broadcast: write failed",
				"session", s.RemoteAddress, "error", err)
		}
	}
}
