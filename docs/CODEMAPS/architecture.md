<!-- Generated: 2026-05-10 | Updated for Phase F #29-32 (triggers/dialogue/quest/Lua) + Phase E #25-28 (affects/quaff/cooldowns/embrace/weave-teachers) + zone-reset doors | Token estimate: ~1700 -->

# Architecture

WheelMUD is a single-binary Go MUD server. One TCP listener fans out to a
goroutine-per-connection model. The auth + chargen + world + combat +
progression layers are all wired end-to-end. SQLite (modernc pure-Go
driver) backs every persisted aggregate; YAML loaders seed rooms / mobs /
items / chargen content / news / help at boot. The scheduler manages
heartbeat ticks (combat, regen, area reset, autosave). Single-session-per-
account is enforced via a process-level session registry.

## Layers

```
┌────────────────────────────────────────────────────────────────────┐
│ cmd/server/main.go    listener + DI wiring + graceful shutdown      │
├────────────────────────────────────────────────────────────────────┤
│ Tick / persist / events / sessions / panic recovery                 │
│   internal/tick/        Scheduler (1 Hz) + named Buckets            │
│                         (combat / regen / areaReset / save)         │
│   internal/persist/     Autosave manager, FlushAll on shutdown      │
│   internal/eventbus/    Typed pub/sub (PlayerEntered/Left,          │
│                         CombatStarted/Hit/Miss/Death/XPAwarded)     │
│   internal/session/     Process-level Registry; single-session-per- │
│                         account; FindByCharacterName / Snapshot     │
│   internal/safego/      panic-recovery wrapper for long goroutines  │
├────────────────────────────────────────────────────────────────────┤
│ Modes + command surface                                             │
│   internal/mode/        login / create / character_select /         │
│                         character_create (chargen substep machine)/ │
│                         postauth / account_menu / game              │
│   internal/cmd/         ~50 verbs (catalog in commands.md)          │
│   internal/audit/       Record(verb, target, args) for admin verbs  │
├────────────────────────────────────────────────────────────────────┤
│ Game systems                                                        │
│   internal/combat/      Per-room Fight, attack/damage resolution,   │
│                         threat tables, group XP split, decayer +    │
│                         flee mover seams                            │
│   internal/group/       In-memory party manager (Group + Manager,   │
│                         MaxGroupSize=6, leader-leaves-disbands)     │
│   internal/progression/ XP curve (1000·n·(n-1)/2, MaxLevel=20),     │
│                         ComputeLevelUp, LevelGains pending deltas    │
│   internal/channeling/  Slot refresh + madness tick (Phase E #27)   │
│   internal/effects/     YAML affect catalog (HoT/DoT/buffs)         │
│   internal/affects/     SessionTicker, Apply/Tick/Effective + stack │
│                         caps + ExpireMessage (Phase E #25)          │
│   internal/dialogue/    Tree/Node/Response/Effect data + Validate   │
│                         (Phase F #30)                               │
│   internal/quest/       Per-character engine, talk/kill/reach/script│
│                         steps + RefSets validator (Phase F #31)     │
│   internal/trigger/     Registry + ActionRegistry + Runner +        │
│                         Dispatcher (eventbus → on_enter/say/attack/ │
│                         death/tick) + fault budget (Phase F #29/#32)│
│   internal/scripts/     Embedded Lua catalog (one *.lua per file,   │
│                         SCRIPT_DIR override, syntax-validate boot)  │
│   internal/lua/         Sandboxed gopher-lua runner — LState pool   │
│                         of 8, 50ms ctx cap, V1+V2+V3+V4 APIs        │
│                         (say/emote/log/quest/push_mode/apply_affect/│
│                         give_item/target/room/clock) (Phase F #32)  │
│   internal/chargen/     YAML catalogs (backgrounds, classes, feats, │
│                         skills, weaves, items) + cross-reference    │
│                         validation at Load + chargen.HashID FNV-32a │
│   internal/news/        MOTD/news catalog (YAML)                    │
│   internal/help/        Help-topic catalog (YAML)                   │
│   internal/mob/         Wander tick + trail recording               │
│   internal/prompt/      %h/%H/%r/%g template renderer               │
│   internal/display/     SectionHeader/Subsection/FieldRow/Rule/     │
│                         Defang — shared cfmt-styled render helpers  │
├────────────────────────────────────────────────────────────────────┤
│ Domain models + persistence                                         │
│   internal/auth/        bcrypt Hash / Verify (tunable cost)         │
│   internal/repo/        Account, Character, Room, Exit, Item,       │
│                         MobTemplate, MobInstance, MobTrail, Zone,   │
│                         Channel, Channeling, Shop, ShopStock,       │
│                         Banker, Trainer, WeaveTeacher, Trigger,     │
│                         AdminAudit, AccountLogin, WorldState        │
│                         — sqlite + memory + shared tests            │
│   internal/creature/    Core stat block, Equipment, Channeling,     │
│                         SkillRanks shared by characters + mobs      │
│   internal/affects/     pure Effective/Tick/Apply over Core.Affects;│
│                         SessionTicker for out-of-combat duration    │
│   internal/currency/    Amount type + denomination conversions      │
│   internal/world/       YAML loader (parse → validate → tx-sync),   │
│                         Restocker + ZoneResetter (areaReset — mob   │
│                         respawn + door restore + item respawn),     │
│                         Clock + Day/Night                           │
├────────────────────────────────────────────────────────────────────┤
│ internal/db/            SQLite Open + 48 embedded migrations        │
├────────────────────────────────────────────────────────────────────┤
│ telnet/                 protocol + I/O + mode/registry/dispatch     │
│   server.go             RunSession, readLoop, dispatcher            │
│   session.go            Session, write lock, mode stack, AuthLevel  │
│   iac.go                IAC/SB negotiation (TERM_TYPE, NAWS)        │
│   command.go            Registry, Lookup, Dispatch (Auth check,     │
│                         segmented `;`-chained commands)             │
│   mode.go / completion  Mode interface + Tab completion             │
│   wrap.go / color.go    ANSI-aware wrap, SGR, RGB downsampling      │
│   alias.go / tokenize   Per-session aliases, quoted tokenizer       │
│   lineedit.go / history Line editor + history ring                  │
└────────────────────────────────────────────────────────────────────┘
```

