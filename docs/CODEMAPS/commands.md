<!-- Generated: 2026-05-08 | Updated for Phase D #19: death/respawn/XP-debt | Token estimate: ~1500 -->

# Command Catalog

The user-facing surface (post-login) is the verb registry. Pre-login the
surface is whichever `Mode` is on top of the stack — see
`architecture.md` for the auth + chargen pipeline.

## Wiring

`cmd/server/main.go::buildRegistry` constructs the registry once at boot
and registers all verbs. New commands take their dependencies (repos,
session registry, eventbus, catalogs, audit repo, combat manager, group
manager, clock, shutdown controller) by parameter and return a
`*telnet.Command`. Commands with required arguments declare `MinArgs` +
`Long` so the dispatcher emits a Long-aware usage block on too-few-args.

```go
type Command struct {
    Name, Help, Long string
    Aliases []string
    MinArgs int
    Auth    AuthLevel    // checked by Registry.Dispatch
    Run     func(*Context) error
}
```

`Registry.Dispatch` is segment-aware: a top-level `;` outside quotes
splits the input into multiple commands run in order via `dispatchOne`
(alias-expand → lookup → auth → tokenize → run). 16-segment cap;
alias-introduces-`;` bounded at depth 3.

## Catalog (by category)

### Communication

| Verb | Aliases | Auth | File | Purpose |
|---|---|---|---|---|
| `say` | — | Player | `comm.go` | Room broadcast `<Name> says, "..."` |
| `tell` | `whisper` | Player | `comm.go` | Private DM via session registry; sets recipient `LastTellFrom` |
| `reply` | — | Player | `comm.go` | Reply to last tell |
| `shout` / `yell` | — | Player | `shout.go` | Zone-wide broadcast |
| `<channel>` | — | Player | `channel.go` | Dynamic verb per channel row (`ooc`, `gossip`, `newbie`, ...). No args toggles mute; args broadcast |
| `channels` | `ch` | Player | `channel.go` | List channels with on/off mute state |

### Movement & navigation

| Verb | Aliases | Auth | File | Purpose |
|---|---|---|---|---|
| `north`/`south`/`east`/`west`/`up`/`down`/`ne`/`nw`/`se`/`sw` | `n`/`s`/`e`/`w`/`u`/`d` etc. | Player | `move.go` | 10-direction movement via `moveDir`; chains followers in same group |
| `look` | `l` | Player | `look.go` | Render current room; `look in <container>`; `look <item>`/`<mob>`/`<player>` |
| `examine` | `ex`/`exa` | Player | `examine.go` | Detailed item / mob descriptor |
| `map` | — | Player | `map.go` | BFS minimap (default depth 3, max 5) |
| `zonemap` | — | Player | `map.go` | Bigger zone-scoped map |
| `track` | — | Player | `track.go` | Read recent `mob_trails` entries for tracking |
| `time` | — | Player | `time.go` | In-world clock from `world.Clock` |
| `whereami` | — | Admin | `whereami.go` | Current room id + zone for builders |
| `zones` | — | Admin | `zones.go` | List zones with room counts |
| `coords` | — | Admin | `coords.go` | `coords rebuild`/`show <id>`/`issues` — auto-coord BFS pass |

### Inventory & equipment

| Verb | Aliases | Auth | File | Purpose |
|---|---|---|---|---|
| `inventory` | `i`/`inv` | Player | `inventory.go` | List held items, annotated `(worn)`/`(wielded)`/`(offhand)` |
| `get` | `take` | Player | `inventory.go` | `get <item>`, `get <item> from <container>` |
| `drop` | — | Player | `inventory.go` | Auto-unequips if held in a slot |
| `give` | — | Player | `inventory.go` | `give <item> <player>` |
| `put` | — | Player | `container.go` | Capacity-aware container insert |
| `wear` / `wield [off]` / `remove` | — | Player | `equipment.go` | Driving `creature.Equipment` slot map (overlay on inventory) |
| `equipment` | `eq` | Player | `equipment.go` | Slot listing |

### Door verbs

