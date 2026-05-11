package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Jasrags/WheelMUD/internal/affects"
	"github.com/Jasrags/WheelMUD/internal/channeling"
	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/cmd"
	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/db"
	"github.com/Jasrags/WheelMUD/internal/dialogue"
	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/effects"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/group"
	"github.com/Jasrags/WheelMUD/internal/help"
	luaeng "github.com/Jasrags/WheelMUD/internal/lua"
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
	triggers   *trigger.Dispatcher
	quest      *quest.Engine
	luaRunner  *luaeng.Runner
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
	trainers := repo.NewSQLiteTrainerRepo(conn)
	weaveTeachers := repo.NewSQLiteWeaveTeacherRepo(conn)

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

	// Phase D #18 combat-render subscribers. Each one snapshots names
	// via best-effort repo lookups so a despawned participant still
	// produces readable output. The "skip-N-sessions" broadcasts use
	// inline Snapshot loops rather than chaining combatBroadcastExcept
	// — for Hit/Miss the attacker AND defender both need second-person
	// echoes, so the third-person broadcast must skip both.
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
	// combatBroadcastExcept is the §19 player-death variant that
	// skips one specific session — used so the dying player doesn't
	// see "X falls dead!" alongside their "You die!" line, and the
	// respawned player doesn't see "X appears" stacked on top of
	// their own bound-room render.
	combatBroadcastExcept := func(roomID int64, msg string, exclude *telnet.Session) {
		for _, peer := range sessions.Snapshot() {
			if peer == nil || peer == exclude || peer.CurrentRoomID != roomID {
				continue
			}
			if err := peer.WriteAsync(msg); err != nil {
				slog.Debug("combat: broadcast write failed", "error", err)
			}
		}
	}
	eventbus.Subscribe(bus, func(ctx context.Context, ev combat.CombatStance) {
		name := combatActorName(ctx, ev.Actor, characters, mobs)
		switch ev.Kind {
		case "parry":
			combatBroadcast(ev.RoomID, "{{"+name+" raises a weapon to parry.}}::yellow\r\n")
		}
	})
	eventbus.Subscribe(bus, func(ctx context.Context, ev combat.CombatParry) {
		def := combatActorName(ctx, ev.Defender, characters, mobs)
		atk := combatActorName(ctx, ev.Attacker, characters, mobs)
		combatBroadcast(ev.RoomID, "{{"+def+" parries "+atk+"'s blow!}}::cyan\r\n")
	})
	eventbus.Subscribe(bus, func(ctx context.Context, ev combat.CombatFlee) {
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

	// combatBroadcastSkip is a two-exclude variant of combatBroadcast.
	// Inline so the closure can capture sessions without forcing every
	// per-event subscriber to thread an extra arg.
	combatBroadcastSkip := func(roomID int64, msg string, a, b *telnet.Session) {
		for _, peer := range sessions.Snapshot() {
			if peer == nil || peer == a || peer == b || peer.CurrentRoomID != roomID {
				continue
			}
			if err := peer.WriteAsync(msg); err != nil {
				slog.Debug("combat: broadcast write failed", "error", err)
			}
		}
	}

	// CombatHit: per-attacker echo, per-defender echo (if player),
	// third-person line to room peers excluding both. Crit adds a
	// suffix so the player sees the dice-result of their roll.
	eventbus.Subscribe(bus, func(ctx context.Context, ev combat.CombatHit) {
		atkName := combatActorName(ctx, ev.Attacker, characters, mobs)
		defName := combatActorName(ctx, ev.Defender, characters, mobs)
		critTail := ""
		if ev.IsCrit {
			critTail = " {{(critical!)}}::yellow|bold"
		}
		var atkSess, defSess *telnet.Session
		if ev.Attacker.Kind == combat.ActorKindCharacter {
			atkSess = cmd.LookupByCharacterID(sessions, ev.Attacker.ID)
			if atkSess != nil {
				_ = atkSess.WriteAsync(fmt.Sprintf(variantHitSelfFormat(ev.Variant),
					defName, ev.Damage, critTail))
			}
		}
		if ev.Defender.Kind == combat.ActorKindCharacter {
			defSess = cmd.LookupByCharacterID(sessions, ev.Defender.ID)
			if defSess != nil {
				_ = defSess.WriteAsync(fmt.Sprintf("{{%s hits you for %d damage.}}::red%s",
					atkName, ev.Damage, critTail))
			}
		}
		combatBroadcastSkip(ev.RoomID,
			fmt.Sprintf("{{%s hits %s for %d damage.}}::yellow%s\r\n",
				atkName, defName, ev.Damage, critTail),
			atkSess, defSess)
	})

	// CombatMiss: symmetric to Hit but no damage line; both
	// participants and the room see the swing-and-miss beat.
	eventbus.Subscribe(bus, func(ctx context.Context, ev combat.CombatMiss) {
		atkName := combatActorName(ctx, ev.Attacker, characters, mobs)
		defName := combatActorName(ctx, ev.Defender, characters, mobs)
		var atkSess, defSess *telnet.Session
		if ev.Attacker.Kind == combat.ActorKindCharacter {
			atkSess = cmd.LookupByCharacterID(sessions, ev.Attacker.ID)
			if atkSess != nil {
				_ = atkSess.WriteAsync(fmt.Sprintf(variantMissSelfFormat(ev.Variant), defName))
			}
		}
		if ev.Defender.Kind == combat.ActorKindCharacter {
			defSess = cmd.LookupByCharacterID(sessions, ev.Defender.ID)
			if defSess != nil {
				_ = defSess.WriteAsync(fmt.Sprintf("{{%s swings at you and misses.}}::gray", atkName))
			}
		}
		combatBroadcastSkip(ev.RoomID,
			fmt.Sprintf("{{%s swings at %s and misses.}}::gray\r\n", atkName, defName),
			atkSess, defSess)
	})

	// CombatDeath for mob victims: "You killed X" to the killer (if
	// a player) and "X falls dead!" to room peers excluding the
	// killer. Player victims are handled by the CharacterDied
	// subscriber below — gate on Victim.Kind to avoid double-render.
	eventbus.Subscribe(bus, func(ctx context.Context, ev combat.CombatDeath) {
		if ev.Victim.Kind != combat.ActorKindMob {
			return
		}
		// Prefer the publish-time snapshot since the mob_instance row
		// is already gone by the time this fires; combatActorName
		// would fall back to "A creature" otherwise.
		victimName := ev.VictimName
		if victimName == "" {
			victimName = combatActorName(ctx, ev.Victim, characters, mobs)
		}
		var killerSess *telnet.Session
		if ev.Killer.Kind == combat.ActorKindCharacter {
			killerSess = cmd.LookupByCharacterID(sessions, ev.Killer.ID)
			if killerSess != nil {
				_ = killerSess.WriteAsync("{{You killed " + victimName + "!}}::green|bold")
			}
		}
		combatBroadcastExcept(ev.RoomID,
			"{{"+victimName+" falls dead!}}::red|bold\r\n", killerSess)
	})

	// CombatXPAwarded: private "You gain N XP." to the awardee, with
	// an optional XP-debt-drain suffix when DebtTaken > 0 so the
	// player understands why their gain looks smaller than the
	// gross share.
	eventbus.Subscribe(bus, func(ctx context.Context, ev combat.CombatXPAwarded) {
		if ev.Awardee.Kind != combat.ActorKindCharacter {
			return
		}
		sess := cmd.LookupByCharacterID(sessions, ev.Awardee.ID)
		if sess == nil {
			return
		}
		msg := fmt.Sprintf("{{You gain %d xp.}}::cyan", ev.Amount)
		if ev.DebtTaken > 0 {
			msg += fmt.Sprintf("  {{(%d xp went to clearing your xp debt)}}::gray", ev.DebtTaken)
		}
		_ = sess.WriteAsync(msg)
	})

	// Phase D §19 player-death subscribers. CharacterDied broadcasts
	// the death-room line + a "You die!" private message to the
	// dying player. CharacterRespawned then stamps the victim's
	// session room, renders the bound room, and broadcasts to peers
	// in the new room. The repo layer (handleCharacterDeath) already
	// persisted the room change via RecordRoom; the subscriber just
	// updates the live in-memory session.
	eventbus.Subscribe(bus, func(ctx context.Context, ev combat.CharacterDied) {
		victim := cmd.LookupByCharacterID(sessions, ev.Victim.ID)
		name := combatActorName(ctx, ev.Victim, characters, mobs)
		// Broadcast "X falls dead!" to peers — but skip the dying
		// player so they don't see it alongside "You die!" on the
		// next line. Their CurrentRoomID is still DeathRoomID at
		// this point (SetCurrentRoom runs in the respawn handler).
		combatBroadcastExcept(ev.DeathRoomID,
			"{{"+name+" falls dead!}}::red|bold\r\n", victim)
		if victim != nil {
			msg := "{{You die!}}::red|bold"
			if ev.XPDebtAdded > 0 {
				msg += fmt.Sprintf("  {{(+%d xp debt)}}::gray", ev.XPDebtAdded)
			}
			if err := victim.WriteAsync(msg); err != nil {
				slog.Debug("character died: victim notify failed",
					"char", ev.Victim.ID, "error", err)
			}
		}
	})
	eventbus.Subscribe(bus, func(ctx context.Context, ev combat.CharacterRespawned) {
		victim := cmd.LookupByCharacterID(sessions, ev.Character.ID)
		name := combatActorName(ctx, ev.Character, characters, mobs)
		if victim != nil {
			victim.SetCurrentRoom(ev.RoomID)
			// Detached context so the render isn't bound to whatever
			// resolveAction's ctx was; mirrors transferToCaller.
			if err := cmd.RenderRoom(context.Background(), victim, rooms, exits, items, mobs, clock); err != nil {
				slog.Debug("character respawned: render failed",
					"char", ev.Character.ID, "error", err)
			}
		}
		// Skip the respawned player on the arrival broadcast — they
		// just got their own bound-room render above; "X appears,
		// eyes hollow." stacked on top would be noise.
		combatBroadcastExcept(ev.RoomID,
			"{{"+name+" appears, eyes hollow.}}::cyan\r\n", victim)
	})

	// Phase E #26: out-of-combat affects ticker. Walks every in-world
	// session, skips characters already in a fight (combat's end-of-
	// round tick handles those), and decrements affect durations on
	// the rest. Cadence is 6 s — just slow enough that scanning the
	// session map per pulse stays cheap, fast enough that short buffs
	// have visible feedback.
	affectsCandidates := func() []affects.Candidate {
		snap := sessions.Snapshot()
		out := make([]affects.Candidate, 0, len(snap))
		for _, s := range snap {
			if s == nil {
				continue
			}
			charID, _, roomID := s.InWorld()
			if charID == 0 || roomID == 0 {
				continue
			}
			out = append(out, affects.Candidate{CharacterID: charID, RoomID: roomID})
		}
		return out
	}
	affectsTicker := affects.NewSessionTicker(
		affectsCandidates,
		combatMgr,
		characterAffectsLoader{characters},
		eventbusAdapter{bus},
		slog.Default(),
	)
	affectsTicker.SetDeathHook(func(ctx context.Context, charID int64, _ string) {
		// DoT-death entrypoint into the §19 death pipeline. Cause is
		// surfaced via the TickDamaged event the ticker also publishes,
		// so the handler doesn't need it.
		combatMgr.HandleAffectDeath(ctx, charID)
	})
	buckets.Affects.Subscribe(affectsTicker.Tick)

	// Phase E #27: channeling ticker. Refills slot pools 8h after the
	// last refresh and accrues Madness on every pulse for embraced
	// male channelers. Subscribed to the Regen bucket (30s) — slow
	// enough that the 8h gate adds negligible load and fast enough
	// that Madness accrual is observable in tests with a tightened
	// TickInterval.
	channelingCandidates := func() []channeling.Candidate {
		snap := sessions.Snapshot()
		out := make([]channeling.Candidate, 0, len(snap))
		for _, s := range snap {
			if s == nil {
				continue
			}
			charID, _, roomID := s.InWorld()
			if charID == 0 {
				continue
			}
			out = append(out, channeling.Candidate{CharacterID: charID, RoomID: roomID})
		}
		return out
	}
	channelingTicker := channeling.NewSessionTicker(
		channelingCandidates,
		characterChannelingLoader{characters},
		nil, // time.Now
		slog.Default(),
	)
	buckets.Regen.Subscribe(channelingTicker.Tick)

	// Phase L slice 63: stamina regen ticker. Tops StaminaCurrent
	// toward StaminaMax at the racial StaminaRegen rate (halved by
	// heavy armor) on every Regen pulse. Subscribed alongside the
	// channeling ticker so the action-cost pool refills at the same
	// cadence the channeling pools refresh.
	staminaCandidates := func() []combat.StaminaCandidate {
		snap := sessions.Snapshot()
		out := make([]combat.StaminaCandidate, 0, len(snap))
		for _, s := range snap {
			if s == nil {
				continue
			}
			charID, _, roomID := s.InWorld()
			if charID == 0 {
				continue
			}
			out = append(out, combat.StaminaCandidate{CharacterID: charID, RoomID: roomID})
		}
		return out
	}
	staminaTicker := combat.NewStaminaTicker(
		staminaCandidates,
		characters,
		items,
		slog.Default(),
	)
	buckets.Regen.Subscribe(staminaTicker.Tick)

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

	// affects.Expired subscriber: emits one cfmt line per entry to
	// the owning session via WriteAsync (cross-session output rule).
	// Catalog-driven affects carry an authored MessageOnExpire string
	// (Phase E #25 slice 3); admin-applied affects (slice 1) leave it
	// empty and fall back to the generic fade line.
	eventbus.Subscribe(bus, func(_ context.Context, ev affects.Expired) {
		victim := cmd.LookupByCharacterID(sessions, ev.CharacterID)
		if victim == nil {
			return
		}
		for _, e := range ev.Entries {
			var msg string
			if e.Message != "" {
				msg = "{{" + e.Message + "}}::cyan\r\n"
			} else {
				msg = "{{Your " + e.Name + " fades.}}::cyan\r\n"
			}
			if err := victim.WriteAsync(msg); err != nil {
				slog.Debug("affects: expired notify failed",
					"char", ev.CharacterID, "name", e.Name, "error", err)
			}
		}
	})

	// affects.TickDamaged subscriber: per-tick HP delta lines from
	// poison/bleed/regen affects. Phase E #25 slice 2.
	eventbus.Subscribe(bus, func(_ context.Context, ev affects.TickDamaged) {
		victim := cmd.LookupByCharacterID(sessions, ev.CharacterID)
		if victim == nil {
			return
		}
		for _, te := range ev.Events {
			if te.Delta == 0 {
				continue
			}
			var msg string
			if te.Delta < 0 {
				msg = fmt.Sprintf("{{You suffer %d damage from %s.}}::red\r\n", -te.Delta, te.Name)
			} else {
				msg = fmt.Sprintf("{{You recover %d hp from %s.}}::green\r\n", te.Delta, te.Name)
			}
			if err := victim.WriteAsync(msg); err != nil {
				slog.Debug("affects: tick notify failed",
					"char", ev.CharacterID, "name", te.Name, "error", err)
			}
		}
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
		triggers:   triggerDispatcher,
		quest:      questEngine,
		luaRunner:  luaRunner,
		closed:     make(chan struct{}),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	srv.stopSignal = stop

	registry, err := buildRegistry(rooms, exits, items, mobs, mobTemplates, zones, characters, audits, shops, bankers, trainers, weaveTeachers, sessions, bus, channels, clock, newsCatalog, helpCatalog, chargenCatalog, effectsCatalog, combatMgr, groups, questCatalog, questEngine, luaRunner, srv)
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
func buildQuestRefSets(ctx context.Context, rooms repo.RoomRepo, templates repo.MobTemplateRepo, scriptCat *scripts.Catalog) (*quest.RefSets, error) {
	allRooms, err := rooms.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rooms: %w", err)
	}
	mobIDs, err := templates.ListExternalIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list mob templates: %w", err)
	}
	refs := &quest.RefSets{
		Mobs:  make(map[string]bool, len(mobIDs)),
		Rooms: make(map[string]bool, len(allRooms)),
	}
	for _, id := range mobIDs {
		refs.Mobs[id] = true
	}
	for _, r := range allRooms {
		if r.ExternalID != "" {
			refs.Rooms[r.ExternalID] = true
		}
	}
	// Phase F #32 slice 2: cross-ref StepScript against the loaded
	// Lua catalog. nil scriptCat (e.g. boots that disable scripting)
	// disables the check.
	if scriptCat != nil {
		refs.Scripts = make(map[string]bool, len(scriptCat.ByName))
		for name := range scriptCat.ByName {
			refs.Scripts[name] = true
		}
	}
	return refs, nil
}

// validateDialogueScriptRefs walks every mob_template's stored
// dialogue_json and asserts that every `effects: kind: script`
// references a script the catalog knows. A missing script at
// runtime degrades gracefully (the dialogue effect logs + no-ops),
// but boot-time fail-fast keeps authoring mistakes from sitting
// silently in the world. Phase F #32 slice 2.
//
// nil scriptCat disables the check (mirrors quest.RefSets.Scripts
// nil-disables): boots that ship without scripting authored skip
// the cross-ref entirely. An empty catalog is *not* the same as nil
// — we still validate and reject any script reference.
func validateDialogueScriptRefs(ctx context.Context, templates repo.MobTemplateRepo, scriptCat *scripts.Catalog) error {
	if scriptCat == nil {
		return nil
	}
	ids, err := templates.ListExternalIDs(ctx)
	if err != nil {
		return fmt.Errorf("list mob templates: %w", err)
	}
	for _, ext := range ids {
		t, err := templates.GetByExternalID(ctx, ext)
		if err != nil {
			return fmt.Errorf("get mob template %q: %w", ext, err)
		}
		if len(t.DialogueJSON) == 0 || string(t.DialogueJSON) == "null" {
			continue
		}
		var tree dialogue.Tree
		if err := json.Unmarshal(t.DialogueJSON, &tree); err != nil {
			return fmt.Errorf("decode dialogue for mob %q: %w", ext, err)
		}
		for nodeID, node := range tree.Nodes {
			for ri, resp := range node.Responses {
				for ei, eff := range resp.Effects {
					if eff.Kind != dialogue.EffectScript {
						continue
					}
					name := eff.Args["script"]
					if _, ok := scriptCat.Get(name); !ok {
						return fmt.Errorf("mob %q dialogue node %q response[%d] effect[%d]: unknown script %q",
							ext, nodeID, ri, ei, name)
					}
				}
			}
		}
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

// variantHitSelfFormat returns the complete Sprintf format string
// for a CombatHit subscriber's first-person echo line. cfmt wrap is
// baked in so the returned value can be passed as a literal format
// to Sprintf and `go vet` statically validates the verb/arg counts.
// Verbs: %s (defender name), %d (damage), %s (crit tail). Phase L #61.
func variantHitSelfFormat(v combat.AttackVariant) string {
	switch v {
	case combat.VariantPower:
		return "{{You lunge with a power strike at %s for %d damage.}}::cyan%s"
	case combat.VariantQuick:
		return "{{You flick a quick jab at %s for %d damage.}}::cyan%s"
	default:
		return "{{You hit %s for %d damage.}}::cyan%s"
	}
}

// variantMissSelfFormat is the miss-side mirror of
// variantHitSelfFormat. Verb: %s (defender name).
func variantMissSelfFormat(v combat.AttackVariant) string {
	switch v {
	case combat.VariantPower:
		return "{{You lunge wide of %s and miss.}}::gray"
	case combat.VariantQuick:
		return "{{You flick at %s and miss.}}::gray"
	default:
		return "{{You swing at %s and miss.}}::gray"
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

func buildRegistry(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, mobTemplates repo.MobTemplateRepo, zones repo.ZoneRepo, characters repo.CharacterRepo, audits repo.AdminAuditRepo, shops repo.ShopRepo, bankers repo.BankerRepo, trainers repo.TrainerRepo, weaveTeachers repo.WeaveTeacherRepo, sessions *session.Registry, bus *eventbus.Bus, channels []repo.Channel, clock *world.Clock, newsCatalog *news.Catalog, helpCatalog *help.Catalog, chargenCatalog *chargen.Catalog, effectsCatalog *effects.Catalog, combatMgr *combat.Manager, groups *group.Manager, questCatalog *quest.Catalog, questEngine *quest.Engine, luaRunner *luaeng.Runner, shutdownCtl cmd.ShutdownController) (*telnet.Registry, error) {
	r := telnet.NewRegistry()
	if err := r.Register(cmd.Quit, cmd.Colors); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewWho(sessions, characters),
		cmd.NewSay(sessions, rooms, bus),
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
		cmd.NewQuaff(items, characters, effectsCatalog, sessions),
		cmd.NewWear(items, characters, sessions),
		cmd.NewWield(items, characters, sessions),
		cmd.NewRemove(items, characters, sessions),
		cmd.NewEquipment(items, characters),
		cmd.NewSpawn(items, mobTemplates, mobs, characters, sessions, audits),
	); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewAttack(combatMgr, rooms, mobs, characters, sessions, groups),
		cmd.NewPower(combatMgr, rooms, mobs, characters, sessions, groups),
		cmd.NewJab(combatMgr, rooms, mobs, characters, sessions, groups),
	); err != nil {
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
	if err := r.Register(cmd.NewScore(characters, items, chargenCatalog)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewXP(characters)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewTrain(characters, mobs, mobTemplates, trainers, chargenCatalog, audits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewLearn(characters, chargenCatalog, audits, mobs, mobTemplates, weaveTeachers)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewFeat(characters, chargenCatalog, audits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewBump(characters, audits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewEmbrace(characters)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewRelease(characters)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewStill(characters, sessions, audits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewUnstill(characters, sessions, audits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewAffects(characters)); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewAffect(characters, sessions, audits),
		cmd.NewDispel(characters, sessions, audits),
	); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewCooldowns(characters, chargenCatalog)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewCooldown(characters, sessions, audits, chargenCatalog)); err != nil {
		return nil, err
	}
	// Phase F #30: NPC dialogue trees. main.go is the only place that
	// can hand cmd a closure that builds a *mode.Dialogue, since cmd
	// can't import internal/mode without risking an import cycle
	// through chargen.
	hooks := mode.DialogueHooks{}
	if questEngine != nil {
		// Honor the dispatcher's ctx so a stalled repo write under
		// shutdown drain or session teardown unblocks instead of
		// holding the dialogue handler open. Engine repo calls
		// already accept the propagated ctx.
		hooks.AcceptQuest = func(ctx context.Context, s *telnet.Session, questID string) error {
			return questEngine.AcceptQuest(ctx, s.CharacterID, questID)
		}
		hooks.AdvanceQuest = func(ctx context.Context, s *telnet.Session, questID, npcExternalID string) error {
			return questEngine.AdvanceTalkTo(ctx, s.CharacterID, questID, npcExternalID)
		}
	}
	// Phase F #32 slice 2: dialogue `script` effect runs a Lua
	// catalog script with the V2 mutation surface. Hook is unbound
	// when no Lua runner is wired (test harness, malformed boot) —
	// applyEffects logs and continues so a misconfigured boot
	// doesn't lock players inside the dialogue.
	if luaRunner != nil {
		hooks.RunScript = func(ctx context.Context, s *telnet.Session, name string) error {
			bindings := luaeng.APIBindings{
				Logger: slog.Default(),
				Ctx: luaeng.CtxView{
					Event:     "dialogue.script",
					ActorID:   s.CharacterID,
					ActorKind: "character",
					RoomID:    s.CurrentRoomID,
					Text:      name,
				},
			}
			if questEngine != nil {
				bindings.QuestAccept = func(id string) error {
					return questEngine.AcceptQuest(ctx, s.CharacterID, id)
				}
				bindings.QuestAdvance = func(id string) error {
					return questEngine.Advance(ctx, s.CharacterID, id)
				}
			}
			// V3 surface (Phase F #32 slice 3). Reuse the same
			// closure constructors the trigger path uses; the
			// dialogue actor is always the calling character so no
			// extra actor-kind guard is needed here.
			applyAffect := makeLuaApplyAffect(characters, effectsCatalog)
			giveItem := makeLuaGiveItem(items)
			targetHP := makeLuaTargetHP(characters)
			targetLevel := makeLuaTargetLevel(characters)
			targetClasses := makeLuaTargetClasses(characters, chargenCatalog)
			roomPlayers := makeLuaRoomPlayers(sessions)
			roomMobs := makeLuaRoomMobs(mobs)
			bindings.ApplyAffect = func(targetID int64, effectID string, durationOverride int32) error {
				return applyAffect(ctx, targetID, effectID, durationOverride)
			}
			// Per-invocation give_item cap (mirrors trigger path).
			// Counter is captured per RunScript call so each
			// dialogue script fire gets a fresh budget.
			giveCount := 0
			bindings.GiveItem = func(targetID int64, externalID string) error {
				giveCount++
				if giveCount > trigger.MaxGiveItemsPerInvocation {
					return fmt.Errorf("give_item exceeded per-invocation cap of %d", trigger.MaxGiveItemsPerInvocation)
				}
				return giveItem(ctx, targetID, externalID)
			}
			bindings.TargetHP = func(targetID int64) (int32, int32, error) {
				return targetHP(ctx, targetID)
			}
			bindings.TargetLevel = func(targetID int64) (int, error) {
				return targetLevel(ctx, targetID)
			}
			bindings.TargetClasses = func(targetID int64) (map[string]int, error) {
				return targetClasses(ctx, targetID)
			}
			// V4 surface (Phase F #32 slice 4) — room queries
			// resolve from the dialogue session's CurrentRoomID,
			// not a Lua-side argument.
			bindings.RoomPlayers = func() ([]int64, error) {
				return roomPlayers(ctx, s.CurrentRoomID)
			}
			bindings.RoomMobs = func() ([]int64, error) {
				return roomMobs(ctx, s.CurrentRoomID)
			}
			bindings.ClockHour = clock.HourOfDay
			bindings.ClockDay = clock.Day
			// PushMode is intentionally nil for dialogue scripts: V2
			// has no concrete cross-mode push targets and the
			// classified Lua error makes the unbound state visible
			// to authors.
			return luaRunner.Run(ctx, name, func(l *luastd.LState) { bindings.Bind(l) })
		}
	}
	pushDialogue := func(s *telnet.Session, npcName, npcExternalID string, tree *dialogue.Tree) error {
		dm, err := mode.NewDialogue(npcName, npcExternalID, tree, hooks)
		if err != nil {
			return err
		}
		return s.PushMode(dm)
	}
	if err := r.Register(cmd.NewTalk(mobs, mobTemplates, pushDialogue)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewQuest(characters, questCatalog, questEngine, audits)); err != nil {
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

// makeLuaApplyAffect builds the Phase F #32 slice 3 apply_affect
// closure. Resolves effectID through the catalog, builds a
// creature.Affect via Effect.ToAffect (sentinel Source =
// cmd.LuaAffectSource → "script" label in the inspect verb), and
// persists via affects.Apply + RecordAffects.
//
// Slice 4: durationOverride > 0 overrides the catalog's authored
// DurationTicks; 0 means "use catalog default".
func makeLuaApplyAffect(characters repo.CharacterRepo, eff *effects.Catalog) func(context.Context, int64, string, int32) error {
	return func(ctx context.Context, targetID int64, effectID string, durationOverride int32) error {
		e, ok := eff.Get(effectID)
		if !ok {
			return fmt.Errorf("unknown effect %q", effectID)
		}
		ch, err := characters.GetByID(ctx, targetID)
		if err != nil {
			return err
		}
		affect := e.ToAffect(cmd.LuaAffectSource)
		if durationOverride > 0 {
			affect.DurationTicks = durationOverride
		}
		next := affects.Apply(ch.Core.Affects, affect)
		return characters.RecordAffects(ctx, targetID, next)
	}
}

// luaGiveItemSeq breaks ties when two give_item calls land in the
// same nanosecond on the same target — without it, the generated
// external_id collides on the items.external_id UNIQUE index and
// the second call trips the trigger fault budget. Process-global,
// monotonic, never reset.
var luaGiveItemSeq int64

// makeLuaGiveItem clones the YAML-seeded template at externalID and
// places the fresh row directly into the target's inventory. Mirrors
// the admin spawn path (internal/cmd/spawn.go::spawnItems) but skips
// admin auditing — Lua-driven spawns are content-author tools, not
// privileged operator actions.
func makeLuaGiveItem(items repo.ItemRepo) func(context.Context, int64, string) error {
	return func(ctx context.Context, targetID int64, externalID string) error {
		template, err := items.FindByExternalID(ctx, externalID)
		if err != nil {
			return err
		}
		seq := atomic.AddInt64(&luaGiveItemSeq, 1)
		spawn := repo.Item{
			ExternalID:       fmt.Sprintf("%s#lua-%d-%d-%d", externalID, time.Now().UnixNano(), targetID, seq),
			Name:             template.Name,
			NameLower:        template.NameLower,
			ShortDesc:        template.ShortDesc,
			OwnerCharacterID: targetID,
			Type:             template.Type,
			Weight:           template.Weight,
			Value:            template.Value,
			Quality:          template.Quality,
			Flags:            template.Flags,
			Stats:            repo.CloneItemStats(template.Stats),
		}
		_, err = items.Create(ctx, spawn)
		return err
	}
}

// makeLuaTargetHP returns a closure exposing a character's
// HPCurrent / HPMax via target.hp(id) in Lua scripts.
func makeLuaTargetHP(characters repo.CharacterRepo) func(context.Context, int64) (int32, int32, error) {
	return func(ctx context.Context, targetID int64) (int32, int32, error) {
		ch, err := characters.GetByID(ctx, targetID)
		if err != nil {
			return 0, 0, err
		}
		return ch.Core.HPCurrent, ch.Core.HPMax, nil
	}
}

// makeLuaTargetLevel sums ClassLevels into a single integer for
// target.level(id). Multiclassed characters return the sum of
// every class's level.
func makeLuaTargetLevel(characters repo.CharacterRepo) func(context.Context, int64) (int, error) {
	return func(ctx context.Context, targetID int64) (int, error) {
		ch, err := characters.GetByID(ctx, targetID)
		if err != nil {
			return 0, err
		}
		total := 0
		for _, lvl := range ch.ClassLevels {
			total += int(lvl)
		}
		return total, nil
	}
}

// makeLuaTargetClasses returns the multiclass map keyed by the
// chargen catalog's canonical class id (e.g. "armsman", "initiate").
// Phase F #32 slice 4 — companion to target.level which sums into
// a single int. Empty map for a character with no class levels
// (defensive — chargen always stamps at least one). Falls back to
// "class_<int>" when a catalog row is missing for a given enum
// value (shouldn't happen in practice; defensive only).
func makeLuaTargetClasses(characters repo.CharacterRepo, cat *chargen.Catalog) func(context.Context, int64) (map[string]int, error) {
	enumToID := make(map[creature.Class]string)
	for _, c := range cat.Classes() {
		enumToID[c.Enum] = c.ID
	}
	return func(ctx context.Context, targetID int64) (map[string]int, error) {
		ch, err := characters.GetByID(ctx, targetID)
		if err != nil {
			return nil, err
		}
		out := make(map[string]int, len(ch.ClassLevels))
		for cls, lvl := range ch.ClassLevels {
			id, ok := enumToID[cls]
			if !ok {
				id = fmt.Sprintf("class_%d", int(cls))
			}
			out[id] = int(lvl)
		}
		return out, nil
	}
}

// makeLuaRoomPlayers returns the bound character ids in roomID,
// sorted ascending. Phase F #32 slice 4 — feeds room.players() in
// Lua. Returns an empty slice (never nil) so the Lua-side ipairs
// always sees a valid table.
func makeLuaRoomPlayers(sessions *session.Registry) func(context.Context, int64) ([]int64, error) {
	return func(_ context.Context, roomID int64) ([]int64, error) {
		out := make([]int64, 0, 4)
		for charID, s := range sessions.Snapshot() {
			if s == nil {
				continue
			}
			_, _, sRoom := s.InWorld()
			if sRoom == roomID {
				out = append(out, charID)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out, nil
	}
}

// makeLuaRoomMobs returns mob_instance ids in roomID via
// MobInstanceRepo.ListInRoom. Phase F #32 slice 4.
func makeLuaRoomMobs(mobs repo.MobInstanceRepo) func(context.Context, int64) ([]int64, error) {
	return func(ctx context.Context, roomID int64) ([]int64, error) {
		list, err := mobs.ListInRoom(ctx, roomID)
		if err != nil {
			return nil, err
		}
		out := make([]int64, 0, len(list))
		for _, m := range list {
			out = append(out, m.ID)
		}
		return out, nil
	}
}

// validateConsumableEffectRefs walks every consumable item parsed
// from the world YAML and confirms its EffectID resolves through the
// loaded effects catalog. Zero (no effect set) is treated as authored
// intent — the potion fizzles when quaffed. Phase E #25 slice 2.
func validateConsumableEffectRefs(loaded world.LoadedWorld, eff *effects.Catalog) error {
	for zone, specs := range loaded.ItemSpecsByZone {
		for _, spec := range specs {
			if spec.Item.Type != repo.ItemTypeConsumable {
				continue
			}
			stats, ok := spec.Item.Stats.(repo.ConsumableStats)
			if !ok {
				continue
			}
			if stats.EffectID == 0 {
				continue
			}
			if _, ok := eff.IDForHash(stats.EffectID); !ok {
				return fmt.Errorf("zone %s: consumable %q references unknown effect (hash=%d)",
					zone, spec.Item.ExternalID, stats.EffectID)
			}
		}
	}
	return nil
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

// characterAffectsLoader adapts repo.CharacterRepo to the slim
// affects.CharLoader interface. Only the Affects field of the loaded
// row is exposed to the ticker — everything else stays in the repo.
type characterAffectsLoader struct {
	chars repo.CharacterRepo
}

func (a characterAffectsLoader) GetByID(ctx context.Context, id int64) (affects.Character, error) {
	ch, err := a.chars.GetByID(ctx, id)
	if err != nil {
		return affects.Character{}, err
	}
	return affects.Character{
		Affects:   ch.Core.Affects,
		HPCurrent: ch.Core.HPCurrent,
		HPMax:     ch.Core.HPMax,
		Subdual:   ch.Core.Subdual,
		Condition: ch.Core.Conditions,
		Position:  ch.Core.Position,
	}, nil
}

func (a characterAffectsLoader) RecordAffects(ctx context.Context, id int64, list []creature.Affect) error {
	return a.chars.RecordAffects(ctx, id, list)
}

func (a characterAffectsLoader) RecordHP(ctx context.Context, id int64, hp, subdual int32) error {
	return a.chars.RecordHP(ctx, id, hp, subdual)
}

// characterChannelingLoader adapts repo.CharacterRepo to the slim
// channeling.CharLoader interface. Phase E #27 — only the Channeling
// pointer is exposed to the ticker.
type characterChannelingLoader struct {
	chars repo.CharacterRepo
}

func (a characterChannelingLoader) GetByID(ctx context.Context, id int64) (channeling.Character, error) {
	ch, err := a.chars.GetByID(ctx, id)
	if err != nil {
		return channeling.Character{}, err
	}
	return channeling.Character{Channeling: ch.Channeling}, nil
}

func (a characterChannelingLoader) RecordChanneling(ctx context.Context, id int64, c *creature.Channeling) error {
	return a.chars.RecordChanneling(ctx, id, c)
}

// eventbusAdapter wraps *eventbus.Bus to satisfy affects.EventPublisher.
// affects.EventPublisher takes an `any` so the affects package stays
// free of the eventbus import; eventbus.Event is interface{}, so any
// payload (including the typed affects.Expired struct) round-trips
// through reflection inside Publish.
type eventbusAdapter struct {
	bus *eventbus.Bus
}

func (a eventbusAdapter) Publish(ctx context.Context, ev any) {
	if a.bus == nil {
		return
	}
	a.bus.Publish(ctx, ev)
}
