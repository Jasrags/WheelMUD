# Sector × climate matrix

Migration 0025 widened `rooms.sector` to the full enum. Pick the
sector that matches the room's *physical character*; pick the climate
on the zone level.

## Sector enum (current)

- `inside` — building interior, stable temperature.
- `city` — urban street, stone/cobble, dense.
- `field` — open grassland, tilled land, meadow.
- `forest` — dense tree cover.
- `hills`
- `mountain`
- `water_swim` — shallow water, fordable.
- `water_noswim` — deep water, river, sea.
- `air` — flying / great height.
- `desert` — Aiel Waste, Termool, sandy/dry.
- `swamp` — bog, marsh.
- `tundra` — Borderlands frost, Spine of the World.
- `road` — paved/maintained route through any of the above.
- `cave` — underground, stoneworks, dungeon.

(Confirm against the migration 0025 enum if adding new values.)

## Climate types

`temperate` / `arid` / `cold` / `tropical` / `subarctic`. Drives
default phase ambients.

## Plausible combos

| Climate | Sector picks |
|---|---|
| temperate | city, field, forest, hills, road, water_*, inside |
| arid | desert, hills, mountain, road, inside |
| cold | tundra, mountain, forest, road, inside, water_noswim |
| tropical | forest, swamp, water_*, road, inside |

## Phase ambients

Per-sector ambient lines are picked from the zone's `ambient:` block
or the sector default. Avoid weather-specific lines that hard-conflict
with the climate (e.g. "snow drifts" in `tropical`).

## See also

- `phase_ambients_open_questions.md` memory — empty-sector default
  policy, windowed-indoors flag, per-room overrides, zone-climate
  variants. Sector defaults ship first; per-room overrides land
  later.