| Verb | Auth | File | Purpose |
|---|---|---|---|
| `open` / `close` / `lock` / `unlock` / `pick` | Player | `door.go` | Door-flagged exit ops; reverse-side broadcast |

### Economy

| Verb | Auth | File | Purpose |
|---|---|---|---|
| `list` / `buy` / `sell` / `value` | Player | `shop.go` | Resolve shopkeeper from `mobs.ListInRoom` + `shops.GetByMobTemplateID`; honours `FlagTradeGood`, `FlagNoSell`, `BuyTypes`. `buy` clones template; `sell` deletes |
| `balance` / `deposit` / `withdraw` | Player | `banker.go` | Resolve banker; gate on `Banker.IsOpenAt(clock.HourOfDay())`; coin via `RecordCoin(coin, bank, expectedVersion)` with `ErrCoinConflict` retry messaging |

### Combat (§D #18-22)

| Verb | Aliases | Auth | File | Purpose |
|---|---|---|---|---|
| `attack` | `kill` | Player | `attack.go` | Resolves room mob via `MatchMob` or peer via `MatchPlayer`; PvP guard order; queues `ActionAttack`. Reissue while fight active switches target |
| `flee` / `run` | — | Player | `flee.go` | Flee from active fight (sector gate pending) |
| `parry` | — | Player | `parry.go` | Toggles parry stance; CondFlatFooted enforcement |
| `pvp` | — | Player | `pvp.go` | Toggle PvP opt-in (migration 0037, `RecordPvP`) |
| `score` | — | Player | `score.go` | Read-only stat sheet using `internal/display` helpers |

### Group / party (§D #22)

| Verb | Auth | File | Purpose |
|---|---|---|---|
| `group` | Player | `group.go` | Bare = roster. Subverbs: `invite`/`accept`/`decline`/`leave`/`kick`/`disband` |
| `follow <player>` / `unfollow` | Player | `follow.go` | Same-party + same-room + non-cycling. Move chain; on follower failure clears with "couldn't keep up" |

### Progression (§E #23-25)

