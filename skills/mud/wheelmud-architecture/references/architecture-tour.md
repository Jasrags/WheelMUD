# Architecture tour

Top-level wiring of WheelMUD. This is the "where does X live" reference.

## Entrypoint

`cmd/server/main.go` — reads env, opens DB (`internal/db.Open` runs
embedded migrations 0001–0032+), constructs every repo, loads the
news catalog (`internal/news`) and the chargen catalog
(`internal/chargen`), runs `world.LoadAndSync` to seed the DB from
`WORLD_DIR`, builds the command registry, instantiates the `server`
struct holding long-lived deps, starts `tick.Scheduler` +
`tick.Buckets` + `persist.Manager` autosaver, then accepts TCP
connections.

## Long-lived vs per-connection

- **Long-lived (on `server` struct):** repos, registry, eventbus, tick
  scheduler, persist manager, news catalog, chargen catalog,
  `session.Registry`, `world.Restocker`, `world.Clock`.
- **Per-connection:** `telnet.Session` (one per TCP conn), the mode
  stack (lives on the Session), the dispatcher goroutine, the read
  goroutine.

## Where new deps go

- A new long-lived dep goes on the `server` struct, constructed in
  `main.go`, and threaded into command factories that need it.
- A new per-connection state field goes on `telnet.Session` and
  follows the `crossMu` rule if any goroutine other than the dispatcher
  reads/writes it.

## Cross-references

- `CLAUDE.md` "Architecture" section — primary source.
- `docs/CODEMAPS/architecture.md` — token-lean map for AI context.
- `docs/CODEMAPS/dependencies.md` — third-party module surface.
