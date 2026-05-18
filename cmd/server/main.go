package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
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
	"github.com/Jasrags/WheelMUD/internal/config"
	"github.com/Jasrags/WheelMUD/internal/db"
	"github.com/Jasrags/WheelMUD/internal/effects"
	"github.com/Jasrags/WheelMUD/internal/emote"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/gmcp"
	"github.com/Jasrags/WheelMUD/internal/group"
	"github.com/Jasrags/WheelMUD/internal/help"
	luaeng "github.com/Jasrags/WheelMUD/internal/lua"
	"github.com/Jasrags/WheelMUD/internal/metrics"
	"github.com/Jasrags/WheelMUD/internal/mob"
	"github.com/Jasrags/WheelMUD/internal/mode"
	"github.com/Jasrags/WheelMUD/internal/news"
	"github.com/Jasrags/WheelMUD/internal/persist"
	"github.com/Jasrags/WheelMUD/internal/quest"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/safego"
	"github.com/Jasrags/WheelMUD/internal/scripts"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/tick"
	"github.com/Jasrags/WheelMUD/internal/trigger"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"

	luastd "github.com/yuin/gopher-lua"
)

const (
	shutdownDrainTimeout  = 10 * time.Second
	defaultPromptTemplate = "<%h/%H hp> "
)