| Verb | Auth | File | Purpose |
|---|---|---|---|
| `xp` | Player | `xp.go` | Read-only — pending level via `progression.XPToNext`; shows "XP debt: N" line when `Character.XPDebt > 0` (Phase D #19) |
| `train` | Player | `train.go` | Trainer-NPC commit one class level (HP/BAB/saves + 4 pending pool deltas via `RecordLevelUp(LevelUpFields)`); audit on success |
| `learn` | Player | `learn.go` | Spend `pending_skill_points` anywhere. `learn` (menu), `learn <id\|#> [n]`, `learn info <id>`. Cap = level+3; class∪bg skills only V1. Routes to `learn weave` if first arg == "weave" |
| `learn weave` | Player | `learn_weave.go` | Channeler-only weave-pick. `learn weave` (menu), `learn weave <id>`. Affinity-gated; drains `pending_weaves`; audit on success |
| `feat` | Player | `feat.go` | Spend `pending_feats` anywhere. `feat` (menu), `feat <id>`, `feat info <id>`. Background-aware allowed-list; no prereq enforcement V1; audit on success |
| `bump` | Player | `bump.go` | Spend `pending_ability_bumps` to raise an ability by +1 (hard cap 20). `bump <ability>` accepts str/dex/con/int/wis/cha or full names; audit on success |

### Help & UX

| Verb | Aliases | Auth | File | Purpose |
|---|---|---|---|---|
| `help` | `?` | Guest | `help.go` | YAML-driven help-topic catalog; exact → keyword → unique-prefix match |
| `colors` | `colortest`/`palette` | Player | `colors.go` | 16/256/RGB palette samples |
| `who` | — | Player | `who.go` | Online players from `session.Registry.Snapshot`; PvP tag; wizinvis-filtered |
| `news` | — | Player | `news.go` | MOTD/news catalog; `news <id>` advances `last_news_seen` |
| `prompt` | — | Player | `prompt.go` | Per-character prompt template |
| `alias` / `unalias` | — | Player | `alias.go` | Per-session aliases |

### Admin (AuthAdmin)

| Verb | File | Purpose |
|---|---|---|
| `teleport` (`tp`) | `teleport.go` | `teleport <room_id>` jumps; bypasses exits |
| `goto <player\|room>` | `admin_movement.go` | Thin wrapper over teleport for player names or room ids |
| `transfer <player> [<room>]` | `admin_movement.go` | Yank a player to current room or named room |
| `summon <player>` | `admin_movement.go` | Bring a player to the caller |
| `wizinvis` | `wizinvis.go` | Toggle session wizinvis (filters from `who` and tell-name completion) |
| `spawn mob <ext> [count]` / `spawn item <ext> [count]` | `spawn.go` | Clone YAML-seeded template into current room |
| `shutdown` / `reboot` | `shutdown.go` | `[<delay>] [<reason>]` (default 30s, ≤1h) or `cancel`/`abort`. Countdown broadcasts at T-{60,30,10,5..0}s. `reboot` re-execs via `syscall.Exec` after drain |
| `quit` | `quit.go` | Closes the session (returns `ErrSessionEnded`) — Player auth, listed here for completeness |

## Shared helpers

- `keyword.go::MatchItem` / `MatchMob` / `MatchPlayer` — resolve by
  exact / prefix / ordinal (`2.sword`, `2.alice`).
- `level.go::characterLevel(ch)` — sum of `ClassLevels` values.
  `NewbiePvPLevelCap = 10`.
- `encumbrance.go::LoadFor` — Str-keyed d20 carrying-capacity table;
  reads transitive ownership via `ItemRepo.ListAllOwnedTransitive`.
- `flee_mover.go` — `combat.Manager.SetFleeMover` callback.
- `chargen.HashID` (in `internal/chargen/hash.go`) — FNV-32a id hash
  shared with chargen substep persistence so spend-verb keys round-trip.

## Audit policy

Privileged verbs and progression spends record one `admin_audit` row per
successful invocation via `internal/audit.Record(ctx, audits, session,
verb, target, args)`. Refusal paths (auth denied, bad target, NoTeleport,
controller error, unknown template, cap/budget refusal, repo error) MUST
NOT audit — the row represents "this side effect actually happened."

Verbs that audit on success: `spawn`, `teleport`, `goto`, `transfer`,
`summon`, `wizinvis`, `shutdown`, `reboot`, `train`, `learn`/`learn weave`,
`feat`, `bump`, banker `deposit`/`withdraw`. Synchronous by design so a
`shutdown` row commits before drain begins.

## Adding a command

```go
// internal/cmd/foo.go
func NewFoo(deps...) *telnet.Command {
    return &telnet.Command{
        Name: "foo", Aliases: []string{"f"},
        Help: "one-liner", Long: "multi-line usage",
        MinArgs: 0, Auth: telnet.AuthPlayer,
        Run: func(c *telnet.Context) error {
            // c.Ctx, c.Session, c.Args, c.Raw available
            return c.Session.WriteString("...")
        },
    }
}
```

Register in `buildRegistry`. Verb names must be lowercase ASCII with no
whitespace (`validVerb` rejects others at registration time). Cross-
session output goes through `WriteAsync`; the dispatcher's reply to its
own session uses `WriteString`.

## Auth gating

- `AuthGuest` (default until login): `help` only.
- `AuthPlayer` (post-login): all game commands + comms + channels +
  combat + progression spend.
- `AuthAdmin`: teleport / goto / transfer / summon / wizinvis / spawn /
  shutdown / reboot / coords / whereami / zones. Denials render as
  `"Unknown command"` so the prompt can't enumerate privileged verbs.

## Deferred

- Argument tab-completion for `attack <player>`, `tell`, etc. (verb-name
  completion landed; arg side pending).
- Cross-class skill picks at half rate (`learn` cross-class, deferred
  per chargen V1 stance).
- Feat prereq enforcement, ability bump keying off con modifier,
  weave rank caps — template work deferred.
- `consider <player>`, `assist`, `gtell`, leader succession, invite
  expiry — see `combat_*_followups.md` and `progression_*_followups.md`
  in the auto-memory tree.
