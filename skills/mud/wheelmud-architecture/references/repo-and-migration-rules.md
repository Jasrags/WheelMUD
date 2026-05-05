# Repo and migration rules

## Forward-only migrations

Migrations live in `internal/db/migrations/` as embedded `.sql` files
named `NNNN_description.sql`. **No down migrations.** If you need to
back something out, write a new forward migration that does the
reverse.

Current series: 0001 → 0032+ (see `CLAUDE.md` for the running
annotated history).

## Column lock-step (CRITICAL)

Three tables have raw-SQL paths in the world loader **and** typed
`Create` paths in their repos. New columns must move both lists in
the same PR or the loader will silently desync from the schema.

### `rooms`

- `internal/repo/room_sqlite.go` — `roomSelectCols`,
  `insertCols`, `insertVals`
- `internal/world/loader.go::roomInsertValues` — `cols`, `vals`

### `items`

- `internal/repo/item_sqlite.go` — `itemSelectCols`, `scanItemRow`,
  `Create` INSERT
- `internal/world/loader.go::insertItems` — raw INSERT, single tx

### `characters`

- `internal/repo/character_sql.go` — `charPlayerColumns`,
  `charPlayerValues`, `charPlayerScanDest` (ordering is load-bearing;
  `auth_level` from migration 0019 is the canonical example)

## Optimistic-lock columns

Two columns currently use a version token bumped on every mutation:

- `items.version` → `ItemRepo.Transfer*` returns `ErrItemMoved` on
  mismatch.
- `characters.coin_version` → `CharacterRepo.RecordCoin(coin, bank,
  expectedVersion)` returns `ErrCoinConflict` on mismatch.

Verbs that mutate these columns pass the version they computed
against. The repo refuses the UPDATE on mismatch, and the verb surfaces
the conflict ("your purse just changed — try again"). `buy` is the
documented exception: it logs-and-accepts because the item already
shipped.

## Item ownership invariant

An item is in **exactly one** of three locations:

- `room_id` set, others `NULL` → on the floor
- `owner_character_id` set, others `NULL` → in someone's inventory
- `parent_item_id` set, others `NULL` → inside another item

`ItemRepo.SetOwner` / `SetRoom` and the `Transfer*` family flip the
relevant columns atomically and clear the other two. Do not write the
columns directly. The `Transfer*` variants (preferred from the command
layer) also guard on prior location so a concurrent `get`/`give`/`put`
race surfaces as `ErrItemMoved`.

`Character.Inventory` (JSON id list on `inventory_json`) is just the
display ordering. SQL `owner_character_id` is the source of truth;
`inventory.go::orderInventory` self-heals stale or missing JSON
entries. Items inside containers are NOT in `inventory_json`;
encumbrance reads them via `ListAllOwnedTransitive` (BFS through
`parent_item_id`).

## Memory + sqlite parity

Every repo has memory + sqlite implementations and a shared test
suite. New repos follow the same pattern (`*_memory.go` + `*_sqlite.go`
+ `*_test.go` exercising both via the suite). If a behavior is hard
to express in memory (e.g. `coin_version` optimistic lock), implement
it in both — do not let the implementations drift.