Dependency direction (no cycles): `cmd/server` → `internal/{tick, persist,
eventbus, session, safego, mode, cmd, combat, group, progression,
channeling, affects, effects, dialogue, quest, trigger, scripts, lua,
chargen, news, help, mob, audit, prompt, display, auth, repo, creature,
currency, world, db}` → `telnet`. Cross-package wiring rules: `internal/cmd`
does NOT import `internal/mode` (chargen-cycle risk) — the `talk` verb
takes a `cmd.PushDialogueFn` closure that `cmd/server/main.go` constructs
after both packages resolve. `internal/dialogue` and `internal/cmd` do NOT
import `internal/quest` — quest-effect dispatch goes through
`mode.DialogueHooks` (closure-injection from `cmd/server/main.go`).
`progression` imports `repo` for `repo.Character` input; `repo` does NOT
import `progression` — `repo.LevelUpFields` mirrors `LevelGains`.

## Boot

```
main ─► db.Open(DB_DSN)                    runs embedded migrations 0001-0048
     ─► repo.NewSQLite{Account, Character, AccountLogin,
        Room, Exit, Item, MobTemplate, MobInstance, MobTrail,
        Zone, Channel, Channeling, Shop, Banker, Trainer,
        WeaveTeacher, Trigger, AdminAudit, WorldState}Repo
     ─► news.Load + help.Load + chargen.Load + effects.Load + quest.Load
     ─► scripts.Load (compiles every *.lua at boot — syntax fails loud)
     ─► world.LoadAndSync(ctx, conn, world.SourceFS())
                                            seeds rooms/exits/items/mobs/
                                            shops/bankers/trainers/
                                            weave_teachers/triggers/zones;
                                            cross-validates dialogue
                                            script refs against scripts catalog
     ─► world.NewClock(state) + world.NewRestocker(...) + world.NewZoneResetter(...)
        ZoneResetter wired to tick.Buckets.AreaReset (mob respawn → door
        restore via ExitRepo.RestoreAuthored → item respawn)
     ─► combat.NewManager(...) + group.NewManager()
        combatMgr.SetGroupResolver(groups.MembersInRoom)
     ─► affects.NewSessionTicker(candidates, combatMgr, chars, bus)
        wired to tick.Buckets.Affects (6 s)
     ─► channeling.NewSessionTicker(candidates, chars, bus)
        wired to tick.Buckets.Regen (30 s)
     ─► quest.NewEngine(catalog, chars, mobTpls, ...) + Subscribe to
        combat.CombatDeath (kill_n) + world.PlayerEntered (reach_room)
     ─► lua.NewRunner(scripts) + APIBindings wiring
        (trigger.LuaHooks → quest+affect+item+target+room+clock)
     ─► trigger.NewRegistry + ActionRegistry (noop/say/emote/lua) +
        Runner + Dispatcher (eventbus → on_enter/say/attack/death/tick)
     ─► session.NewRegistry()               single-session-per-account
     ─► eventbus.New()                      typed pub/sub
     ─► tick.New() + tick.NewBuckets()      combat / regen / areaReset / save / wander / phase / decay / affects
     ─► persist.New()                       autosave + shutdown flush
     ─► buildRegistry(...)                  ~70 verbs registered
                                            (cmd.PushDialogueFn closure
                                            constructed after mode + cmd
                                            resolve; mode.DialogueHooks
                                            wired to quest engine)
     ─► mode.NewGame(registry)              shared in-world target
     ─► server{...}                         all deps; rebootOnExit flag
     ─► scheduler.Start(ctx)                heartbeat goroutines via safego
     ─► net.Listen → Accept loop            per-conn goroutines

Shutdown order (reverse — matters for ctx cancellation propagation):
     stop ── triggers.Stop() → lua.Runner.Stop() → channeling ticker
              → affects ticker → bus.Stop() → persist.FlushAll → conn close
```