// Build metadata stamped at link time via goreleaser ldflags (Phase J
// slice J7). Empty when built without ldflags (e.g. `go build`); the
// build_info metric still emits an empty-label series in that case so
// scrapers don't see a missing target.
var (
	buildVersion = ""
	buildCommit  = ""
	buildDate    = ""
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
	accounts    repo.AccountRepo
	characters  repo.CharacterRepo
	rooms       repo.RoomRepo
	exits       repo.ExitRepo
	items       repo.ItemRepo
	mobs        repo.MobInstanceRepo
	audits      repo.AdminAuditRepo
	sessions    *session.Registry
	scheduler   *tick.Scheduler
	buckets     *tick.Buckets
	bus         *eventbus.Bus
	saves       *persist.Manager
	news        *news.Catalog
	chargen     *chargen.Catalog
	combat      *combat.Manager
	groups      *group.Manager
	triggers    *trigger.Dispatcher
	quest       *quest.Engine
	luaRunner   *luaeng.Runner
	gmcp        *gmcp.Manager
	metrics     *metrics.Metrics
	metricsHTTP *http.Server
	badInput    *telnet.BadInputTracker
	newInitial  func() telnet.Mode

	// cfg is the resolved YAML+env config; the MSSP block is read on
	// every DO MSSP from a crawler so changes via SIGHUP (if/when that
	// lands) would take effect on the next response.
	cfg config.Config

	// startedAt is the wall-clock at which the server began accepting
	// connections. Surfaced as MSSP UPTIME (unix seconds) — crawlers
	// compute the live duration on read.
	startedAt time.Time

	// worldStats is a boot-time snapshot of the world counts surfaced
	// via MSSP. See msspWorldStats.
	worldStats msspWorldStats

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
	configPath := flag.String("config", "", "Path to YAML config file (optional; env vars still apply)")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("wheelmud version=%s commit=%s date=%s\n",
			versionOr(buildVersion, "dev"),
			versionOr(buildCommit, "none"),
			versionOr(buildDate, "unknown"))
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		// Log via the default handler — slog isn't configured yet.
		slog.Error("Config load failed", "path", *configPath, "error", err)
		os.Exit(1)
	}

	level, levelOK := parseLogLevel(cfg.Log.Level)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)
	if !levelOK {
		// Warn through the now-configured handler so format / level
		// match the rest of startup logging.
		slog.Warn("log.level: unknown value, defaulting to info", "value", cfg.Log.Level)
	}

	addr := cfg.Server.ListenAddr
	dsn := cfg.DB.DSN

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
	commandAudits := repo.NewSQLiteCharacterAuditRepo(conn)
	logins := repo.NewSQLiteAccountLoginRepo(conn)
	shops := repo.NewSQLiteShopRepo(conn)
	bankers := repo.NewSQLiteBankerRepo(conn)
	trainers := repo.NewSQLiteTrainerRepo(conn)
	weaveTeachers := repo.NewSQLiteWeaveTeacherRepo(conn)
	builderZones := repo.NewSQLiteBuilderZoneRepo(conn)

	// Boot-time data integrity audit: a character row with auth_level
	// outside [0, AuthLevelMax] would later trip the post-load scan
	// validator with "invalid auth_level <N>" and lock the owning
	// account out of character select. The schema's CHECK constraint
	// (migration 0019) forbids this under normal code paths, but a
	// hand-edited row or a row that predates the constraint can still
	// be present. Clamp + warn so a fresh boot recovers instead of
	// requiring manual SQL.
	if clamped, err := characters.ClampInvalidAuthLevels(context.Background()); err != nil {
		slog.Warn("auth_level audit: clamp failed", "error", err)
	} else if clamped > 0 {
		slog.Warn("auth_level audit: clamped out-of-range rows",
			"rows", clamped, "ceiling", repo.AuthLevelMax)
	}

	loaded, err := world.LoadAndSync(context.Background(), conn, world.SourceFS())
	if err != nil {
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

	// Phase F #32 slice 5b — wire the login publisher hook so
	// promoteToGame publishes world.PlayerLoggedIn after each
	// character finishes promotion. Single-shot setter; tests
	// leave it nil. Wired before any listener starts so the first
	// login publishes correctly.
	//
	// Phase F #32a polish: also publish a PlayerEntered event with
	// FromRoomID=0 so existing on_enter triggers fire on login
	// without a separate trigger event kind. Greeter-style mobs
	// authored against on_enter now react to "any time a player
	// appears in my room" — login OR movement — with one rule
	// rather than two. The FromRoomID=0 sentinel lets handlers
	// distinguish a login-spawn from a movement-arrival if needed.
	mode.SetLoginPublisher(func(characterID, roomID int64) {
		ctx := context.Background()
		bus.Publish(ctx, world.PlayerLoggedIn{
			CharacterID: characterID,
			RoomID:      roomID,
		})
		bus.Publish(ctx, world.PlayerEntered{
			CharacterID: characterID,
			FromRoomID:  0, // sentinel: came from "nowhere" (login spawn)
			ToRoomID:    roomID,
		})
	})

	newsCatalog, err := news.Load()
	if err != nil {
		slog.Error("Failed to load news catalog", "error", err)
		os.Exit(1)
	}

	helpFS, err := help.SourceFS()
	if err != nil {
		slog.Error("Failed to resolve help source", "error", err)
		os.Exit(1)
	}
	helpCatalog, err := help.LoadFS(helpFS)
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

	effectsFS, err := effects.SourceFS()
	if err != nil {
		slog.Error("Failed to resolve effects source", "error", err)
		os.Exit(1)
	}
	effectsCatalog, err := effects.Load(effectsFS)
	if err != nil {
		slog.Error("Failed to load effects catalog", "error", err)
		os.Exit(1)
	}
	slog.Info("Effects catalog loaded", "effects", len(effectsCatalog.IDs()))

	emoteFS, err := emote.SourceFS()
	if err != nil {
		slog.Error("Failed to resolve emote source", "error", err)
		os.Exit(1)
	}
	emoteCatalog, err := emote.Load(emoteFS)
	if err != nil {
		slog.Error("Failed to load emote catalog", "error", err)
		os.Exit(1)
	}
	slog.Info("Emote catalog loaded", "socials", len(emoteCatalog.IDs()))

	// Cross-validate consumable EffectIDs against the loaded effects
	// catalog so a typo in `effect_id_string:` fails the boot loudly
	// instead of fizzling silently at quaff time.
	if err := validateConsumableEffectRefs(loaded, effectsCatalog); err != nil {
		slog.Error("Consumable effect refs invalid", "error", err)
		os.Exit(1)
	}

	questFS, err := quest.SourceFS()
	if err != nil {
		slog.Error("Failed to resolve quest source", "error", err)
		os.Exit(1)
	}
	questCatalog, err := quest.Load(questFS)
	if err != nil {
		slog.Error("Failed to load quest catalog", "error", err)
		os.Exit(1)
	}
	slog.Info("Quest catalog loaded", "quests", len(questCatalog.ByID))

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

	// §7 Area/zone reset pipeline: per-zone tick that respawns
	// missing mob populations, restores authored door state, and
	// recreates any authored items that have left the world (sold,
	// destroyed). Empty-mode zones consult the session registry via
	// zoneOccupied so a reset can't tick on a fight in progress.
	zoneResetter := world.NewZoneResetter(
		zones, mobTemplates, mobs, rooms, exits, items,
		loaded.ItemSpecsByZone,
		world.OccupancyCheckerFunc(func(ctx context.Context, zoneID int64) bool {
			for _, sess := range sessions.Snapshot() {
				roomID := sess.CurrentRoomID
				if roomID == 0 {
					continue
				}
				room, err := rooms.FindByID(ctx, roomID)
				if err != nil {
					continue
				}
				if room.ZoneID == zoneID {
					return true
				}
			}
			return false
		}))
	buckets.AreaReset.Subscribe(zoneResetter.Tick)

	// §11 / Phase D #16 combat tick spine. Manager owns per-room
	// Fight aggregates, advances Round on every Combat-bucket pulse,
	// and emits CombatStarted / RoundStarted / CombatEnded events.
	// Verbs (#18) and damage math (#17/#18) consume this spine but
	// are not in this slice.
	combatMgr := combat.New(bus, characters, mobs, mobTemplates, items)
	// Phase L slice 65: feat-driven cadence modifiers. The Manager
	// reads the chargen catalog every actorActionCost call to resolve
	// FeatModifiers off ch.Feats — nil-safe so combat-only tests can
	// skip it.
	combatMgr.SetCatalog(chargenCatalog)
	buckets.Combat.Subscribe(combatMgr.Tick)

	// Phase D #22: in-memory party manager. State is process-level
	// and dropped on restart, mirroring the in-flight Fight model.
	// Threaded into the group/follow/attack verbs and the combat
	// XP-split path.
	groups := group.New()
	combatMgr.SetGroupResolver(func(charID, roomID int64) []int64 {
		return groups.MembersInRoom(charID, roomID, sessions)
	})

	// Phase D #18 follow-up: flee verb. The mover lives in cmd/ so it
	// has access to sessions + RenderRoom; combat invokes it from
	// resolveAction without holding the manager lock. RNG defaults to
	// time-seeded; tests override.
	combatMgr.SetFleeMover(cmd.NewFleeMover(rooms, exits, items, mobs, characters, sessions, bus, clock, nil))

	// Phase D §19 closer: drop-on-death toggle. When enabled, a dying
	// player's inventory, equipment, and carried coin spill into a
	// player-corpse and the 10% XP-debt is waived. Default false.
	combatMgr.SetDropOnDeath(cfg.Combat.DropOnDeath)

	setupCombatSubscribers(combatSubscriberDeps{
		bus:        bus,
		sessions:   sessions,
		characters: characters,
		mobs:       mobs,
		items:      items,
		rooms:      rooms,
		exits:      exits,
		clock:      clock,
	})

	setupTickers(tickerDeps{
		sessions:       sessions,
		combatMgr:      combatMgr,
		characters:     characters,
		items:          items,
		chargenCatalog: chargenCatalog,
		bus:            bus,
		buckets:        buckets,
	})

	// Phase F #29: trigger / event dispatch. Loads every YAML-seeded
	// triggers row into an in-memory Registry, then subscribes to the
	// existing event bus + Phase bucket so on_enter / on_say /
	// on_attack / on_death / on_tick handlers fire for the relevant
	// owners. The action vocabulary ships with three builtins
	// (noop / say / emote); consumers (#30 dialogue, #31 quests,
	// #32 Lua) register more handlers off triggerActions before
	// triggerDispatcher.Start runs.
	triggerRepo := repo.NewSQLiteTriggerRepo(conn)
	triggerRegistry := trigger.NewRegistry()
	if n, err := triggerRegistry.Reload(context.Background(), triggerRepo); err != nil {
		slog.Error("Failed to load triggers", "error", err)
		os.Exit(1)
	} else {
		slog.Info("Triggers loaded", "count", n)
	}
	// Phase F #32 slice 1: re-zero every trigger's fault counter
	// + disabled flag at boot. Operator-managed recovery is the
	// only path back to enabled, so a re-deploy intentionally
	// resets the world. The reset runs after Reload (above) so the
	// in-memory copy already reflects DEFAULT 0 / 0.
	if err := triggerRepo.ResetAllFaults(context.Background()); err != nil {
		slog.Error("Failed to reset trigger fault counters", "error", err)
		os.Exit(1)
	}

	// Phase F #32 slice 1: load the Lua script catalog and stand up
	// the runner. The runner's pool is pre-allocated (poolSize=8)
	// so no LStates spin up on the bus goroutine. Script syntax
	// errors fail Load loudly here.
	scriptFS, err := scripts.SourceFS()
	if err != nil {
		slog.Error("Failed to resolve script source", "error", err)
		os.Exit(1)
	}
	parser := luastd.NewState()
	scriptCatalog, err := scripts.Load(scriptFS, parser)
	parser.Close()
	if err != nil {
		slog.Error("Failed to load script catalog", "error", err)
		os.Exit(1)
	}
	slog.Info("Script catalog loaded", "scripts", len(scriptCatalog.ByName))
	luaRunner := luaeng.NewRunner(scriptCatalog, slog.Default())

	// Phase F #31: cross-validate the loaded quest catalog against
	// world content (rooms + mob templates) so a typo in a quest
	// YAML fails the boot. Then stand up the engine and subscribe
	// it to combat / movement events.
	//
	// Ordering note (Phase F #32 slice 2): questEngine is constructed
	// here, before trigger.RegisterLuaAction below, so the trigger
	// Lua action can route quest.accept / quest.advance through the
	// engine. The engine.Start subscription happens after the trigger
	// dispatcher starts so both have wired their event subscribers
	// before any traffic flows.
	questRefs, err := buildQuestRefSets(context.Background(), rooms, mobTemplates, scriptCatalog)
	if err != nil {
		slog.Error("Failed to build quest ref sets", "error", err)
		os.Exit(1)
	}
	if err := quest.Validate(questCatalog, questRefs); err != nil {
		slog.Error("Quest catalog validation failed", "error", err)
		os.Exit(1)
	}
	// Phase F #32 slice 2: cross-ref dialogue `script` effects on
	// every loaded mob_template against the script catalog so a typo
	// fails the boot loudly rather than no-opping at runtime.
	if err := validateDialogueScriptRefs(context.Background(), mobTemplates, scriptCatalog); err != nil {
		slog.Error("Dialogue script refs validation failed", "error", err)
		os.Exit(1)
	}
	questEngine := quest.NewEngine(questCatalog, characters, rooms, audits, bus, sessions)

	// Phase F #32 slice 5b — late-bound shutdown ctx for the wait()
	// binding. The luaHooks block below builds the factory closure
	// here, but srv.ctx (signal.NotifyContext) isn't assigned until
	// after the registry is constructed. We store it in an
	// atomic.Pointer so the boot-time Store establishes a
	// happens-before edge for every dispatch-goroutine Load,
	// independent of the (informal) scheduler.Start ordering.
	var srvShutdownCtx atomic.Pointer[context.Context]

	triggerActions := trigger.DefaultActions()
	luaHooks := trigger.LuaHooks{
		ApplyAffect:   makeLuaApplyAffect(characters, effectsCatalog),
		GiveItem:      makeLuaGiveItem(items),
		TargetHP:      makeLuaTargetHP(characters),
		TargetLevel:   makeLuaTargetLevel(characters),
		TargetClasses: makeLuaTargetClasses(characters, chargenCatalog),
		RoomPlayers:   makeLuaRoomPlayers(sessions),
		RoomMobs:      makeLuaRoomMobs(mobs),
		ClockHour:     clock.HourOfDay,
		ClockDay:      clock.Day,
		// Phase F #32 slice 5a — combat + inventory mutations.
		DealDamage:   makeLuaDealDamage(combatMgr, characters, mobs),
		Heal:         makeLuaHeal(combatMgr, characters, mobs),
		TransferItem: makeLuaTransferItem(items),
		DropItem:     makeLuaDropItem(items),
		// Phase F #32 slice 5b — async + inventory iter. The
		// shutdown ctx pointer is back-filled after
		// signal.NotifyContext below; the wait factory Loads at
		// fire time so the Store/Load pair establishes the barrier.
		Wait:      makeLuaWait(scheduler, luaRunner, &srvShutdownCtx),
		Inventory: makeLuaInventory(items),
		// Phase F #32 slice 5c — sub-second wait + transitive inventory.
		WaitMs:       makeLuaWaitMs(scheduler, luaRunner, &srvShutdownCtx),
		InventoryAll: makeLuaInventoryAll(items),
	}
	if questEngine != nil {
		luaHooks.Accept = questEngine.AcceptQuest
		luaHooks.Advance = questEngine.Advance
	}
	trigger.RegisterLuaAction(triggerActions, luaRunner, scriptCatalog, luaHooks)
	triggerRunner := trigger.NewRunner(triggerRegistry, triggerActions, trigger.ActionDeps{
		Rooms:    rooms,
		Mobs:     mobs,
		Sessions: sessions,
		Logger:   slog.Default(),
		Triggers: triggerRepo,
	})
	triggerDispatcher := trigger.NewDispatcher(bus, buckets.Phase, triggerRunner, mobs)
	triggerDispatcher.Start(context.Background())
	questEngine.Start(context.Background())

	// GMCP manager — owns per-session subscription lifecycle for the
	// Char.*, Room.*, and Comm.* outbound packages plus inbound Core.*
	// dispatch. Wired onto each Session.GMCPHandler in acceptLoop, and
	// torn down in handleConnection's defer via UnwireSession.
	gmcpManager := gmcp.New(bus, sessions, characters, rooms, exits, zones)

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
		triggers:   triggerDispatcher,
		quest:      questEngine,
		luaRunner:  luaRunner,
		gmcp:       gmcpManager,
		cfg:        cfg,
		startedAt:  time.Now(),
		worldStats: collectMSSPWorldStats(context.Background(), zones, rooms, mobTemplates, items),
		closed:     make(chan struct{}),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	srv.stopSignal = stop
	// Phase F #32 slice 5b — back-fill the late-bound shutdown ctx
	// captured by the wait() factory's atomic.Pointer. The
	// runner.Stop() in the shutdown drain will also interrupt any
	// in-flight deferred script, but this propagates cancellation
	// to pending timers via tick.AfterCtx's watcher goroutine
	// BEFORE bus.Stop. The Store() pairs with Load()s in
	// makeLuaWait's fire closure for the memory barrier.
	srvShutdownCtx.Store(&ctx)

	registry, err := buildRegistry(rooms, exits, items, mobs, mobTemplates, zones, characters, audits, shops, bankers, trainers, weaveTeachers, builderZones, sessions, bus, channels, clock, newsCatalog, helpCatalog, chargenCatalog, effectsCatalog, emoteCatalog, combatMgr, groups, questCatalog, questEngine, luaRunner, scheduler, &srvShutdownCtx, srv)
	if err != nil {
		slog.Error("Failed to build command registry", "error", err)
		os.Exit(1)
	}

	// §M.1: backfill the help catalog with auto-generated per-command
	// topics so every registered verb has a `help <name>` entry even
	// when no on-disk article exists. Authored topics keep their
	// precedence (MergeGenerated skips on ID collision); aliases on the
	// generated topic surface as keywords so `help <alias>` resolves
	// through the same pipeline. Runs once at boot before the listener
	// opens.
	if added, skipped := helpCatalog.MergeGenerated(cmd.GenerateCommandTopics(registry)); added > 0 || skipped > 0 {
		slog.Info("help: generated topics merged", "added", added, "skipped", skipped)
	}

	// §M.2: silently throttle clients spamming unknown / unauthorized
	// verbs and uniform-time both rejection paths so a probe can't
	// distinguish "verb doesn't exist" from "verb exists but I'm not
	// privileged". 20 hits per 30s window — comfortable for typos,
	// suffocating for an enumeration loop.
	badInput := telnet.NewBadInputTracker(30*time.Second, 20)
	registry.SetBadInputTracker(badInput)
	srv.badInput = badInput

	gameMode := mode.NewGame(registry, characters, rooms, defaultPromptTemplate)
	if cfg.Audit.CommandsEnabled {
		gameMode.SetAudit(buildCommandAuditFn(commandAudits, cfg.Audit.CommandsExclude))
		slog.Info("Per-character command audit enabled",
			"exclude", cfg.Audit.CommandsExclude)
	}

	setupMetrics(srv, gameMode, cfg, conn, sessions)
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
		login.SetBuilders(builderZones)
		// Slice 4: account-menu security view + per-login audit log.
		login.SetLogins(logins)
		return login
	}

	scheduler.Start(ctx)

	setupBackup(ctx, cfg, conn)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}

	slog.Info("Server started", "address", addr, "db", dsn)

	if srv.metrics != nil {
		srv.metrics.SetReady(true)
	}

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
		// §M.2: per-session outbound rate gate. Zero or negative
		// disables; the default 64 KiB/s sustained / 128 KiB burst
		// caps a runaway script-amplified output without affecting
		// normal play. Configured in cfg.Server.FloodBytesPerSec.
		s.SetFloodGate(srv.cfg.Server.FloodBytesPerSec, srv.cfg.Server.FloodBurstBytes)
		// Wire the MSSP responder onto the session before RunSession
		// starts negotiation; the read goroutine consults this on every
		// inbound DO MSSP. Read-only after this point, so no lock.
		s.MSSPProvider = srv.msspVars
		// GMCP handler closure — same wire-before-RunSession pattern.
		// The closure handles every inbound SB GMCP frame from this
		// client and dispatches Core.* into the Manager.
		s.GMCPHandler = srv.gmcp.Handle

		srv.wg.Add(1)
		safego.Go("session-"+s.RemoteAddress, func() {
			defer srv.wg.Done()
			srv.handleConnection(s)
		})
	}
}

