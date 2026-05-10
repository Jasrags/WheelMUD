# WheelMUD

A Wheel of Time MUD server written in Go. Telnet on TCP `:2323`, SQLite-backed
world + accounts, mode-stack command dispatcher, ANSI/cfmt rendering with
width-aware reflow.

> Status: pre-alpha. Network/protocol, login + post-login account menu
> (delete-character, password, settings, security), character
> create/select with full WoT chargen (background/class/abilities/
> feats/skills, channeler-branch affinities + starting weaves),
> connect splash + per-character MOTD/news, hierarchical world loader
> (continent/nation/region/settlement/building YAML →
> zones/rooms/exits/items/mobs/shops/bankers/triggers/dialogue),
> door verbs with authored-state restore on zone reset, full item
> taxonomy + recursive container nesting, inventory + encumbrance +
> equipment slots, communication (say/tell/reply/shout/yell/channels),
> BFS minimap/zonemap/auto-coords/track/time, d20 combat (initiative,
> hit/miss, damage, threat, mob death + loot, player death/respawn
> with XP debt, PvP opt-in, parties/follow), progression (XP curve,
> trainer NPCs, `learn`/`feat`/`bump`/`learn weave` spend verbs,
> mid-game weave teachers + practice points, channeler slot refresh
> + madness, affects/buffs/DoT/HoT with stacking caps + multi-charge
> consumables, per-verb command lag + per-skill cooldowns), shops
> (list/buy/sell/value with restock), bankers (balance/deposit/
> withdraw with hour gate), NPC dialogue trees + per-character quest
> engine (talk/kill/reach/script steps with rewards), declarative
> trigger system (on_enter/on_say/on_attack/on_death/on_tick) with
> sandboxed embedded Lua scripting (50ms ctx cap, fault budget,
> APIs for say/emote/log/quest/push_mode/apply_affect/give_item/
> target/room/clock), and admin tooling (`spawn`/`teleport`/`goto`/
> `transfer`/`summon`/`wizinvis`/`shutdown`/`reboot`/`affect`/
> `cooldown`/news authoring) with append-only audit log. Full OLC,
> mail/boards, broader network protocols (GMCP/MSSP/MCCP/TLS/WS),
> and CI matrix are pending. See [`ROADMAP.md`](ROADMAP.md) for the
> full punch list.

## Quick start

```bash
make build/server          # go build -o /tmp/bin/server cmd/server/main.go
make run/server            # build then run
make run/live/server       # hot reload via cosmtrek/air
docker compose up          # build + run, exposes :2323
go test -race ./...        # full test suite
```

Connect:

```bash
telnet localhost 2323
# or
nc localhost 2323
```

## Configuration

Environment variables, all optional:

| Var           | Default          | Purpose                                            |
| ------------- | ---------------- | -------------------------------------------------- |
| `LISTEN_ADDR` | `:2323`          | TCP listen address                                 |
| `DB_DSN`      | `wheelmud.db`    | SQLite DSN; `:memory:` for ephemeral runs          |
| `LOG_LEVEL`   | `debug`          | `debug` / `info` / `warn` / `error`                |
| `WORLD_DIR`   | `./data/world`   | YAML zone tree the world loader syncs into the DB  |

## Layout

```
cmd/server/           entrypoint — wires repos, catalogs, registry, tickers, dispatcher
telnet/               protocol, session, registry, mode stack, color, wrap, alias, history
internal/cmd/         concrete verbs (look/move/say/tell/shout/channel/who/examine/door/
                      inventory/equipment/shop/banker/quaff/attack/flee/parry/pvp/group/
                      follow/score/xp/train/learn/feat/bump/embrace/release/affects/
                      cooldowns/talk/quest/spawn/teleport/goto/transfer/summon/wizinvis/
                      shutdown/reboot/map/zonemap/coords/track/time/news/whereami/zones/
                      affect/dispel/cooldown)
internal/mode/        login, character_select, character_create, account_menu, postauth,
                      game, dialogue
internal/repo/        account, character, room, exit, item, mob_template, mob_instance,
                      mob_trail, zone, channel, news, shop, banker, trainer, weave_teacher,
                      trigger, admin_audit, account_login
internal/db/          sql.DB open + embedded migrations 0001–0048
internal/world/       YAML loader + sync to DB; Restocker, ZoneResetter, Clock, day/night
internal/chargen/     YAML content catalog (backgrounds/classes/feats/skills/weaves)
internal/news/        embedded MOTD/news catalog
internal/effects/     YAML affect catalog (HoT/DoT/buffs feeding §E #25)
internal/affects/     per-tick session driver, stacking, expiry events
internal/combat/      d20 hit/damage, initiative, threat, group XP split, death pipeline
internal/group/       in-memory party manager (invite/accept/leave/kick/disband)
internal/progression/ XP curve + per-class level-up math (pure functions)
internal/channeling/  slot refresh + madness tick (Phase E #27)
internal/dialogue/    NPC dialogue tree model + validator
internal/quest/       per-character quest engine (talk/kill/reach/script steps)
internal/trigger/     declarative event registry + dispatcher (on_enter/say/attack/death/tick)
internal/scripts/     embedded Lua script catalog (one *.lua per file)
internal/lua/         sandboxed gopher-lua runner (LState pool, 50ms ctx cap, V1+V2+V3 APIs)
internal/audit/       append-only admin/account audit row writer
internal/session/     single-session-per-account registry
internal/eventbus/    typed pub/sub
internal/persist/     periodic + shutdown autosave manager
internal/tick/        scheduler + named buckets (Save, Combat, Regen, Affects, AreaReset, ...)
internal/safego/      panic-recovery goroutine wrapper
internal/auth/        bcrypt password hashing
internal/creature/    Core stat block, Channeling weave model, Equipment slot map
internal/currency/    copper-piece amount type
data/world/           authored zone YAML — hierarchical (continent/nation/region/settlement/
                      building); Emond's Field is the reference. See data/world/README.md.
docs/CODEMAPS/        token-lean architecture maps for AI context
docs/PLAN.md          sequenced plan of attack across roadmap phases
docs/reference/       game-system reference docs (abilities, classes, ...)
```

## Documentation

- [`CLAUDE.md`](CLAUDE.md) — guidance for Claude Code agents
- [`ROADMAP.md`](ROADMAP.md) — feature punch list + status
- [`docs/PLAN.md`](docs/PLAN.md) — sequenced plan of attack across roadmap phases
- [`docs/CODEMAPS/`](docs/CODEMAPS/) — architecture, command catalog, data model, dependencies, telnet protocol
- [`docs/reference/`](docs/reference/) — game-system rules ported from the WoT RPG
- [`data/world/README.md`](data/world/README.md) — zone YAML schema, room ID conventions, currency format, item taxonomy
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — dev workflow, testing, commit conventions

## License

See [`LICENSE`](LICENSE).