## Per-connection lifecycle

```
Accept ─► NewSession ─► writeBanner ─► PushMode(srv.newInitial())  = fresh Login
                                    ─► RunSession
                                               │
                                   ┌───────────┴──────────┐
                                   ▼                      ▼
                              readLoop (1 g/r)      runDispatcher (1 g/r)
                              dispatchByte:         inbox ─► Mode.Handle(ctx)
                                ├ IAC/ANSI         ─► Registry.Dispatch
                                ├ CR/LF            ─► segment on `;`
                                ├ BS/DEL/HT        ─► Auth check + Run
                                └ printable/edit
                                  └─ inbox (cap 16)

teardown:
  ├ readLoop exits on EOF / 10m idle / flood
  ├ runDispatcher drains inbox and exits
  ├ safego wrappers recover panics
  ├ groups.ClearForCharacter(charID); session.Following clears implicitly
  ├ if s.AccountID != 0: sessions.Unbind(s.AccountID, s)  (CAS)
  └ s.Conn.Close()
```

## Auth + character pipeline

```
Login → password verify → sessions.Bind (kicks prior) → postAuth
postAuth.applyAccountSettings (color/width/MOTD)
postAuth:
  0 chars → CharacterCreate (chargen substep machine — see "Chargen" below)
  ≥1 char → AccountMenu (delete / password / settings / security / play)
            └─► CharacterSelect → promoteToGame
promoteToGame stamps AuthLevel + CurrentRoomID + CharacterID/Name onto
session, applies channel mute settings, replaces mode → Game.
news.WriteMOTDBlock renders unseen entries (last_news_seen watermark).
```

## Chargen substep machine