func (srv *server) shutdown() {
	if srv.metrics != nil {
		srv.metrics.SetReady(false)
	}
	if srv.metricsHTTP != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = srv.metricsHTTP.Shutdown(ctx)
		cancel()
	}
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
	if srv.triggers != nil {
		srv.triggers.Stop()
	}
	if srv.quest != nil {
		srv.quest.Stop()
	}
	// Phase F #32 slice 1: stop the Lua runner BEFORE bus.Stop so
	// any in-flight script invocation observes ctx cancellation
	// and exits cleanly. Closing the LStates terminates any
	// gopher-lua goroutines waiting on the per-call ctx.
	if srv.luaRunner != nil {
		srv.luaRunner.Stop()
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

// buildQuestRefSets assembles the (mobs, rooms) ExternalID sets that
// quest.Validate cross-references against. Phase F #31 — runs once
// at boot after the world loader has populated both repos. A typo'd
// reference in a quest YAML fails the boot loudly before the engine
// subscribes to any events.
// firstToken returns the lowercased first whitespace-separated
// token of line, after trimming leading/trailing whitespace. Empty
// input returns the empty string so callers can branch on it.
// Shared between the audit and metric hooks so a future change to
// the verb-extraction rule lands in one place.
func firstToken(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	if idx := strings.IndexAny(trimmed, " \t"); idx >= 0 {
		return strings.ToLower(trimmed[:idx])
	}
	return strings.ToLower(trimmed)
}

// versionOr returns v when non-empty, fallback otherwise. Used for
// the -version flag output so unstamped dev builds still show a
// non-empty triple.
func versionOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
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

func (srv *server) handleConnection(s *telnet.Session) {
	defer s.Conn.Close()
	// Compare-and-delete unbind: if a newer login took over our
	// account, our binding has already been replaced and Unbind is a
	// no-op. Sessions that never authenticated have AccountID == 0
	// and the registry has no entry to remove.
	defer func() {
		// Phase F #32 slice 5b — publish world.PlayerLoggedOut for
		// sessions that had an active character. Account-menu-only
		// disconnects (CharacterID == 0) do NOT publish, mirroring
		// the groups cleanup guard. Publish BEFORE the Unbind so
		// trigger handlers can still resolve the session via the
		// registry if they want a final write.
		if s.CharacterID != 0 && srv.bus != nil {
			_, _, roomID := s.InWorld()
			srv.bus.Publish(context.Background(), world.PlayerLoggedOut{
				CharacterID: s.CharacterID,
				RoomID:      roomID,
			})
		}
		if s.AccountID != 0 {
			srv.sessions.Unbind(s.AccountID, s)
		}
		// §M.2: drop the session's bad-input counter so the tracker
		// doesn't accumulate entries for disconnected clients.
		srv.badInput.Forget(s)
		// Phase D #22: drop any party membership / pending invite
		// the disconnecting character was party to. Leader-leaves
		// disbands; a member departure shrinks. No-op for guests.
		if s.CharacterID != 0 && srv.groups != nil {
			srv.groups.ClearForCharacter(s.CharacterID)
		}
		// Phase I #46: cancel any GMCP subscriptions this session
		// installed. Safe even when the client never sent DO GMCP;
		// TakeGMCPSubs returns an empty slice in that case.
		if srv.gmcp != nil {
			srv.gmcp.UnwireSession(s)
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
