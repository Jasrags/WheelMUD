# Encumbrance

Str-keyed carrying-capacity table, d20 SRD-derived. Lives in
`internal/cmd/encumbrance.go::LoadFor`.

## Bands

For each Str score there are three weight thresholds:

| Band | Multiplier (× light) | Effect |
|---|---|---|
| Light | 1× | No penalty |
| Medium | ~3.3× | Speed -10ft, max Dex AC +3, ACP -3 |
| Heavy | ~5× | Speed -20ft, max Dex AC +1, ACP -6 |

Above heavy: `Over-encumbered`, cannot run/charge, additional
penalties.

## Schema mapping

- Per-character carry weight is computed live from inventory; not a
  column.
- `encumbrance.go::LoadFor(str)` returns the three thresholds.
- `ItemRepo.ListAllOwnedTransitive` (BFS through `parent_item_id`)
  sums recursive container contents — items inside containers count
  toward carry weight.

## Penalties not yet applied

The bands are computed and rendered (`score` / `inv` show carried
weight) but combat/movement penalties **are not yet enforced**.
Phase C combat will consume them; Phase A's score screen can already
display the band.

## Item weight

Each item's weight comes from `items.weight` (typed taxonomy from
migration 0015). Do not hardcode. World YAML authors set per-item
weight in `data/world/**/items.yaml`.