```
chargenStepName        → name validated like account username
chargenStepRace        → human | ogier
chargenStepBackground  → Catalog.BackgroundsForRace; pick by id|#|info
chargenStepClass       → Catalog.ClassesForRace; ogier filtered out
                         of channeler classes per book lore
chargenStepAbilities   → point-buy 25-pt budget, [8..18], cost table
chargenStepIdentity    → gender/age/handed/align/roll height & weight
chargenStepFeat        → bg.BonusFeats auto-merged + one player pick
chargenStepSkills      → budget = max(1, class.SkillPoints+IntMod)×4;
                         class∪bg skills, cap 4 each
chargenStepChanneling  → (channeler classes only) source/affinities/
                         3 level-0 weaves (channeling_json, 0033)
chargenStepEquipment   → starting bundle clone via ItemRepo.Create +
                         auto-equip first armor/shield/outfit/weapon
chargenStepReview      → render full draft; commit via Character.Create
```

Catalog string ids hash to int32 via `chargen.HashID` (FNV-32a) so
`Character.Feats []int32` and `Character.Skills map[int32]SkillRanks`
round-trip through `feats_json` / `skills_json`. Cmd-layer spend verbs
(`learn`; future `pick feat`/`bump`/`learn weave`) call the same hash.

## Combat pipeline (§D #18-22, #19)

```
attack <mob|player> ─► combat.Manager.EnqueueAction(Fight, ActionAttack)
                       Fight is per-room; init = d20+DexMod+InitMod
tick.Buckets.Combat (4s) ─► Manager.Tick:
   ├ pruneDead (stable Order during resolution)
   ├ pop active actor's queued action
   ├ RollAttack (d20 + BAB + StrMod vs Defense; nat-1/20; crit threshold)
   ├ RollDamage (weapon dice + StrMod; ×CritMult on crit; floor 1)
   ├ applyDamage (DR clamp → Resists percent → Subdual route)
   ├ tally damage to Fight.DamageTally + Fight.Threat
   ├ write HP via MobInstanceRepo.UpdateLive / CharacterRepo.RecordCore
   └ on HP≤0:
       ├ mob   → handleMobDeath (corpse spawn, despawn, XP allocation
       │                        via expandTallyByGroup → GroupResolver split;
       │                        applies XP debt drain via ApplyXPAward)
       └ char  → handleCharacterDeath (heal to HPMax, clear CondDying/
                  CondUnconscious + position_flags, move to BoundRoomID,
                  compute DeathDebt = 10% of level XP, write via RecordXPDebt,
                  publish CharacterDied + CharacterRespawned)
```

Death dispatch: `resolveAction` switches on `ActorKind` (mob vs character).

XP-debt model: Passive offset via `ApplyXPAward(award, debt) → (gain, newDebt)`.
On next XP gain, debt is drained off the top before crediting. No tick-scheduled
decay. Player retains inventory/equipment/coin; penalty is in XP only.

CharacterDied event: broadcast to death-room peers (via `combatBroadcastExcept`
to skip victim), victim gets private "You die!" message.

CharacterRespawned event: cmd-layer subscriber calls `Session.SetCurrentRoom` +
`RenderRoom` in detached context, broadcasts arrival to peers in bound room.

PvP gate (`attack <player>`): NoPVP-room → newbie (level<10) →
attacker opt-in → target opt-in → same-group refusal. Same-group + party
follow chain bound by `followDepth = 16`. Ordinal targeting via
`MatchPlayer(target, sessions, self)` mirrors `MatchItem`/`MatchMob`.

## Progression flow (§E #23-25)

