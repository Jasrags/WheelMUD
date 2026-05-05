# World YAML conventions

The world loader (`internal/world/`) syncs `WORLD_DIR` (default
`./data/world`) into the DB on startup.

## Source of truth

`data/world/README.md` is the canonical schema doc. This reference is a
pointer + the rules that don't fit there.

## Hierarchy

`data/world/<continent>/<nation>/<region>/<settlement>/<building>/zone.yaml`

Today: `westlands`, `aiel_waste`, `seanchan`, `shara`, `tremalking`,
`oceans`.

## zone.yaml top-level keys

- `id` (string, unique, kebab-case)
- `name`
- `builder`
- `level_range` (e.g. "1-5")
- `reset_interval_s`
- `reset_mode`
- `climate`
- `ambient` (per-sector phase ambient strings)

Plus optional sub-blocks per mob:
- `shop:` — `sell_markup`, `buy_markdown`, `buy_types`, `stock`
- `banker:` — operating hours

## Room ids

Form: `<zone-id>:<room-suffix>` — kebab-case both halves. The loader
stamps zone ids into rooms via `rooms.zone_id`.

## Currency strings

Coin values render and parse as `"5g 3s 7c"` style strings. See
`internal/coin/` for the parser.

## Typed item stats

Items use the migration 0015 taxonomy (weapon/armor/container/...);
stats are typed JSON columns, not free text. New stat shapes need a
migration.

## Loader transactionality

`internal/world/loader.go` writes raw SQL inside one transaction per
table — that's why the column lists are duplicated against the repo
`Create` paths. See `repo-and-migration-rules.md` for the lock-step
list.

## Restocker + clock

`world.Restocker` (refills sub-max `shop_stock` lines older than
`restock_interval_s`) is wired to `tick.Buckets.AreaReset` (5min
default). `world.Clock.HourOfDay()` backs the banker hour gate.
