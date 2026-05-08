<!-- Generated: 2026-05-07 | Updated for Phase E #25: feat, bump, learn weave | Token estimate: ~1350 -->

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
│                         Banker, Trainer, AdminAudit, AccountLogin,  │
│                         WorldState — sqlite + memory + shared tests │
│   internal/creature/    Core stat block, Equipment, Channeling,     │
│                         SkillRanks shared by characters + mobs      │
│   internal/currency/    Amount type + denomination conversions      │
│   internal/world/       YAML loader (parse → validate → tx-sync),   │
│                         Restocker (areaReset bucket), Clock         │
├────────────────────────────────────────────────────────────────────┤
│ internal/db/            SQLite Open + 39 embedded migrations        │
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
eventbus, session, safego, mode, cmd, combat, group, progression, chargen,
news, help, mob, audit, prompt, display, auth, repo, creature, currency,
world, db}` → `telnet`. `progression` imports `repo` (for `repo.Character`
input to `ComputeLevelUp`); `repo` does NOT import `progression` —
`repo.LevelUpFields` mirrors `LevelGains` to avoid the cycle.

## Boot

```
main ─► db.Open(DB_DSN)                    runs embedded migrations 0001-0039
     ─► repo.NewSQLite{Account, Character, AccountLogin,
        Room, Exit, Item, MobTemplate, MobInstance, MobTrail,
        Zone, Channel, Channeling, Shop, Banker, Trainer,
        AdminAudit, WorldState}Repo
     ─► news.Load + help.Load + chargen.Load
     ─► world.LoadAndSync(ctx, conn, world.SourceFS())
                                            seeds rooms/exits/items/mobs/
                                            shops/bankers/trainers/zones
     ─► world.NewClock(state) + world.NewRestocker(...)
     ─► combat.NewManager(...) + group.NewManager()
        combatMgr.SetGroupResolver(groups.MembersInRoom)
     ─► session.NewRegistry()               single-session-per-account
     ─► eventbus.New()                      typed pub/sub
     ─► tick.New() + tick.NewBuckets()      combat / regen / areaReset / save
     ─► persist.New()                       autosave + shutdown flush
     ─► buildRegistry(...)                  ~50 verbs registered
     ─► mode.NewGame(registry)              shared in-world target
     ─► server{...}                         all deps; rebootOnExit flag
     ─► scheduler.Start(ctx)                heartbeat goroutines via safego
     ─► net.Listen → Accept loop            per-conn goroutines
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

## Combat pipeline (§D #18-22)

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
   └ on HP≤0: handleMobDeath (corpse spawn, despawn, XP allocation
              via expandTallyByGroup → GroupResolver split)
```

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

## Tick buckets

| Bucket | Cadence | Subscribers |
|---|---|---|
| `Combat` | 4 s | `combat.Manager.Tick` per active room |
| `Regen` | 6 s | (HP/subdual regen — pending) |
| `AreaReset` | 5 min | `world.Restocker` (refill sub-max shop_stock) |
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

For the authoritative current architecture see `CLAUDE.md` and
`docs/PLAN.md` at the repo root. Roadmap ledger: `ROADMAP.md`.