```
mob death ─► Fight.DamageTally → expandTallyByGroup(GroupResolver)
                              → CharacterRepo.RecordXP (delta)
xp verb   ─► progression.XPToNext(char.XP) — read-only
train     ─► resolve trainer in room → progression.ComputeLevelUp →
             repo.RecordLevelUp(LevelUpFields):
               • absolute new HP/BAB/Saves/ClassLevels
               • pending_x += FeatDelta/SkillDelta/AbilityDelta/WeaveDelta
             audit row written.
learn     ─► spend pending_skill_points anywhere; allowed list = class∪bg
             skills; cap = level+3; cost 1 pt/rank.
             repo.RecordSkillRank atomic upsert (TX: select skills_json →
             merge → UPDATE skills_json + pending_skill_points). Audit on
             success only.
learn weave ─► spend pending_weaves (channeler-only); affinity-gated;
               allowed list = level-0 weaves matching Channeling.Affinities.
               repo.RecordWeavePick TX-upsert (select channeling_json →
               merge → UPDATE channeling_json + pending_weaves). Audit only.
feat      ─► spend pending_feats anywhere; allowed list = bg.BonusFeats
             ∪ class.BonusFeats (background-aware); no prereq enforcement V1.
             repo.RecordFeatPick TX-upsert (select feats_json → merge →
             UPDATE feats_json + pending_feats). Audit only.
bump      ─► spend pending_ability_bumps to raise one ability by +1
             (hard cap 20 per ability). Format: `bump str` or `bump strength`.
             repo.RecordAbilityBump atomically UPDATEs the *_cur column via
             injected column name (str_cur/dex_cur/con_cur/int_cur/wis_cur/cha_cur).
             Audit only.
```

All four pending_* pools (feats/skill_points/ability_bumps/weaves) are now
drained by player-driven spend verbs. Level-up cycle is end-to-end functional.

## Triggers / dialogue / quests / Lua (§F #29-32)

```
Eventbus pub                       Trigger Dispatcher          Action handler
-------------------------          ----------------------      ----------------
world.PlayerEntered     ─►  on_enter
world.PlayerSaid        ─►  on_say   (substring match)
combat.CombatHit        ─►  on_attack
combat.CombatDeath      ─►  on_death  ─► Runner.Fire(prio DESC, swallow err)
combat.CharacterDied    ─►  on_death                              ├ noop
tick.Buckets.Phase      ─►  on_tick                              ├ say
                                                                  ├ emote
                                                                  └ lua  ─► Runner.Run
                                                                            (50ms ctx,
                                                                            sandbox,
                                                                            fault budget)
```

`talk <mob>` ─► resolves NPC, decodes `MobTemplate.DialogueJSON` ─►
`mode.Dialogue` (per-character flag bag + node id; effects fire in
order: set_flag/clear_flag/goto/push_mode/end/script/accept_quest/
advance_quest). `script` effect routes through `mode.DialogueHooks
.RunScript` (closure-injected from main.go) → `lua.Runner.Run` with
the V2/V3 hooks bound (`quest.accept`/`quest.advance`/`apply_affect`/
`give_item`/`target.*`/`room.*`/`clock.*`).

Quest engine (`internal/quest/Engine`):
- Subscribes to `combat.CombatDeath` (kill_n) and `world.PlayerEntered`
  (reach_room).
- `talk_to` advances via dialogue `advance_quest` effect (closure-
  injected, no compile dep on `internal/dialogue`).
- `script` step waits for an external `quest.advance(id)` Lua call —
  no event subscription. Validated against script catalog at boot.
- Reloads quest log per transition (correctness > throughput on the
  bus goroutine). Final-step: grants XP via `RecordXP` + coin via
  `RecordCoin` (one optimistic-lock retry on `ErrCoinConflict`).

Lua sandbox (`internal/lua`):
- LState pool of 8 (pre-allocated buffered channel; no overflow alloc).
- Strips `os` / `io` / `debug` / `package` / `dofile` / `loadfile` /
  `loadstring` / `load`.
- 50ms `CallTimeout` per call via gopher-lua `SetContext`.
- Release wipe-list resets every binding (`quest`/`push_mode`/
  `apply_affect`/`give_item`/`target`/`room`/`clock`/`say`/`emote`/
  `log`) so a pooled LState never observes a leaked closure.
- nil-bound hooks register classified-error stubs so misuse trips the
  trigger fault budget (`triggers.consecutive_faults`, auto-disable at 5).

## Tick buckets

