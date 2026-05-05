# WheelMUD

A Wheel of Time MUD server written in Go. Telnet on TCP `:2323`, SQLite-backed
world + accounts, mode-stack command dispatcher, ANSI/cfmt rendering with
width-aware reflow.

> Status: pre-alpha. Network/protocol, login + character create/select,
> connect splash + per-character MOTD/news on login, world loader
> with hierarchical zone tree (continent/nation/region/settlement/
> building) and persisted zones table, room/exit/item/mob
> persistence with door verbs (open/close/lock/unlock/pick) and
> item taxonomy (weapons/armor/containers/keys/lights/etc.),
> container semantics (`put` / `get <item> from <container>` /
> `look in`, capacity limits, recursive contents), movement +
> look + examine + teleport, inventory verbs (inventory / get /
> drop / give) with Str-based encumbrance and currency-aware
> `give 5sp <name>`, communication (say / tell / reply / shout /
> yell / channels), session registry, persist manager, tick
> scheduler, BFS minimap + `zonemap` + auto-derived room
> coordinates (`coords rebuild|show|issues`), `track` heuristic,
> `time` clock, and admin tooling (whereami, zones, `spawn mob`
> / `spawn item`, news authoring) are in. Combat, skills, quests,
> full OLC, area resets, banks/shops, and broader operator
> tooling are pending.
> See [`ROADMAP.md`](ROADMAP.md) for the full punch list.

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
cmd/server/         entrypoint
telnet/             protocol, session, registry, mode stack, color, wrap
internal/cmd/       concrete commands (look, move, say, shout/yell, channel, who, examine, door verbs, inventory verbs, put, spawn, map/zonemap/coords/track, news, zones, ...)
internal/mode/      login, character_select, character_create, game, postauth
internal/repo/      account, character, room, exit, item, mob_*, channeling, channel, zone, mob_trail, news
internal/db/        sql.DB open + embedded migrations
internal/world/     YAML world loader + sync to DB
internal/session/   single-session-per-account registry
internal/eventbus/  typed pub/sub
internal/persist/   periodic + shutdown autosave manager
internal/tick/      scheduler + named buckets (Save, Combat, Regen, AreaReset, ...)
internal/safego/    panic-recovery goroutine wrapper
internal/auth/      bcrypt password hashing
internal/creature/  Core stat block, Channeling weave model
internal/currency/  copper-piece amount type
data/world/         authored zone YAML — hierarchical layout (continent/nation/region/settlement/building);
                    Emond's Field is the reference template. See data/world/README.md for the schema.
docs/CODEMAPS/      token-lean architecture maps for AI context
docs/reference/     game-system reference docs (abilities, classes, ...)
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