| Bucket | Cadence | Subscribers |
|---|---|---|
| `Combat` | 4 s | `combat.Manager.Tick` per active room (also publishes per-round affects.Tick on participants so DoT/HoT credit at round-end) |
| `Regen` | 30 s | `channeling.SessionTicker` — slot refresh (8h wall-clock interval, refills `Slots[*].Cur`) + madness accrual (Embraced + SourceSaidin only; clamped at int16 max) (Phase E #27) |
| `AreaReset` | 5 min | `world.Restocker` (refill sub-max shop_stock); `world.ZoneResetter` per-zone gate runs three steps in order — mob respawn from anchored templates, door restoration via `ExitRepo.RestoreAuthored`, item respawn via `ItemRepo.FindByExternalID` global presence check + `Create` (§7) |
| `Affects` | 6 s | `affects.SessionTicker` walks `sessions.Snapshot()`, skips characters in active fights, decrements `Core.Affects` durations + applies `TickEffect` HP deltas, persists via `CharacterRepo.RecordCore`/`RecordAffects`. Publishes `affects.Expired` (with Entries[].ExpireMessage) and `affects.TickDamaged` events (Phase E #25/#26) |
| `Phase` | 4×/day-cycle | `world.PhaseAmbient` → room ambient broadcast on dawn/midday/dusk/midnight; trigger Dispatcher fans `on_tick` to subscribed triggers |
| `Wander` | 30 s | `mob.Wander` rolls `MobTemplate.WanderChance` per instance and walks via random exit; appends `mob_trails` row |
| `Save` | 30 s | `persist.Manager` autosave (lastPlayed, ticks counter) |

## Cross-session output rule

Any write that targets a session other than the dispatcher's
`c.Session` MUST go through `Session.WriteAsync` (not `WriteString`).
WriteAsync wraps the message with CR+EL erase prefix and replays the
cached prompt + line-edit buffer so a mid-line broadcast doesn't clobber
the player's input. This is universally enforced by `say`, `tell`,
`shout`/`yell`, channel verbs, `give`/`put`/`get`/door verbs, mob
arrivals/departures, combat hits, group follow chain output, the
shutdown countdown, and chargen review broadcasts.

## Entry points

- `cmd/server/main.go::main` — listener, DI wiring, accept loop.
- `cmd/server/main.go::handleConnection` — per-connection setup,
  group cleanup defer, sessions.Unbind on teardown, hands off to
  `telnet.RunSession`.
- `telnet.RunSession` — drives the read + dispatch goroutines.

### `cmd/server/` sibling files

main.go is the orchestrator; each concern lives in its own sibling:

- `registry.go` — `buildRegistry`: every verb registration in one
  file, grouped by subsystem (comm, movement, combat, OLC, …).
- `lua_bindings.go` — all `makeLua*` factories, wait-slot
  acquire/release, target/wait-ctx resolution, `defangScriptSource`.
- `subscribers_combat.go` — `setupCombatSubscribers`: every combat
  event subscriber (Hit/Miss/Death/XPAwarded/Flee/Stance/Parry +
  CharacterDied/Respawned + script Damage/Heal + affects Expired/
  TickDamaged).
- `tickers.go` — `setupTickers`: corpse decay + affects + channeling
  + stamina regen ticker wiring (subscribes onto `tick.Buckets`).
- `bootstrap_observability.go` — `setupMetrics` (Prom + pprof +
  healthz HTTP) + `setupBackup` (VACUUM-INTO manager).
- `shutdown_admin.go` — `RequestShutdown` / `RequestReboot` /
  `RequestAbort` + `scheduleStop` + countdown broadcaster +
  `broadcast` helper.
- `mssp.go` — `msspWorldStats` type + `collectMSSPWorldStats` +
  `msspVars`.
- `adapters.go` — `characterAffectsLoader`,
  `characterChannelingLoader`, `eventbusAdapter`, `combatActorName`,
  `variantHit/MissSelfFormat`.
- `audit_metrics.go` — `buildCommandAuditFn` + `buildCommandMetricFn`.
- `catalog_validate.go` — `buildQuestRefSets`,
  `validateDialogueScriptRefs`, `validateConsumableEffectRefs`.

For the authoritative current architecture see `CLAUDE.md` and
`docs/PLAN.md` at the repo root. Roadmap ledger: `ROADMAP.md`.
